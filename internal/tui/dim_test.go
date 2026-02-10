package tui

import (
	"strings"
	"testing"
)

func TestDimANSIColors_EmptyString(t *testing.T) {
	result := dimANSIColors("", 0.4)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestDimANSIColors_PlainText(t *testing.T) {
	result := dimANSIColors("hello world", 0.4)
	// Should prepend dim default foreground then the text.
	if !strings.HasPrefix(result, "\x1b[38;2;91;100;109m") {
		t.Errorf("expected dim default prefix, got %q", result)
	}
	if !strings.HasSuffix(result, "hello world") {
		t.Errorf("expected text at end, got %q", result)
	}
}

func TestDimANSIColors_24BitForeground(t *testing.T) {
	// Red foreground: ESC[38;2;255;0;0m → dimmed at 0.5 → 127;0;0
	input := "\x1b[38;2;255;0;0mhello"
	result := dimANSIColors(input, 0.5)
	if !strings.Contains(result, "\x1b[38;2;127;0;0m") {
		t.Errorf("expected dimmed 24-bit fg, got %q", result)
	}
	if !strings.HasSuffix(result, "hello") {
		t.Errorf("expected text at end, got %q", result)
	}
}

func TestDimANSIColors_24BitBackground(t *testing.T) {
	// Green background: ESC[48;2;0;200;0m → dimmed at 0.5 → 0;100;0
	input := "\x1b[48;2;0;200;0mhello"
	result := dimANSIColors(input, 0.5)
	if !strings.Contains(result, "\x1b[48;2;0;100;0m") {
		t.Errorf("expected dimmed 24-bit bg, got %q", result)
	}
}

func TestDimANSIColors_256Color(t *testing.T) {
	// 256-color index 196 = bright red (255,0,0) → dimmed at 0.5 → 127,0,0
	input := "\x1b[38;5;196mred"
	result := dimANSIColors(input, 0.5)
	if !strings.Contains(result, "\x1b[38;2;127;0;0m") {
		t.Errorf("expected 256-color converted to dimmed 24-bit, got %q", result)
	}
}

func TestDimANSIColors_256ColorBackground(t *testing.T) {
	// 256-color bg index 21 = blue (0,0,255) → dimmed at 0.5 → 0,0,127
	input := "\x1b[48;5;21mblue"
	result := dimANSIColors(input, 0.5)
	if !strings.Contains(result, "\x1b[48;2;0;0;127m") {
		t.Errorf("expected 256-color bg converted to dimmed 24-bit, got %q", result)
	}
}

func TestDimANSIColors_Basic16Foreground(t *testing.T) {
	// Red (31) → xterm red (205,0,0) → dimmed at 0.4 → 82,0,0
	input := "\x1b[31mred text"
	result := dimANSIColors(input, 0.4)
	if !strings.Contains(result, "\x1b[38;2;82;0;0m") {
		t.Errorf("expected basic red dimmed to 24-bit, got %q", result)
	}
}

func TestDimANSIColors_Basic16Background(t *testing.T) {
	// Green bg (42) → xterm green (0,205,0) → dimmed at 0.4 → 0,82,0
	input := "\x1b[42mgreen bg"
	result := dimANSIColors(input, 0.4)
	if !strings.Contains(result, "\x1b[48;2;0;82;0m") {
		t.Errorf("expected basic green bg dimmed to 24-bit, got %q", result)
	}
}

func TestDimANSIColors_BrightForeground(t *testing.T) {
	// Bright red (91) → (255,0,0) → dimmed at 0.4 → 102,0,0
	input := "\x1b[91mbright red"
	result := dimANSIColors(input, 0.4)
	if !strings.Contains(result, "\x1b[38;2;102;0;0m") {
		t.Errorf("expected bright red dimmed, got %q", result)
	}
}

func TestDimANSIColors_BrightBackground(t *testing.T) {
	// Bright cyan bg (106) → bright cyan bg is 100+6=106, index 8+6=14 → (0,255,255) → dimmed at 0.4 → 0,102,102
	input := "\x1b[106mbright cyan bg"
	result := dimANSIColors(input, 0.4)
	if !strings.Contains(result, "\x1b[48;2;0;102;102m") {
		t.Errorf("expected bright cyan bg dimmed, got %q", result)
	}
}

func TestDimANSIColors_Reset(t *testing.T) {
	// Reset should re-apply dim default foreground.
	input := "\x1b[31mred\x1b[0mnormal"
	result := dimANSIColors(input, 0.4)
	// After reset, should see re-applied dim default.
	if !strings.Contains(result, "\x1b[0;38;2;91;100;109m") {
		t.Errorf("expected reset + dim default, got %q", result)
	}
}

func TestDimANSIColors_DefaultForeground(t *testing.T) {
	// ESC[39m (default fg) → replaced with dim default.
	input := "\x1b[39mdefault"
	result := dimANSIColors(input, 0.4)
	if !strings.Contains(result, "\x1b[38;2;91;100;109m") {
		t.Errorf("expected default fg replaced with dim, got %q", result)
	}
}

func TestDimANSIColors_NonColorSGR(t *testing.T) {
	// Bold (1), italic (3), underline (4) should pass through.
	input := "\x1b[1;3;4mformatted"
	result := dimANSIColors(input, 0.4)
	if !strings.Contains(result, "\x1b[1;3;4m") {
		t.Errorf("expected non-color SGR passed through, got %q", result)
	}
}

func TestDimANSIColors_NonSGRSequence(t *testing.T) {
	// Cursor movement ESC[H should pass through.
	input := "\x1b[Hhello"
	result := dimANSIColors(input, 0.4)
	if !strings.Contains(result, "\x1b[H") {
		t.Errorf("expected non-SGR sequence passed through, got %q", result)
	}
}

func TestDimANSIColors_MixedContent(t *testing.T) {
	// Mix of colored text, reset, and plain text.
	input := "\x1b[38;2;255;100;50mcolored\x1b[0m plain \x1b[1mbold"
	result := dimANSIColors(input, 0.5)
	// Should contain dimmed color.
	if !strings.Contains(result, "\x1b[38;2;127;50;25m") {
		t.Errorf("expected dimmed color in mixed content, got %q", result)
	}
	// Should contain reset + dim default.
	if !strings.Contains(result, "\x1b[0;38;2;91;100;109m") {
		t.Errorf("expected reset + dim default in mixed content, got %q", result)
	}
	// Should contain bold passed through.
	if !strings.Contains(result, "\x1b[1m") {
		t.Errorf("expected bold passed through in mixed content, got %q", result)
	}
}

func TestDimANSIColors_CombinedColorAndStyle(t *testing.T) {
	// ESC[1;31m = bold + red fg
	input := "\x1b[1;31mcombined"
	result := dimANSIColors(input, 0.4)
	// Bold should pass through, red should be dimmed.
	if !strings.Contains(result, "1;38;2;82;0;0") {
		t.Errorf("expected bold + dimmed red, got %q", result)
	}
}

func TestDimANSIColors_EmptyResetSequence(t *testing.T) {
	// ESC[m is equivalent to ESC[0m.
	input := "\x1b[mtext"
	result := dimANSIColors(input, 0.4)
	if !strings.Contains(result, "\x1b[0;38;2;91;100;109m") {
		t.Errorf("expected empty reset treated as reset, got %q", result)
	}
}

func TestDimANSIColors_GrayscaleRamp256(t *testing.T) {
	// 256-color index 240 = grayscale (8 + (240-232)*10 = 88) → dimmed at 0.5 → 44
	input := "\x1b[38;5;240mgray"
	result := dimANSIColors(input, 0.5)
	if !strings.Contains(result, "\x1b[38;2;44;44;44m") {
		t.Errorf("expected grayscale dimmed, got %q", result)
	}
}

func TestColor256ToRGB_Cube(t *testing.T) {
	// Index 196 = 6x6x6 cube: (196-16) = 180, r=180/36=5, g=(180/6)%6=0, b=180%6=0
	// r=55+5*40=255, g=0, b=0
	r, g, b := color256ToRGB(196)
	if r != 255 || g != 0 || b != 0 {
		t.Errorf("expected (255,0,0), got (%d,%d,%d)", r, g, b)
	}
}

func TestColor256ToRGB_Grayscale(t *testing.T) {
	// Index 232 = grayscale: 8 + 0*10 = 8
	r, g, b := color256ToRGB(232)
	if r != 8 || g != 8 || b != 8 {
		t.Errorf("expected (8,8,8), got (%d,%d,%d)", r, g, b)
	}
}

func TestColor256ToRGB_Basic(t *testing.T) {
	// Index 1 = red (205,0,0)
	r, g, b := color256ToRGB(1)
	if r != 205 || g != 0 || b != 0 {
		t.Errorf("expected (205,0,0), got (%d,%d,%d)", r, g, b)
	}
}

func TestDimRGB(t *testing.T) {
	r, g, b := dimRGB(200, 100, 50, 0.5)
	if r != 100 || g != 50 || b != 25 {
		t.Errorf("expected (100,50,25), got (%d,%d,%d)", r, g, b)
	}
}

func TestDimANSIColors_NewlineReappliesDimDefault(t *testing.T) {
	// Each newline should re-emit dim default so that lipgloss.JoinHorizontal
	// doesn't leave lines inheriting the sidebar's reset state.
	input := "line1\nline2"
	result := dimANSIColors(input, 0.4)
	dimDefault := "\x1b[38;2;91;100;109m"
	expected := dimDefault + "line1\n" + dimDefault + "line2"
	if result != expected {
		t.Errorf("expected dim default after newline\ngot:  %q\nwant: %q", result, expected)
	}
}

func TestDimANSIColors_NonCSIEscapeDigitTerminator(t *testing.T) {
	// ESC ( 0 (graphics charset) has a digit terminator.
	// Parser must NOT over-consume subsequent content.
	input := "\x1b(0\x1b[38;2;255;0;0mred"
	result := dimANSIColors(input, 0.5)
	// The graphics charset escape should be passed through.
	if !strings.Contains(result, "\x1b(0") {
		t.Errorf("expected ESC(0 passed through, got %q", result)
	}
	// The color should be dimmed (not swallowed).
	if !strings.Contains(result, "\x1b[38;2;127;0;0m") {
		t.Errorf("expected color to be dimmed after ESC(0, got %q", result)
	}
}

func TestDimANSIColors_SaveRestoreCursor(t *testing.T) {
	// ESC 7 (save cursor) is a 2-byte escape with digit terminator.
	input := "\x1b7\x1b[31mred"
	result := dimANSIColors(input, 0.4)
	if !strings.Contains(result, "\x1b7") {
		t.Errorf("expected ESC7 passed through, got %q", result)
	}
	if !strings.Contains(result, "\x1b[38;2;82;0;0m") {
		t.Errorf("expected red dimmed after ESC7, got %q", result)
	}
}

func TestDimANSIColors_OSCSequence(t *testing.T) {
	// OSC sequence terminated by BEL.
	input := "\x1b]8;;https://example.com\x07link text"
	result := dimANSIColors(input, 0.4)
	if !strings.Contains(result, "\x1b]8;;https://example.com\x07") {
		t.Errorf("expected OSC sequence passed through, got %q", result)
	}
	if !strings.Contains(result, "link text") {
		t.Errorf("expected text after OSC, got %q", result)
	}
}
