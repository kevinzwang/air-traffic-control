package tui

import (
	"strconv"
	"strings"
)

// dimANSIColors walks an ANSI-colored string and reduces the brightness of
// every color by the given factor (0.0–1.0). Non-color SGR attributes (bold,
// italic, underline, …) and non-SGR escape sequences (cursor movement, etc.)
// are passed through unchanged.
func dimANSIColors(s string, factor float64) string {
	if len(s) == 0 {
		return s
	}

	// Dim default foreground: rgb(91,100,109)
	const dimDefault = "\x1b[38;2;91;100;109m"

	var out strings.Builder
	out.Grow(len(s) + 64)

	// Start with dim default foreground so plain text is also dimmed.
	out.WriteString(dimDefault)

	i := 0
	for i < len(s) {
		if s[i] == '\n' {
			// Re-emit dim default after each newline so that
			// lipgloss.JoinHorizontal (which splits on \n and
			// concatenates each line with the sidebar) doesn't
			// leave us inheriting the sidebar's ANSI reset state.
			out.WriteByte('\n')
			out.WriteString(dimDefault)
			i++
			continue
		}

		if s[i] != '\x1b' {
			out.WriteByte(s[i])
			i++
			continue
		}

		// Found ESC — find the end of the escape sequence.
		start := i
		i++ // skip ESC

		if i >= len(s) {
			out.WriteByte(s[start])
			break
		}

		if s[i] == ']' {
			// OSC sequence (ESC ] ... BEL/ST). Pass through.
			i++ // skip ']'
			for i < len(s) {
				if s[i] == '\x07' {
					i++ // include BEL terminator
					break
				}
				if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
					i += 2 // include ST (ESC \)
					break
				}
				i++
			}
			out.WriteString(s[start:i])
			continue
		}

		if s[i] != '[' {
			// Non-CSI escape sequence. Per ECMA-48:
			//   - Intermediate bytes are 0x20–0x2F
			//   - Final byte is 0x30–0x7E
			// Examples: ESC ( B (charset), ESC 7 (save cursor), ESC ( 0 (graphics)
			if s[i] >= 0x20 && s[i] <= 0x2F {
				// Skip intermediate bytes, then consume one final byte.
				for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2F {
					i++
				}
				if i < len(s) {
					i++ // final byte
				}
			} else {
				// Single final byte (e.g. ESC 7, ESC 8, ESC =, ESC >).
				i++
			}
			out.WriteString(s[start:i])
			continue
		}

		i++ // skip '['

		// Collect the CSI parameter bytes + intermediate bytes.
		paramStart := i
		for i < len(s) && s[i] >= 0x20 && s[i] <= 0x3F {
			i++
		}

		if i >= len(s) {
			// Truncated sequence — emit as-is.
			out.WriteString(s[start:i])
			break
		}

		finalByte := s[i]
		i++ // skip final byte

		if finalByte != 'm' {
			// Non-SGR CSI sequence (cursor movement, erase, etc.) — pass through.
			out.WriteString(s[start:i])
			continue
		}

		// SGR sequence: parse and transform colors.
		paramStr := s[paramStart : i-1] // everything between '[' and 'm'
		transformed := transformSGR(paramStr, factor)
		out.WriteString("\x1b[")
		out.WriteString(transformed)
		out.WriteByte('m')
	}

	return out.String()
}

