package styles

import (
	"strings"
	"testing"

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
	assert.Contains(t, r, "\x1b[")

	// Basic color/constants are non-empty
	assert.NotEmpty(t, Primary)
	assert.NotEmpty(t, Success)

	// Empty style passes through unchanged.
	assert.Equal(t, "Y", Style{}.Render("Y"))
}

// TestApplyPaletteDark verifies the dark palette is applied correctly.
func TestApplyPaletteDark(t *testing.T) {
	applyPalette(true)
	assert.Equal(t, darkPalette.Primary, Primary)
	assert.Equal(t, darkPalette.Success, Success)
	assert.Equal(t, darkPalette.Warning, Warning)
	assert.Equal(t, darkPalette.Error, Error)
	assert.Equal(t, darkPalette.Info, Info)
	// Derived styles must embed the color.
	assert.Contains(t, TitleStyle.Render("T"), "T")
	assert.Contains(t, SuccessStyle.Render("S"), "S")
}

// TestApplyPaletteLight verifies the light palette is applied correctly.
func TestApplyPaletteLight(t *testing.T) {
	applyPalette(false)
	assert.Equal(t, lightPalette.Primary, Primary)
	assert.Equal(t, lightPalette.Success, Success)
	assert.Equal(t, lightPalette.Warning, Warning)
	assert.Equal(t, lightPalette.Error, Error)
	assert.Equal(t, lightPalette.Info, Info)
	// Derived styles must embed the color.
	assert.Contains(t, TitleStyle.Render("T"), "T")
	assert.Contains(t, InfoStyle.Render("I"), "I")

	// Restore dark palette so other tests are unaffected.
	t.Cleanup(func() { applyPalette(true) })
}

// TestIsDarkBackgroundDefault verifies the default is dark when COLORFGBG is unset.
func TestIsDarkBackgroundDefault(t *testing.T) {
	t.Setenv("COLORFGBG", "")
	assert.True(t, isDarkBackground(), "should default to dark when COLORFGBG is empty")
}

// TestIsDarkBackgroundDark verifies detection of dark backgrounds via COLORFGBG.
func TestIsDarkBackgroundDark(t *testing.T) {
	// bg value < 8 → dark
	t.Setenv("COLORFGBG", "15;0")
	assert.True(t, isDarkBackground())

	t.Setenv("COLORFGBG", "15;7")
	assert.True(t, isDarkBackground())
}

// TestIsDarkBackgroundLight verifies detection of light backgrounds via COLORFGBG.
func TestIsDarkBackgroundLight(t *testing.T) {
	// bg value >= 8 → light
	t.Setenv("COLORFGBG", "0;15")
	assert.False(t, isDarkBackground())

	t.Setenv("COLORFGBG", "0;8")
	assert.False(t, isDarkBackground())
}

// TestIsDarkBackgroundMalformed verifies fallback to dark when COLORFGBG is malformed.
func TestIsDarkBackgroundMalformed(t *testing.T) {
	t.Setenv("COLORFGBG", "notanumber")
	assert.True(t, isDarkBackground(), "malformed COLORFGBG should fall back to dark")

	t.Setenv("COLORFGBG", "15;abc")
	assert.True(t, isDarkBackground(), "non-numeric bg should fall back to dark")
}
