// Package sdk provides a wrapper around the GitHub Copilot SDK.
//
// This package abstracts Copilot SDK integration, providing session management,
// event handling, and error handling. It provides a simplified interface for
// Ralph's needs while handling the complexity of the underlying SDK.
//
// See .github/copilot-instructions.md for the architectural overview and developer guide.
package sdk

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	copilot "github.com/github/copilot-sdk/go"
)

// Default configuration values.
const (
	DefaultModel     = "gpt-4"
	DefaultLogLevel  = "info"
	DefaultTimeout   = 60 * time.Second
	DefaultStreaming = true
)

// Retry backoff durations for transient errors.
var retryBackoffs = []time.Duration{
	1 * time.Second,
	2 * time.Second,
	5 * time.Second,
}

// isRetryableError determines if an error is transient and can be retried.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// HTTP/2 connection errors are transient
	if strings.Contains(errStr, "GOAWAY") {
		return true
	}
	if strings.Contains(errStr, "connection reset") {
		return true
	}
	if strings.Contains(errStr, "connection refused") {
		return true
	}
	if strings.Contains(errStr, "connection terminated") {
		return true
	}
	if strings.Contains(errStr, "EOF") {
		return true
	}
	if strings.Contains(errStr, "timeout") {
		return true
	}
	return false
}

// CopilotClient wraps the GitHub Copilot SDK.
// It provides session management, event handling, and tool registration.
type CopilotClient struct {
	sdkClient         *copilot.Client
	sdkSession        *copilot.Session
	model             string
	logLevel          string
	workingDir        string
	systemMessageMode string
	systemMessage     string
	timeout           time.Duration
	streaming         bool
	started           bool

	// Rate-limit state. quotaMu guards the fields below, which are written
	// from SDK event callbacks (a separate goroutine) and read by
	// sendPromptWithRetry.
	quotaMu       sync.Mutex
	quotaResetAt  time.Time
	hasQuotaReset bool
	// lastRateLimit captures the most recent rate-limit error reported
	// inside a Send() invocation so the retry loop can pick it up after
	// the call returns.
	lastRateLimit *rateLimitInfo
}

// rateLimitInfo records details about a rate-limit / quota error observed in
// the current Send() invocation.
type rateLimitInfo struct {
	resetAt   time.Time
	message   string
	errorType string
	hasReset  bool
}

// clientConfig holds configuration options for the client.
type clientConfig struct {
	model             string
	logLevel          string
	workingDir        string
	systemMessageMode string
	systemMessage     string
	timeout           time.Duration
	streaming         bool
}

// ClientOption configures the CopilotClient.
type ClientOption func(*clientConfig)

// WithModel sets the AI model to use.
func WithModel(model string) ClientOption {
	return func(c *clientConfig) {
		c.model = model
	}
}

// WithLogLevel sets the logging level (debug, info, warn, error).
func WithLogLevel(level string) ClientOption {
	return func(c *clientConfig) {
		c.logLevel = level
	}
}

// WithWorkingDir sets the working directory for file operations.
func WithWorkingDir(dir string) ClientOption {
	return func(c *clientConfig) {
		c.workingDir = dir
	}
}

// WithStreaming enables or disables streaming responses.
func WithStreaming(streaming bool) ClientOption {
	return func(c *clientConfig) {
		c.streaming = streaming
	}
}

// WithSystemMessage sets the system message for the session.
// Mode can be "append" or "replace".
func WithSystemMessage(message, mode string) ClientOption {
	return func(c *clientConfig) {
		c.systemMessage = message
		c.systemMessageMode = mode
	}
}

// WithTimeout sets the request timeout.
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *clientConfig) {
		c.timeout = timeout
	}
}