// transformSGR takes the parameter portion of an SGR sequence (e.g. "38;2;255;0;0")
// and returns a transformed version with dimmed colors.
func transformSGR(params string, factor float64) string {
	if params == "" {
		// ESC[m is equivalent to ESC[0m (reset).
		return "0;38;2;91;100;109"
	}

	parts := strings.Split(params, ";")
	var out []string
	i := 0
	for i < len(parts) {
		p := parts[i]
		code, err := strconv.Atoi(p)
		if err != nil {
			// Non-numeric param — pass through.
			out = append(out, p)
			i++
			continue
		}

		switch {
		case code == 0:
			// Reset — emit reset + re-apply dim default foreground.
			out = append(out, "0", "38", "2", "91", "100", "109")
			i++

		case code == 39:
			// Default foreground — replace with dim default.
			out = append(out, "38", "2", "91", "100", "109")
			i++

		case code == 49:
			// Default background — pass through.
			out = append(out, p)
			i++

		case (code == 38 || code == 48) && i+1 < len(parts):
			// Extended color.
			next, _ := strconv.Atoi(parts[i+1])
			if next == 2 && i+4 < len(parts) {
				// 24-bit: 38;2;R;G;B or 48;2;R;G;B
				r, _ := strconv.Atoi(parts[i+2])
				g, _ := strconv.Atoi(parts[i+3])
				b, _ := strconv.Atoi(parts[i+4])
				r, g, b = dimRGB(r, g, b, factor)
				out = append(out, p, "2",
					strconv.Itoa(r), strconv.Itoa(g), strconv.Itoa(b))
				i += 5
			} else if next == 5 && i+2 < len(parts) {
				// 256-color: 38;5;N or 48;5;N — convert to 24-bit dimmed.
				n, _ := strconv.Atoi(parts[i+2])
				r, g, b := color256ToRGB(n)
				r, g, b = dimRGB(r, g, b, factor)
				out = append(out, p, "2",
					strconv.Itoa(r), strconv.Itoa(g), strconv.Itoa(b))
				i += 3
			} else {
				out = append(out, p)
				i++
			}

		case (code >= 30 && code <= 37):
			// Basic foreground (30-37).
			r, g, b := ansi16Colors[code-30][0], ansi16Colors[code-30][1], ansi16Colors[code-30][2]
			r, g, b = dimRGB(r, g, b, factor)
			out = append(out, "38", "2",
				strconv.Itoa(r), strconv.Itoa(g), strconv.Itoa(b))
			i++

		case (code >= 40 && code <= 47):
			// Basic background (40-47).
			r, g, b := ansi16Colors[code-40][0], ansi16Colors[code-40][1], ansi16Colors[code-40][2]
			r, g, b = dimRGB(r, g, b, factor)
			out = append(out, "48", "2",
				strconv.Itoa(r), strconv.Itoa(g), strconv.Itoa(b))
			i++

		case (code >= 90 && code <= 97):
			// Bright foreground (90-97).
			r, g, b := ansi16Colors[code-90+8][0], ansi16Colors[code-90+8][1], ansi16Colors[code-90+8][2]
			r, g, b = dimRGB(r, g, b, factor)
			out = append(out, "38", "2",
				strconv.Itoa(r), strconv.Itoa(g), strconv.Itoa(b))
			i++

		case (code >= 100 && code <= 107):
			// Bright background (100-107).
			r, g, b := ansi16Colors[code-100+8][0], ansi16Colors[code-100+8][1], ansi16Colors[code-100+8][2]
			r, g, b = dimRGB(r, g, b, factor)
			out = append(out, "48", "2",
				strconv.Itoa(r), strconv.Itoa(g), strconv.Itoa(b))
			i++

		default:
			// Non-color attribute (bold, italic, underline, etc.) — pass through.
			out = append(out, p)
			i++
		}
	}

	return strings.Join(out, ";")
}

func dimRGB(r, g, b int, factor float64) (int, int, int) {
	return int(float64(r) * factor),
		int(float64(g) * factor),
		int(float64(b) * factor)
}

// color256ToRGB converts a 256-color index to RGB.
func color256ToRGB(n int) (int, int, int) {
	if n < 0 || n > 255 {
		return 0, 0, 0
	}
	if n < 16 {
		return ansi16Colors[n][0], ansi16Colors[n][1], ansi16Colors[n][2]
	}
	if n < 232 {
		// 6x6x6 color cube: indices 16–231.
		n -= 16
		b := n % 6
		g := (n / 6) % 6
		r := n / 36
		return cubeValue(r), cubeValue(g), cubeValue(b)
	}
	// Grayscale ramp: indices 232–255.
	v := 8 + (n-232)*10
	return v, v, v
}

func cubeValue(i int) int {
	if i == 0 {
		return 0
	}
	return 55 + i*40
}

// ansi16Colors maps the standard 16 ANSI colors to RGB values (xterm defaults).
var ansi16Colors = [16][3]int{
	{0, 0, 0},       // 0: Black
	{205, 0, 0},     // 1: Red
	{0, 205, 0},     // 2: Green
	{205, 205, 0},   // 3: Yellow
	{0, 0, 238},     // 4: Blue
	{205, 0, 205},   // 5: Magenta
	{0, 205, 205},   // 6: Cyan
	{229, 229, 229}, // 7: White
	{127, 127, 127}, // 8: Bright Black (Gray)
	{255, 0, 0},     // 9: Bright Red
	{0, 255, 0},     // 10: Bright Green
	{255, 255, 0},   // 11: Bright Yellow
	{92, 92, 255},   // 12: Bright Blue
	{255, 0, 255},   // 13: Bright Magenta
	{0, 255, 255},   // 14: Bright Cyan
	{255, 255, 255}, // 15: Bright White
}
