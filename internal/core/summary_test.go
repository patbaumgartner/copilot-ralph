package core

import "testing"

func TestExtractSummary(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"none", "no summary here", ""},
		{"single", "<summary>did things</summary>", "did things"},
		{"trims", "<summary>\n  did things\n</summary>", "did things"},
		{"last wins", "<summary>first</summary>\nmore\n<summary>second</summary>", "second"},
		{"multiline", "<summary>line1\nline2</summary>", "line1\nline2"},
		{"empty block", "<summary></summary>", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSummary(tt.in)
			if got != tt.want {
				t.Fatalf("extractSummary(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTruncateSummary(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"under", "abc", 5, "abc"},
		{"exact", "abcde", 5, "abcde"},
		{"over", "abcdef", 5, "abcde…"},
		{"disabled zero", "abcdef", 0, "abcdef"},
		{"disabled neg", "abcdef", -1, "abcdef"},
		{"unicode counts as one rune", "äöüß!!", 4, "äöüß…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateSummary(tt.in, tt.max)
			if got != tt.want {
				t.Fatalf("truncateSummary(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
		})
	}
}
