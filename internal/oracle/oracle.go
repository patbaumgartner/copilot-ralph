// Package oracle provides a one-shot second-opinion client backed by
// the Copilot SDK. It is intentionally separate from the streaming
// session client used by the main loop so the two cannot interfere.
package oracle

import (
	"context"
	"fmt"
	"strings"

	"github.com/patbaumgartner/copilot-ralph/internal/sdk"
)

// SDKOracle wraps a CopilotClient to satisfy core.OracleClient. The
// wrapped client owns its own session lifecycle managed by Consult.
type SDKOracle struct {
	client oracleClient
}

type oracleClient interface {
	Start() error
	Stop() error
	CreateSession(context.Context) error
	DestroySession(context.Context) error
	SendPrompt(context.Context, string) (<-chan sdk.Event, error)
}

// New builds a fresh oracle client targeting model. The caller passes
// the same working directory used by the main loop so file-touching
// tools resolve to the right tree.
func New(model, workingDir string) (*SDKOracle, error) {
	c, err := sdk.NewCopilotClient(
		sdk.WithModel(model),
		sdk.WithWorkingDir(workingDir),
		sdk.WithStreaming(false),
		sdk.WithLogLevel("error"),
	)
	if err != nil {
		return nil, fmt.Errorf("oracle client: %w", err)
	}
	return &SDKOracle{client: c}, nil
}

// Consult sends prompt to the oracle and returns the assembled assistant
// reply. It manages a fresh session per consultation so previous
// invocations cannot bleed into the next one.
func (o *SDKOracle) Consult(ctx context.Context, prompt string) (string, error) {
	if o == nil || o.client == nil {
		return "", fmt.Errorf("oracle not configured")
	}
	if err := o.client.Start(); err != nil {
		return "", fmt.Errorf("start oracle: %w", err)
	}
	if err := o.client.CreateSession(ctx); err != nil {
		return "", fmt.Errorf("create oracle session: %w", err)
	}
	defer func() {
		_ = o.client.DestroySession(ctx)
	}()

	events, err := o.client.SendPrompt(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("send oracle prompt: %w", err)
	}

	var b strings.Builder
	for ev := range events {
		switch e := ev.(type) {
		case *sdk.TextEvent:
			if !e.Reasoning {
				b.WriteString(e.Text)
			}
		case *sdk.ErrorEvent:
			if b.Len() == 0 {
				return "", fmt.Errorf("oracle: %w", e.Err)
			}
		}
	}
	return strings.TrimSpace(b.String()), nil
}

// Close releases the underlying client.
func (o *SDKOracle) Close() error {
	if o == nil || o.client == nil {
		return nil
	}
	return o.client.Stop()
}
