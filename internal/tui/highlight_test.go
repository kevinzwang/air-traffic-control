package tui

import (
	"strings"
	"testing"
)

func TestLightenRGB(t *testing.T) {
	// Black (0,0,0) lightened by 0.35 → (89,89,89)
	r, g, b := lightenRGB(0, 0, 0, 0.35)
	if r != 89 || g != 89 || b != 89 {
		t.Errorf("lighten black: expected (89,89,89), got (%d,%d,%d)", r, g, b)
	}

	// White (255,255,255) lightened by 0.35 → stays (255,255,255)
	r, g, b = lightenRGB(255, 255, 255, 0.35)
	if r != 255 || g != 255 || b != 255 {
		t.Errorf("lighten white: expected (255,255,255), got (%d,%d,%d)", r, g, b)
	}

	// Red (200,0,0) lightened by 0.5 → (227,127,127)
	r, g, b = lightenRGB(200, 0, 0, 0.5)
	if r != 227 || g != 127 || b != 127 {
		t.Errorf("lighten red: expected (227,127,127), got (%d,%d,%d)", r, g, b)
	}

	// Factor 0 → no change
	r, g, b = lightenRGB(100, 50, 25, 0.0)
	if r != 100 || g != 50 || b != 25 {
		t.Errorf("lighten factor 0: expected (100,50,25), got (%d,%d,%d)", r, g, b)
	}
}

func TestUpdateColorState(t *testing.T) {
	t.Run("reset", func(t *testing.T) {
		state := ansiColorState{fgSet: true, fgR: 255, bgSet: true, bgR: 100}
		updateColorState(&state, "0")
		if state.fgSet || state.bgSet {
			t.Error("expected reset to clear fg and bg")
		}
	})

	t.Run("empty reset", func(t *testing.T) {
		state := ansiColorState{fgSet: true, fgR: 255}
		updateColorState(&state, "")
		if state.fgSet {
			t.Error("expected empty param to reset fg")
		}
	})

	t.Run("24-bit foreground", func(t *testing.T) {
		var state ansiColorState
		updateColorState(&state, "38;2;100;200;50")
		if !state.fgSet || state.fgR != 100 || state.fgG != 200 || state.fgB != 50 {
			t.Errorf("expected fg (100,200,50), got (%d,%d,%d) set=%v", state.fgR, state.fgG, state.fgB, state.fgSet)
		}
	})

	t.Run("24-bit background", func(t *testing.T) {
		var state ansiColorState
		updateColorState(&state, "48;2;10;20;30")
		if !state.bgSet || state.bgR != 10 || state.bgG != 20 || state.bgB != 30 {
			t.Errorf("expected bg (10,20,30), got (%d,%d,%d) set=%v", state.bgR, state.bgG, state.bgB, state.bgSet)
		}
	})

	t.Run("256-color foreground", func(t *testing.T) {
		var state ansiColorState
		updateColorState(&state, "38;5;196") // 196 = bright red (255,0,0)
		if !state.fgSet || state.fgR != 255 || state.fgG != 0 || state.fgB != 0 {
			t.Errorf("expected fg (255,0,0), got (%d,%d,%d)", state.fgR, state.fgG, state.fgB)
		}
	})

	t.Run("256-color background", func(t *testing.T) {
		var state ansiColorState
		updateColorState(&state, "48;5;21") // 21 = blue (0,0,255)
		if !state.bgSet || state.bgR != 0 || state.bgG != 0 || state.bgB != 255 {
			t.Errorf("expected bg (0,0,255), got (%d,%d,%d)", state.bgR, state.bgG, state.bgB)
		}
	})

	t.Run("basic foreground", func(t *testing.T) {
		var state ansiColorState
		updateColorState(&state, "31") // Red = (205,0,0)
		if !state.fgSet || state.fgR != 205 || state.fgG != 0 || state.fgB != 0 {
			t.Errorf("expected fg (205,0,0), got (%d,%d,%d)", state.fgR, state.fgG, state.fgB)
		}
	})

	t.Run("basic background", func(t *testing.T) {
		var state ansiColorState
		updateColorState(&state, "42") // Green = (0,205,0)
		if !state.bgSet || state.bgR != 0 || state.bgG != 205 || state.bgB != 0 {
			t.Errorf("expected bg (0,205,0), got (%d,%d,%d)", state.bgR, state.bgG, state.bgB)
		}
	})

	t.Run("bright foreground", func(t *testing.T) {
		var state ansiColorState
		updateColorState(&state, "91") // Bright red = (255,0,0)
		if !state.fgSet || state.fgR != 255 || state.fgG != 0 || state.fgB != 0 {
			t.Errorf("expected fg (255,0,0), got (%d,%d,%d)", state.fgR, state.fgG, state.fgB)
		}
	})

	t.Run("bright background", func(t *testing.T) {
		var state ansiColorState
		updateColorState(&state, "106") // Bright cyan bg = (0,255,255)
		if !state.bgSet || state.bgR != 0 || state.bgG != 255 || state.bgB != 255 {
			t.Errorf("expected bg (0,255,255), got (%d,%d,%d)", state.bgR, state.bgG, state.bgB)
		}
	})

	t.Run("default foreground", func(t *testing.T) {
		state := ansiColorState{fgSet: true, fgR: 255}
		updateColorState(&state, "39")
		if state.fgSet {
			t.Error("expected code 39 to clear fgSet")
		}
	})

	t.Run("default background", func(t *testing.T) {
		state := ansiColorState{bgSet: true, bgR: 100}
		updateColorState(&state, "49")
		if state.bgSet {
			t.Error("expected code 49 to clear bgSet")
		}
	})

	t.Run("non-color attribute ignored", func(t *testing.T) {
		var state ansiColorState
		updateColorState(&state, "1;3;4") // bold, italic, underline
		if state.fgSet || state.bgSet {
			t.Error("non-color attributes should not set colors")
		}
	})
}

