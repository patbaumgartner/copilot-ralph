// Package styles provides minimal ANSI styling helpers for CLI output.
//
// This package defines colors and text styles used throughout Ralph's
// console output. It uses raw ANSI escape codes (truecolor / 24-bit)
// to avoid pulling in a styling library.
//
// Colors are chosen automatically based on the terminal background.
// Detection uses the COLORFGBG environment variable (set by most terminals).
// When that is absent the dark-background palette is used as the default.
package styles

import (
	"os"
	"strconv"
	"strings"
)

// Color is a truecolor (24-bit) ANSI SGR fragment, e.g. "38;2;255;28;240".
type Color string

// Dark-background palette (vibrant / neon).
var darkPalette = struct {
	Primary, Success, Warning, Error, Info Color
}{
	Primary: "38;2;255;28;240",  // Hot Pink
	Success: "38;2;80;250;123",  // Bright Green
	Warning: "38;2;249;226;175", // Soft Yellow
	Error:   "38;2;243;139;168", // Soft Red
	Info:    "38;2;0;217;255",   // Bright Cyan
}

// Light-background palette (deep / saturated for readability on white).
var lightPalette = struct {
	Primary, Success, Warning, Error, Info Color
}{
	Primary: "38;2;160;0;160", // Deep Magenta
	Success: "38;2;0;128;0",   // Dark Green
	Warning: "38;2;160;90;0",  // Dark Amber
	Error:   "38;2;180;0;0",   // Dark Red
	Info:    "38;2;0;100;180", // Dark Cerulean
}

// isDarkBackground reports whether the terminal appears to use a dark background.
// It inspects COLORFGBG (format "fg;bg", bg < 8 → dark) and falls back to true.
func isDarkBackground() bool {
	if v := os.Getenv("COLORFGBG"); v != "" {
		parts := strings.Split(v, ";")
		if len(parts) >= 2 {
			bg := parts[len(parts)-1]
			if n, err := strconv.Atoi(bg); err == nil {
				return n < 8
			}
		}
	}
	return true // default: assume dark background
}

// Color palette — populated in init based on background detection.
var (
	// Primary colors
	Primary Color
	Success Color
	Warning Color
	Error   Color
	Info    Color
)

func init() {
	applyPalette(isDarkBackground())
}

// applyPalette sets package-level Color variables from the chosen palette.
// Exposed so tests can switch themes without forking a subprocess.
func applyPalette(dark bool) {
	if dark {
		Primary = darkPalette.Primary
		Success = darkPalette.Success
		Warning = darkPalette.Warning
		Error = darkPalette.Error
		Info = darkPalette.Info
	} else {
		Primary = lightPalette.Primary
		Success = lightPalette.Success
		Warning = lightPalette.Warning
		Error = lightPalette.Error
		Info = lightPalette.Info
	}
	// Re-apply to derived Style variables.
	TitleStyle = Style{codes: "1;" + string(Primary)}
	SubTitleStyle = Style{codes: string(Primary)}
	InfoStyle = Style{codes: string(Info)}
	SuccessStyle = Style{codes: string(Success)}
	WarningStyle = Style{codes: string(Warning)}
	ErrorStyle = Style{codes: string(Error)}
}

// Style applies a sequence of ANSI SGR codes around a string.
type Style struct {
	codes string
}

// Render wraps text with this style's ANSI codes and a reset.
func (s Style) Render(text string) string {
	if s.codes == "" {
		return text
	}
	return "\x1b[" + s.codes + "m" + text + "\x1b[0m"
}

// Title styles
var (
	// TitleStyle is for main screen titles (bold + primary color).
	TitleStyle Style
)

// Message styles
var (
	// SubTitleStyle for primary text.
	SubTitleStyle Style
	// InfoStyle for informational messages.
	InfoStyle Style
	// SuccessStyle for success messages.
	SuccessStyle Style
	// WarningStyle for warning messages.
	WarningStyle Style
	// ErrorStyle for error messages.
	ErrorStyle Style
)
