package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func TestRalphFoxAndStyles(t *testing.T) {
	// ASCII art should be non-empty and recognisable as the new fox.
	assert.True(t, len(RalphFox) > 0)
	assert.True(t, strings.Contains(RalphFox, "+++"))
	assert.True(t, strings.Contains(RalphFox, "###"))

	// Ensure style variables render without panicking and include content
	r := TitleStyle.Render("X")
	assert.Contains(t, r, "X")

	// Basic color/constants are non-empty
	assert.NotEmpty(t, Primary)
	assert.NotEmpty(t, Success)

	// Verify lipgloss style construction works (sanity check)
	_ = lipgloss.NewStyle()
}