func TestApplyHighlightToLine_PlainText(t *testing.T) {
	line := "hello world"
	result := applyHighlightToLine(line, 0, 4, 0.35)

	// Should contain "hello" with highlight SGR around it
	if !strings.Contains(result, "hello") {
		t.Errorf("expected 'hello' in result, got %q", result)
	}
	// Should contain a 38;2 (fg color) SGR before the highlighted text
	if !strings.Contains(result, "\x1b[38;2;") {
		t.Errorf("expected lightened fg SGR, got %q", result)
	}
	// Should contain a 48;2 (bg color) SGR for the background
	if !strings.Contains(result, "\x1b[48;2;") {
		t.Errorf("expected lightened bg SGR, got %q", result)
	}
	// Should NOT contain reverse video
	if strings.Contains(result, "\x1b[7m") {
		t.Errorf("should not use reverse video, got %q", result)
	}
	// "world" should appear after restore SGR (outside highlight)
	if !strings.Contains(result, "world") {
		t.Errorf("expected 'world' in result, got %q", result)
	}
}

func TestApplyHighlightToLine_WithANSIColors(t *testing.T) {
	// Line with red foreground: the highlight should lighten the red, not reverse it.
	line := "\x1b[38;2;200;0;0mred text"
	result := applyHighlightToLine(line, 0, 2, 0.35)

	// The original SGR should be present.
	if !strings.Contains(result, "\x1b[38;2;200;0;0m") {
		t.Errorf("expected original color SGR, got %q", result)
	}

	// Lightened red: 200 + int(55*0.35) = 200 + 19 = 219
	if !strings.Contains(result, "219") {
		t.Errorf("expected lightened red (219) in highlight SGR, got %q", result)
	}

	// Should NOT contain reverse video.
	if strings.Contains(result, "\x1b[7m") {
		t.Errorf("should not use reverse video, got %q", result)
	}
}

func TestApplyHighlightToLine_ANSIMidSelection(t *testing.T) {
	// Regression test for issue 2: style change mid-selection shouldn't break highlight.
	// "ab" in default, then bold+color change, then "cd".
	line := "ab\x1b[1;38;2;100;150;200mcd"
	result := applyHighlightToLine(line, 0, 3, 0.35)

	// All 4 chars should be present.
	if !strings.Contains(result, "ab") {
		t.Errorf("expected 'ab' in result, got %q", result)
	}
	if !strings.Contains(result, "cd") {
		t.Errorf("expected 'cd' in result, got %q", result)
	}

	// After the mid-selection SGR, the highlight should be re-emitted.
	// The original SGR (bold+color) should be present.
	if !strings.Contains(result, "\x1b[1;38;2;100;150;200m") {
		t.Errorf("expected original mid-selection SGR, got %q", result)
	}

	// Lightened version of (100,150,200): fg= 100+int(155*0.35)=154, 150+int(105*0.35)=186, 200+int(55*0.35)=219
	if !strings.Contains(result, "154") {
		t.Errorf("expected lightened fg R=154 after mid-selection SGR, got %q", result)
	}
}

func TestApplyHighlightToLine_PadBeyondContent(t *testing.T) {
	// Regression test for issue 1: endCol beyond line content should pad with spaces.
	line := "abc"
	result := applyHighlightToLine(line, 0, 9, 0.35) // endCol=9, but line is only 3 chars

	// Count spaces — should have 7 padding spaces (for cols 3-9).
	plainResult := stripANSI(result)
	if len(plainResult) != 10 { // "abc" + 7 spaces
		t.Errorf("expected 10 visible chars (3 + 7 padding), got %d: %q", len(plainResult), plainResult)
	}

	// The padding should be highlighted spaces.
	if !strings.HasSuffix(plainResult, "       ") { // 7 spaces
		t.Errorf("expected trailing spaces for padding, got %q", plainResult)
	}
}

