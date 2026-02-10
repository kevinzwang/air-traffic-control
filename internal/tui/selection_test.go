package tui

import "testing"

func TestIsWordChar(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'a', true},
		{'z', true},
		{'A', true},
		{'Z', true},
		{'0', true},
		{'9', true},
		{'_', true},
		{' ', false},
		{'\t', false},
		{'.', false},
		{'-', false},
		{'/', false},
		{'(', false},
		{'!', false},
	}
	for _, tt := range tests {
		got := isWordChar(tt.r)
		if got != tt.want {
			t.Errorf("isWordChar(%q) = %v, want %v", tt.r, got, tt.want)
		}
	}
}

func TestWordBoundsAt(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		col       int
		wantStart int
		wantEnd   int
	}{
		{
			name:      "word middle",
			input:     "hello world",
			col:       2,
			wantStart: 0,
			wantEnd:   4,
		},
		{
			name:      "word start",
			input:     "hello world",
			col:       0,
			wantStart: 0,
			wantEnd:   4,
		},
		{
			name:      "word end",
			input:     "hello world",
			col:       4,
			wantStart: 0,
			wantEnd:   4,
		},
		{
			name:      "second word",
			input:     "hello world",
			col:       6,
			wantStart: 6,
			wantEnd:   10,
		},
		{
			name:      "space between words",
			input:     "hello world",
			col:       5,
			wantStart: 5,
			wantEnd:   5,
		},
		{
			name:      "punctuation run",
			input:     "foo::bar",
			col:       3,
			wantStart: 3,
			wantEnd:   4,
		},
		{
			name:      "underscore in word",
			input:     "my_var = 42",
			col:       3,
			wantStart: 0,
			wantEnd:   5,
		},
		{
			name:      "empty string",
			input:     "",
			col:       0,
			wantStart: 0,
			wantEnd:   0,
		},
		{
			name:      "out of bounds column",
			input:     "abc",
			col:       10,
			wantStart: 10,
			wantEnd:   10,
		},
		{
			name:      "negative column",
			input:     "abc",
			col:       -1,
			wantStart: -1,
			wantEnd:   -1,
		},
		{
			name:      "single char word",
			input:     "a b c",
			col:       0,
			wantStart: 0,
			wantEnd:   0,
		},
		{
			name:      "multiple spaces",
			input:     "foo   bar",
			col:       4,
			wantStart: 3,
			wantEnd:   5,
		},
		{
			name:      "punctuation at start",
			input:     "...hello",
			col:       1,
			wantStart: 0,
			wantEnd:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runes := []rune(tt.input)
			start, end := wordBoundsAt(runes, tt.col)
			if start != tt.wantStart || end != tt.wantEnd {
				t.Errorf("wordBoundsAt(%q, %d) = (%d, %d), want (%d, %d)",
					tt.input, tt.col, start, end, tt.wantStart, tt.wantEnd)
			}
		})
	}
}
