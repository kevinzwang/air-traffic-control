package terminal

import "testing"

func TestAddAltModifier(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Plain arrows -> Alt+arrows
		{"Alt+Up", "\x1b[A", "\x1b[1;3A"},
		{"Alt+Down", "\x1b[B", "\x1b[1;3B"},
		{"Alt+Right", "\x1b[C", "\x1b[1;3C"},
		{"Alt+Left", "\x1b[D", "\x1b[1;3D"},

		// Shift+arrows -> Alt+Shift+arrows (2+2=4)
		{"Alt+Shift+Up", "\x1b[1;2A", "\x1b[1;4A"},
		{"Alt+Shift+Down", "\x1b[1;2B", "\x1b[1;4B"},
		{"Alt+Shift+Right", "\x1b[1;2C", "\x1b[1;4C"},
		{"Alt+Shift+Left", "\x1b[1;2D", "\x1b[1;4D"},

		// Ctrl+arrows -> Alt+Ctrl+arrows (5+2=7)
		{"Alt+Ctrl+Up", "\x1b[1;5A", "\x1b[1;7A"},
		{"Alt+Ctrl+Down", "\x1b[1;5B", "\x1b[1;7B"},
		{"Alt+Ctrl+Right", "\x1b[1;5C", "\x1b[1;7C"},
		{"Alt+Ctrl+Left", "\x1b[1;5D", "\x1b[1;7D"},

		// Ctrl+Shift+arrows -> Alt+Ctrl+Shift+arrows (6+2=8)
		{"Alt+Ctrl+Shift+Up", "\x1b[1;6A", "\x1b[1;8A"},
		{"Alt+Ctrl+Shift+Down", "\x1b[1;6B", "\x1b[1;8B"},
		{"Alt+Ctrl+Shift+Right", "\x1b[1;6C", "\x1b[1;8C"},
		{"Alt+Ctrl+Shift+Left", "\x1b[1;6D", "\x1b[1;8D"},

		// Home/End
		{"Alt+Home", "\x1b[H", "\x1b[1;3H"},
		{"Alt+End", "\x1b[F", "\x1b[1;3F"},
		{"Alt+Shift+Home", "\x1b[1;2H", "\x1b[1;4H"},
		{"Alt+Shift+End", "\x1b[1;2F", "\x1b[1;4F"},
		{"Alt+Ctrl+Home", "\x1b[1;5H", "\x1b[1;7H"},
		{"Alt+Ctrl+End", "\x1b[1;5F", "\x1b[1;7F"},

		// Tilde keys (no existing modifier)
		{"Alt+Insert", "\x1b[2~", "\x1b[2;3~"},
		{"Alt+Delete", "\x1b[3~", "\x1b[3;3~"},
		{"Alt+PgUp", "\x1b[5~", "\x1b[5;3~"},
		{"Alt+PgDown", "\x1b[6~", "\x1b[6;3~"},

		// Tilde keys with existing modifier
		{"Alt+Ctrl+PgUp", "\x1b[5;5~", "\x1b[5;7~"},
		{"Alt+Ctrl+PgDown", "\x1b[6;5~", "\x1b[6;7~"},

		// SS3 function keys (F1-F4)
		{"Alt+F1", "\x1bOP", "\x1b[1;3P"},
		{"Alt+F2", "\x1bOQ", "\x1b[1;3Q"},
		{"Alt+F3", "\x1bOR", "\x1b[1;3R"},
		{"Alt+F4", "\x1bOS", "\x1b[1;3S"},

		// Tilde function keys (F5-F12)
		{"Alt+F5", "\x1b[15~", "\x1b[15;3~"},
		{"Alt+F6", "\x1b[17~", "\x1b[17;3~"},
		{"Alt+F7", "\x1b[18~", "\x1b[18;3~"},
		{"Alt+F8", "\x1b[19~", "\x1b[19;3~"},
		{"Alt+F9", "\x1b[20~", "\x1b[20;3~"},
		{"Alt+F10", "\x1b[21~", "\x1b[21;3~"},
		{"Alt+F11", "\x1b[23~", "\x1b[23;3~"},
		{"Alt+F12", "\x1b[24~", "\x1b[24;3~"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addAltModifier(tt.input)
			if got != tt.want {
				t.Errorf("addAltModifier(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