func TestApplyHighlightToLine_256Color(t *testing.T) {
	// 256-color fg index 196 = (255,0,0)
	line := "\x1b[38;5;196mred"
	result := applyHighlightToLine(line, 0, 2, 0.35)

	// State should track the 256-color as RGB(255,0,0).
	// Lightened: 255+int(0*0.35)=255, 0+int(255*0.35)=89, 0+int(255*0.35)=89
	if !strings.Contains(result, "255") && !strings.Contains(result, "89") {
		t.Errorf("expected lightened 256-color in highlight, got %q", result)
	}
}

func TestApplyHighlightToLine_24BitColor(t *testing.T) {
	// 24-bit bg color
	line := "\x1b[48;2;50;100;150mtext"
	result := applyHighlightToLine(line, 0, 3, 0.35)

	// Lightened bg: 50+int(205*0.35)=121, 100+int(155*0.35)=154, 150+int(105*0.35)=186
	if !strings.Contains(result, "121") {
		t.Errorf("expected lightened bg R=121, got %q", result)
	}
	if !strings.Contains(result, "186") {
		t.Errorf("expected lightened bg B=186, got %q", result)
	}
}

func TestApplyHighlightToLine_PartialSelection(t *testing.T) {
	// Only highlight columns 2-4 of "abcdefgh"
	line := "abcdefgh"
	result := applyHighlightToLine(line, 2, 4, 0.35)
	plain := stripANSI(result)

	// Plain text should be unchanged.
	if plain != "abcdefgh" {
		t.Errorf("expected plain text unchanged, got %q", plain)
	}
}

func TestApplyHighlightToLine_EmptyLine(t *testing.T) {
	// Highlighting an empty line with endCol beyond content should pad.
	result := applyHighlightToLine("", 0, 4, 0.35)
	plain := stripANSI(result)
	if len(plain) != 5 { // 5 spaces for cols 0-4
		t.Errorf("expected 5 padding spaces, got %d: %q", len(plain), plain)
	}
}

func TestApplyHighlightToLine_OSCPassthrough(t *testing.T) {
	// OSC sequence should be passed through without affecting highlight.
	line := "\x1b]8;;https://example.com\x07link text"
	result := applyHighlightToLine(line, 0, 3, 0.35)

	if !strings.Contains(result, "\x1b]8;;https://example.com\x07") {
		t.Errorf("expected OSC sequence passed through, got %q", result)
	}
	if !strings.Contains(result, "link") {
		t.Errorf("expected 'link' in result, got %q", result)
	}
}

func TestApplyHighlightToLine_NonCSIEscape(t *testing.T) {
	// Non-CSI escape (charset) should be passed through.
	line := "\x1b(0abc"
	result := applyHighlightToLine(line, 0, 2, 0.35)

	if !strings.Contains(result, "\x1b(0") {
		t.Errorf("expected charset escape passed through, got %q", result)
	}
}

func TestEmitHighlightSGR_Defaults(t *testing.T) {
	// With no colors set, should use defaults: fg=229,229,229 bg=0,0,0
	var state ansiColorState
	sgr := emitHighlightSGR(&state, 0.35)

	// Default fg (229,229,229) lightened: 229+int(26*0.35)=238
	if !strings.Contains(sgr, "238") {
		t.Errorf("expected lightened default fg, got %q", sgr)
	}

	// Default bg (0,0,0) lightened: 0+int(255*0.35)=89
	if !strings.Contains(sgr, "89") {
		t.Errorf("expected lightened default bg, got %q", sgr)
	}
}

func TestEmitRestoreSGR_WithColors(t *testing.T) {
	state := ansiColorState{
		fgSet: true, fgR: 100, fgG: 200, fgB: 50,
		bgSet: true, bgR: 10, bgG: 20, bgB: 30,
	}
	sgr := emitRestoreSGR(&state)

	if !strings.Contains(sgr, "\x1b[38;2;100;200;50m") {
		t.Errorf("expected fg restore, got %q", sgr)
	}
	if !strings.Contains(sgr, "\x1b[48;2;10;20;30m") {
		t.Errorf("expected bg restore, got %q", sgr)
	}
}

func TestEmitRestoreSGR_Defaults(t *testing.T) {
	var state ansiColorState
	sgr := emitRestoreSGR(&state)

	if !strings.Contains(sgr, "\x1b[39m") {
		t.Errorf("expected default fg restore (code 39), got %q", sgr)
	}
	if !strings.Contains(sgr, "\x1b[49m") {
		t.Errorf("expected default bg restore (code 49), got %q", sgr)
	}
}