// NewCopilotClient creates a new Copilot SDK client with the given options.
// It returns an error if the configuration is invalid.
func NewCopilotClient(opts ...ClientOption) (*CopilotClient, error) {
	// Default configuration
	config := &clientConfig{
		model:             DefaultModel,
		logLevel:          DefaultLogLevel,
		workingDir:        ".",
		streaming:         DefaultStreaming,
		systemMessageMode: "append",
		timeout:           DefaultTimeout,
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	// Validate configuration
	if config.model == "" {
		return nil, fmt.Errorf("model cannot be empty")
	}

	if config.timeout <= 0 {
		return nil, fmt.Errorf("timeout must be positive")
	}

	return &CopilotClient{
		model:             config.model,
		logLevel:          config.logLevel,
		workingDir:        config.workingDir,
		streaming:         config.streaming,
		systemMessageMode: config.systemMessageMode,
		systemMessage:     config.systemMessage,
		timeout:           config.timeout,
		started:           false,
	}, nil
}

// Start initializes the underlying Copilot SDK client. It is idempotent and
// safe to call repeatedly; subsequent calls return nil without re-initializing.
func (c *CopilotClient) Start() error {
	if c.started {
		return nil
	}

	// Initialize the SDK client with options
	c.sdkClient = copilot.NewClient(&copilot.ClientOptions{
		LogLevel: c.logLevel,
		Cwd:      c.workingDir,
	})

	// Start the SDK client
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	if err := c.sdkClient.Start(ctx); err != nil {
		return fmt.Errorf("failed to start SDK client: %w", err)
	}

	c.started = true
	return nil
}

// Stop stops the client and releases resources.
func (c *CopilotClient) Stop() error {
	if !c.started {
		return nil
	}

	// Destroy any active SDK session
	if c.sdkSession != nil {
		_ = c.sdkSession.Disconnect()
		c.sdkSession = nil
	}

	// Stop the SDK client
	if c.sdkClient != nil {
		_ = c.sdkClient.Stop()
		c.sdkClient = nil
	}

	c.started = false
	return nil
}

// CreateSession creates a new Copilot session.
// It initializes the SDK session resources and registers them with the client.
func (c *CopilotClient) CreateSession(ctx context.Context) error {
	if c.sdkClient == nil {
		return fmt.Errorf("SDK client not initialized")
	}

	// Build session config for the SDK
	sessionConfig := &copilot.SessionConfig{
		Model:     c.model,
		Streaming: c.streaming,
		// SDK v0.3.0 requires an explicit permission handler. Ralph runs in
		// non-interactive loops, so we approve all tool/permission requests.
		OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
	}

	// Configure system message if provided
	if c.systemMessage != "" {
		sessionConfig.SystemMessage = &copilot.SystemMessageConfig{
			Mode:    c.systemMessageMode,
			Content: c.systemMessage,
		}
	}

	// Create SDK session
	sdkSession, err := c.sdkClient.CreateSession(ctx, sessionConfig)
	if err != nil {
		return fmt.Errorf("failed to create SDK session: %w", err)
	}

	// Store SDK session reference; we no longer maintain a local Session wrapper
	c.sdkSession = sdkSession
	return nil
}

// DestroySession destroys the current session and cleans up resources.
func (c *CopilotClient) DestroySession(ctx context.Context) error {
	if c.sdkSession == nil {
		return nil
	}

	_ = c.sdkSession.Disconnect()
	c.sdkSession = nil
	return nil
}

// Model returns the configured model name.
func (c *CopilotClient) Model() string {
	return c.model
}

// SendPrompt sends a prompt to the Copilot SDK and returns an event stream.
// The returned channel will be closed when the response is complete.
// An error is returned if there is no active session.
// This method includes automatic retry logic for transient errors.
func (c *CopilotClient) SendPrompt(ctx context.Context, prompt string) (<-chan Event, error) {
	if c.sdkSession == nil {
		return nil, fmt.Errorf("no active session")
	}

	// Create event channel with buffer
	events := make(chan Event, 100)

	// Process prompt asynchronously with retry logic
	go func() {
		defer close(events)
		c.sendPromptWithRetry(ctx, prompt, events)
	}()

	return events, nil
}

// safeEventSender safely sends an event to a channel, recovering from panics if the channel is closed.
// Returns an error if the send failed (e.g., channel closed).
func safeEventSender(events chan<- Event, event Event) (err error) {
	defer func() {
		if r := recover(); r != nil {
			// Channel was closed, ignore the panic
			err = fmt.Errorf("event channel closed")
		}
	}()

	events <- event
	return nil
}

// sendPromptWithRetry sends the prompt with automatic retry for transient errors.
// It transparently handles Copilot rate-limit / quota errors by waiting until
// the reported reset time and retrying the prompt.
func (c *CopilotClient) sendPromptWithRetry(ctx context.Context, prompt string, events chan<- Event) {
	const maxRateLimitRetries = 5

	var lastErr error
	attempt := 0
	rateLimitRetries := 0

	for {
		// Check for context cancellation before each attempt
		select {
		case <-ctx.Done():
			// Don't send error event here - just return so the channel closes
			// The caller will detect cancellation via ctx.Done()
			return
		default:
		}

		// If this is a transient retry, wait before trying again.
		if attempt > 0 && attempt <= len(retryBackoffs) {
			backoff := retryBackoffs[attempt-1]
			select {
			case <-ctx.Done():
				_ = safeEventSender(events, NewErrorEvent(ctx.Err()))
				return
			case <-time.After(backoff):
			}
		}

		// Reset per-call rate-limit state before attempting the send so the
		// handler can repopulate it from any new SessionErrorData.
		c.clearLastRateLimit()

		err := c.sendPromptOnce(ctx, prompt, events)
		if err == nil {
			return
		}

		lastErr = err

		// Did this error come from a Copilot rate-limit / quota condition?
		if rl, ok := c.consumeRateLimit(err); ok {
			if rateLimitRetries >= maxRateLimitRetries {
				_ = safeEventSender(events, NewErrorEvent(fmt.Errorf("rate limit retry budget exhausted: %w", err)))
				return
			}
			rateLimitRetries++

			now := time.Now()
			wait := resolveRateLimitWait(rl.resetAt, rl.hasReset, now)

			_ = safeEventSender(events, NewRateLimitEvent(
				rl.message,
				rl.errorType,
				rl.resetAt,
				rl.hasReset,
				wait,
			))

			if !sleepCtx(ctx, wait) {
				return
			}

			// Reset the transient-retry counter so a rate-limit pause does
			// not consume the connection-error retry budget.
			attempt = 0
			continue
		}

		if !isRetryableError(err) {
			_ = safeEventSender(events, NewErrorEvent(err))
			return
		}

		attempt++
		if attempt > len(retryBackoffs) {
			break
		}
	}

	// All retries exhausted
	_ = safeEventSender(events, NewErrorEvent(fmt.Errorf("max retries exceeded: %w", lastErr)))
}

// sleepCtx sleeps for the given duration, returning false if the context was
// cancelled before the duration elapsed.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// sendPromptOnce sends the prompt once without retrying.
func (c *CopilotClient) sendPromptOnce(ctx context.Context, prompt string, events chan<- Event) error {
	// Set up done channel to wait for session.idle
	done := make(chan struct{})
	doneOnce := &sync.Once{}
	closeDone := func() {
		doneOnce.Do(func() {
			close(done)
		})
	}

	var sessionErr error
	pendingToolCalls := make(map[string]ToolCall)

	// Subscribe to SDK session events
	unsubscribe := c.sdkSession.On(func(event copilot.SessionEvent) {
		// Check if context is cancelled before processing events
		select {
		case <-ctx.Done():
			// Context cancelled, close done channel to unblock and stop processing
			closeDone()
			return
		default:
		}

		if d, ok := event.Data.(*copilot.SessionErrorData); ok {
			sessionErr = fmt.Errorf("SDK error: %s", d.Message)
		}

		c.handleSDKEvent(event, events, closeDone, pendingToolCalls)
	})

	defer unsubscribe()

	// Send the message
	_, err := c.sdkSession.Send(ctx, copilot.MessageOptions{
		Prompt: prompt,
	})
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// Wait for session to become idle or context cancellation
	select {
	case <-ctx.Done():
		// Abort the session and close done to unblock any waiting
		go func() {
			abortCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = c.sdkSession.Abort(abortCtx)
		}()

		closeDone()
		return ctx.Err()
	case <-done:
		// Response complete - check for session error
		if sessionErr != nil {
			return sessionErr
		}
	}

	return nil
}

// handleSDKEvent processes events from the Copilot SDK and forwards them.
// Uses safeEventSender to protect against writing to closed channels.
//
// Adapted for copilot-sdk/go v0.3.0: SessionEvent.Data is now an interface
// (SessionEventData) with concrete per-event types instead of a single flat
// struct. We type-switch on it and copy out only the fields we care about.
func (c *CopilotClient) handleSDKEvent(sdkEvent copilot.SessionEvent, events chan<- Event, closeDone func(), pendingToolCalls map[string]ToolCall) {
	switch d := sdkEvent.Data.(type) {
	case *copilot.AssistantMessageDeltaData:
		if d.DeltaContent == "" {
			return
		}
		_ = safeEventSender(events, NewTextEvent(d.DeltaContent, false))

	case *copilot.AssistantReasoningDeltaData:
		if d.DeltaContent == "" {
			return
		}
		_ = safeEventSender(events, NewTextEvent(d.DeltaContent, true))

	case *copilot.AssistantMessageData:
		if d.Content == "" {
			return
		}
		_ = safeEventSender(events, NewTextEvent(d.Content, false))

	case *copilot.AssistantReasoningData:
		if d.Content == "" {
			return
		}
		_ = safeEventSender(events, NewTextEvent(d.Content, true))

	case *copilot.ToolExecutionStartData:
		toolCall := ToolCall{
			ID:   d.ToolCallID,
			Name: d.ToolName,
		}
		if args, ok := d.Arguments.(map[string]any); ok {
			toolCall.Parameters = args
		}
		pendingToolCalls[toolCall.ID] = toolCall
		_ = safeEventSender(events, NewToolCallEvent(toolCall))

	case *copilot.ToolExecutionCompleteData:
		var toolCall ToolCall
		if tc, ok := pendingToolCalls[d.ToolCallID]; ok {
			toolCall = tc
			delete(pendingToolCalls, d.ToolCallID)
		}
		if toolCall.ID == "" {
			toolCall.ID = d.ToolCallID
		}

		var result string
		if d.Result != nil {
			result = d.Result.Content
		}

		var toolErr error
		if !d.Success && d.Error != nil {
			toolErr = fmt.Errorf("%s", d.Error.Message)
		}

		_ = safeEventSender(events, NewToolResultEvent(toolCall, result, toolErr))

	case *copilot.SessionIdleData:
		closeDone()

	case *copilot.SessionErrorData:
		// Detect rate-limit / quota errors so the retry loop can wait for
		// the reset window. The session error is also surfaced as a normal
		// error event so existing consumers continue to see it.
		if c.recordRateLimit(d) {
			// Don't emit a generic error event for rate-limit cases; the
			// retry loop will emit a dedicated RateLimitEvent instead.
			return
		}
		_ = safeEventSender(events, NewErrorEvent(fmt.Errorf("SDK error: %s", d.Message)))

	case *copilot.AssistantUsageData:
		c.recordQuotaSnapshots(d)
	}

	// `strings` import retained for future use, suppress unused-import lint if any
	_ = strings.Contains
}

// clearLastRateLimit resets the per-Send rate-limit state.
func (c *CopilotClient) clearLastRateLimit() {
	c.quotaMu.Lock()
	c.lastRateLimit = nil
	c.quotaMu.Unlock()
}

// recordRateLimit captures rate-limit details from a SessionErrorData event.
// It returns true if the error was identified as a rate-limit / quota error.
func (c *CopilotClient) recordRateLimit(d *copilot.SessionErrorData) bool {
	if d == nil {
		return false
	}
	if !isRateLimitErrorType(d.ErrorType) && !isRateLimitMessage(d.Message) {
		return false
	}

	info := &rateLimitInfo{
		message:   d.Message,
		errorType: d.ErrorType,
	}

	now := time.Now()
	if reset, ok := parseRateLimitReset(d.Message, now); ok {
		info.resetAt = reset
		info.hasReset = true
	}

	c.quotaMu.Lock()
	defer c.quotaMu.Unlock()

	// Fall back to the most recent quota snapshot reset if the message did
	// not include one of its own.
	if !info.hasReset && c.hasQuotaReset && c.quotaResetAt.After(now) {
		info.resetAt = c.quotaResetAt
		info.hasReset = true
	}

	c.lastRateLimit = info
	return true
}

// recordQuotaSnapshots tracks the soonest future quota reset reported by an
// AssistantUsageData event so subsequent rate-limit errors can fall back to
// it when the error message lacks an explicit timestamp.
func (c *CopilotClient) recordQuotaSnapshots(d *copilot.AssistantUsageData) {
	if d == nil || len(d.QuotaSnapshots) == 0 {
		return
	}
	now := time.Now()
	var soonest time.Time
	var found bool
	for _, snap := range d.QuotaSnapshots {
		if snap.ResetDate == nil {
			continue
		}
		ts := *snap.ResetDate
		if !ts.After(now) {
			continue
		}
		if !found || ts.Before(soonest) {
			soonest = ts
			found = true
		}
	}
	if !found {
		return
	}
	c.quotaMu.Lock()
	c.quotaResetAt = soonest
	c.hasQuotaReset = true
	c.quotaMu.Unlock()
}

// consumeRateLimit returns the rate-limit info recorded for the most recent
// Send() invocation, if any. The state is cleared so the next attempt starts
// fresh. If no SessionErrorData arrived but the returned err itself looks
// like a rate-limit error, a best-effort info struct is constructed.
func (c *CopilotClient) consumeRateLimit(err error) (rateLimitInfo, bool) {
	c.quotaMu.Lock()
	info := c.lastRateLimit
	c.lastRateLimit = nil
	quotaResetAt := c.quotaResetAt
	hasQuotaReset := c.hasQuotaReset
	c.quotaMu.Unlock()

	if info != nil {
		return *info, true
	}

	if err == nil || !isRateLimitMessage(err.Error()) {
		return rateLimitInfo{}, false
	}

	now := time.Now()
	out := rateLimitInfo{message: err.Error()}
	if reset, ok := parseRateLimitReset(err.Error(), now); ok {
		out.resetAt = reset
		out.hasReset = true
	}
	if !out.hasReset && hasQuotaReset && quotaResetAt.After(now) {
		out.resetAt = quotaResetAt
		out.hasReset = true
	}
	return out, true
}
