// Package styles provides minimal ANSI styling helpers for CLI output.
//
// This package defines colors and text styles used throughout Ralph's
// console output. It uses raw ANSI escape codes (truecolor / 24-bit)
// to avoid pulling in a styling library.
package styles

// Color is a truecolor (24-bit) ANSI SGR fragment, e.g. "38;2;255;28;240".
type Color string

// Color palette - vibrant theme.
var (
	// Primary colors
	Primary Color = "38;2;255;28;240"  // Hot Pink (main brand color)
	Success Color = "38;2;80;250;123"  // Bright Green (checkmarks, success)
	Warning Color = "38;2;249;226;175" // Yellow (warnings)
	Error   Color = "38;2;243;139;168" // Red (errors)
	Info    Color = "38;2;0;217;255"   // Bright Cyan
)

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
	TitleStyle = Style{codes: "1;" + string(Primary)}
)

// Message styles
var (
	// SubTitleStyle for primary text.
	SubTitleStyle = Style{codes: string(Primary)}
	// InfoStyle for informational messages.
	InfoStyle = Style{codes: string(Info)}
	// SuccessStyle for success messages.
	SuccessStyle = Style{codes: string(Success)}
	// WarningStyle for warning messages.
	WarningStyle = Style{codes: string(Warning)}
	// ErrorStyle for error messages.
	ErrorStyle = Style{codes: string(Error)}
)
