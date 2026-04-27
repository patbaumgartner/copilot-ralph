package oracle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
