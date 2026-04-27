package oracle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patbaumgartner/copilot-ralph/internal/sdk"
)

type fakeOracleClient struct {
	events               []sdk.Event
	startErr             error
	createSessionErr     error
	sendPromptErr        error
	stopErr              error
	prompt               string
	started              bool
	sessionCreated       bool
	sessionDestroyed     bool
	stopped              bool
	destroySessionCalled int
}

func (f *fakeOracleClient) Start() error {
	f.started = true
	return f.startErr
}

func (f *fakeOracleClient) Stop() error {
	f.stopped = true
	return f.stopErr
}

func (f *fakeOracleClient) CreateSession(context.Context) error {
	f.sessionCreated = true
	return f.createSessionErr
}

func (f *fakeOracleClient) DestroySession(context.Context) error {
	f.destroySessionCalled++
	f.sessionDestroyed = true
	return nil
}

func (f *fakeOracleClient) SendPrompt(_ context.Context, prompt string) (<-chan sdk.Event, error) {
	f.prompt = prompt
	if f.sendPromptErr != nil {
		return nil, f.sendPromptErr
	}
	events := make(chan sdk.Event, len(f.events))
	for _, ev := range f.events {
		events <- ev
	}
	close(events)
	return events, nil
}

// TestNewReturnsClient asserts the constructor builds a non-nil oracle
// even when the model is unknown — the SDK only reports the model error
// at session-creation time.
func TestNewReturnsClient(t *testing.T) {
	o, err := New("gpt-test", t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, o)
	assert.NotNil(t, o.client)
}

// TestNewEmptyModelErrors ensures empty model is rejected at construction.
func TestNewEmptyModelErrors(t *testing.T) {
	o, err := New("", t.TempDir())
	require.Error(t, err)
	assert.Nil(t, o)
}

// TestConsultNilReceiver makes sure Consult fails cleanly when the
// oracle was never constructed.
func TestConsultNilReceiver(t *testing.T) {
	var o *SDKOracle
	out, err := o.Consult(context.Background(), "hi")
	require.Error(t, err)
	assert.Empty(t, out)
}

// TestConsultEmptyClient guards against a partially-initialised oracle.
func TestConsultEmptyClient(t *testing.T) {
	o := &SDKOracle{}
	out, err := o.Consult(context.Background(), "hi")
	require.Error(t, err)
	assert.Empty(t, out)
}

func TestConsultCollectsAssistantText(t *testing.T) {
	client := &fakeOracleClient{
		events: []sdk.Event{
			sdk.NewTextEvent(" reasoning ", true),
			sdk.NewTextEvent(" hello", false),
			sdk.NewTextEvent(" world ", false),
		},
	}
	o := &SDKOracle{client: client}

	out, err := o.Consult(context.Background(), "review this")
	require.NoError(t, err)
	assert.Equal(t, "hello world", out)
	assert.True(t, client.started)
	assert.True(t, client.sessionCreated)
	assert.True(t, client.sessionDestroyed)
	assert.Equal(t, "review this", client.prompt)
	assert.Equal(t, 1, client.destroySessionCalled)
}

func TestConsultReturnsErrorBeforeText(t *testing.T) {
	client := &fakeOracleClient{
		events: []sdk.Event{sdk.NewErrorEvent(errors.New("boom"))},
	}
	o := &SDKOracle{client: client}

	out, err := o.Consult(context.Background(), "review this")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oracle: boom")
	assert.Empty(t, out)
	assert.True(t, client.sessionDestroyed)
}

func TestConsultIgnoresErrorAfterText(t *testing.T) {
	client := &fakeOracleClient{
		events: []sdk.Event{
			sdk.NewTextEvent("answer", false),
			sdk.NewErrorEvent(errors.New("late warning")),
		},
	}
	o := &SDKOracle{client: client}

	out, err := o.Consult(context.Background(), "review this")
	require.NoError(t, err)
	assert.Equal(t, "answer", out)
}

func TestConsultWrapsLifecycleErrors(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		o := &SDKOracle{client: &fakeOracleClient{startErr: errors.New("no sdk")}}
		out, err := o.Consult(context.Background(), "hi")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start oracle: no sdk")
		assert.Empty(t, out)
	})

	t.Run("create session", func(t *testing.T) {
		client := &fakeOracleClient{createSessionErr: errors.New("bad model")}
		o := &SDKOracle{client: client}
		out, err := o.Consult(context.Background(), "hi")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create oracle session: bad model")
		assert.Empty(t, out)
		assert.False(t, client.sessionDestroyed)
	})

	t.Run("send prompt", func(t *testing.T) {
		client := &fakeOracleClient{sendPromptErr: errors.New("send failed")}
		o := &SDKOracle{client: client}
		out, err := o.Consult(context.Background(), "hi")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "send oracle prompt: send failed")
		assert.Empty(t, out)
		assert.True(t, client.sessionDestroyed)
	})
}

func TestClose(t *testing.T) {
	var nilOracle *SDKOracle
	require.NoError(t, nilOracle.Close())
	require.NoError(t, (&SDKOracle{}).Close())

	client := &fakeOracleClient{stopErr: errors.New("stop failed")}
	err := (&SDKOracle{client: client}).Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stop failed")
	assert.True(t, client.stopped)
}
