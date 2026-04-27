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

// Light-background palette (high-contrast for readability on white).
// All colours satisfy WCAG AA (≥ 4.5:1) against a white background.
var lightPalette = struct {
	Primary, Success, Warning, Error, Info Color
}{
	Primary: "38;2;130;0;130", // Deep Violet  – contrast ~8:1
	Success: "38;2;0;105;0",   // Forest Green – contrast ~7:1
	Warning: "38;2;150;80;0",  // Burnt Amber  – contrast ~6:1
	Error:   "38;2;180;0;0",   // Dark Red      – contrast ~6:1
	Info:    "38;2;0;80;160",  // Navy Blue     – contrast ~7:1
}

// isDarkBackground reports whether the terminal appears to use a dark background.
//
// Detection order:
//  1. VSCODE_THEME_KIND (set by VS Code for every integrated-terminal session).
//  2. COLORFGBG (format "fg;bg", bg < 8 → dark; set by iTerm2 and most
//     xterm-compatible emulators).
//  3. Falls back to dark (the safer default for most developer environments).
func isDarkBackground() bool {
	// VS Code exposes the active theme kind to every integrated terminal.
	switch os.Getenv("VSCODE_THEME_KIND") {
	case "vscode-light", "vscode-high-contrast-light":
		return false
	case "vscode-dark", "vscode-high-contrast":
		return true
	}

	// iTerm2, xterm, and many other emulators set COLORFGBG.
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
	p := lightPalette
	if dark {
		p = darkPalette
	}
	Primary = p.Primary
	Success = p.Success
	Warning = p.Warning
	Error = p.Error
	Info = p.Info
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
