package tui

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// lightenRGB blends a color toward white by the given factor (0.0–1.0).
// Factor 0.35 makes colors noticeably lighter while staying distinguishable.
func lightenRGB(r, g, b int, factor float64) (int, int, int) {
	return r + int(float64(255-r)*factor),
		g + int(float64(255-g)*factor),
		b + int(float64(255-b)*factor)
}

// ansiColorState tracks the current foreground and background RGB colors
// as we walk through a line containing ANSI escape sequences.
type ansiColorState struct {
	fgSet          bool
	fgR, fgG, fgB int
	bgSet          bool
	bgR, bgG, bgB int
}

// updateColorState parses an SGR parameter string (the part between ESC[ and m)
// and updates the color state accordingly.
func updateColorState(state *ansiColorState, paramStr string) {
	if paramStr == "" {
		// ESC[m is equivalent to ESC[0m (reset).
		state.fgSet = false
		state.bgSet = false
		return
	}

	parts := strings.Split(paramStr, ";")
	i := 0
	for i < len(parts) {
		p := parts[i]
		code, err := strconv.Atoi(p)
		if err != nil {
			i++
			continue
		}

		switch {
		case code == 0:
			// Reset.
			state.fgSet = false
			state.bgSet = false
			i++

		case code == 39:
			// Default foreground.
			state.fgSet = false
			i++

		case code == 49:
			// Default background.
			state.bgSet = false
			i++

		case (code == 38 || code == 48) && i+1 < len(parts):
			next, _ := strconv.Atoi(parts[i+1])
			if next == 2 && i+4 < len(parts) {
				// 24-bit: 38;2;R;G;B or 48;2;R;G;B
				r, _ := strconv.Atoi(parts[i+2])
				g, _ := strconv.Atoi(parts[i+3])
				b, _ := strconv.Atoi(parts[i+4])
				if code == 38 {
					state.fgSet = true
					state.fgR, state.fgG, state.fgB = r, g, b
				} else {
					state.bgSet = true
					state.bgR, state.bgG, state.bgB = r, g, b
				}
				i += 5
			} else if next == 5 && i+2 < len(parts) {
				// 256-color: 38;5;N or 48;5;N
				n, _ := strconv.Atoi(parts[i+2])
				r, g, b := color256ToRGB(n)
				if code == 38 {
					state.fgSet = true
					state.fgR, state.fgG, state.fgB = r, g, b
				} else {
					state.bgSet = true
					state.bgR, state.bgG, state.bgB = r, g, b
				}
				i += 3
			} else {
				i++
			}

		case code >= 30 && code <= 37:
			// Basic foreground (30-37).
			c := ansi16Colors[code-30]
			state.fgSet = true
			state.fgR, state.fgG, state.fgB = c[0], c[1], c[2]
			i++

		case code >= 40 && code <= 47:
			// Basic background (40-47).
			c := ansi16Colors[code-40]
			state.bgSet = true
			state.bgR, state.bgG, state.bgB = c[0], c[1], c[2]
			i++

		case code >= 90 && code <= 97:
			// Bright foreground (90-97).
			c := ansi16Colors[code-90+8]
			state.fgSet = true
			state.fgR, state.fgG, state.fgB = c[0], c[1], c[2]
			i++

		case code >= 100 && code <= 107:
			// Bright background (100-107).
			c := ansi16Colors[code-100+8]
			state.bgSet = true
			state.bgR, state.bgG, state.bgB = c[0], c[1], c[2]
			i++

		default:
			// Non-color attribute (bold, italic, underline, etc.) — skip.
			i++
		}
	}
}

// emitHighlightSGR emits an SGR sequence that sets both fg and bg to lightened
// versions of the current colors. Defaults: fg=229,229,229 bg=0,0,0.
func emitHighlightSGR(state *ansiColorState, factor float64) string {
	fgR, fgG, fgB := 229, 229, 229 // default white fg
	if state.fgSet {
		fgR, fgG, fgB = state.fgR, state.fgG, state.fgB
	}
	bgR, bgG, bgB := 0, 0, 0 // default black bg
	if state.bgSet {
		bgR, bgG, bgB = state.bgR, state.bgG, state.bgB
	}

	fgR, fgG, fgB = lightenRGB(fgR, fgG, fgB, factor)
	bgR, bgG, bgB = lightenRGB(bgR, bgG, bgB, factor)

	return "\x1b[38;2;" + strconv.Itoa(fgR) + ";" + strconv.Itoa(fgG) + ";" + strconv.Itoa(fgB) +
		"m\x1b[48;2;" + strconv.Itoa(bgR) + ";" + strconv.Itoa(bgG) + ";" + strconv.Itoa(bgB) + "m"
}

// emitRestoreSGR restores the original (non-lightened) colors after exiting
// the selection region.
func emitRestoreSGR(state *ansiColorState) string {
	var b strings.Builder
	if state.fgSet {
		b.WriteString("\x1b[38;2;" + strconv.Itoa(state.fgR) + ";" + strconv.Itoa(state.fgG) + ";" + strconv.Itoa(state.fgB) + "m")
	} else {
		b.WriteString("\x1b[39m")
	}
	if state.bgSet {
		b.WriteString("\x1b[48;2;" + strconv.Itoa(state.bgR) + ";" + strconv.Itoa(state.bgG) + ";" + strconv.Itoa(state.bgB) + "m")
	} else {
		b.WriteString("\x1b[49m")
	}
	return b.String()
}

// applyHighlightToLine applies a lighten-based highlight to visible columns
// [startCol, endCol] (inclusive) in a line that may contain ANSI escapes.
// If endCol extends beyond the line content, spaces are padded with highlight.
func applyHighlightToLine(line string, startCol, endCol int, lightenFactor float64) string {
	var out strings.Builder
	out.Grow(len(line) + 128)

	var colorState ansiColorState
	visCol := 0
	inHighlight := false
	i := 0

	for i < len(line) {
		if line[i] == '\x1b' {
			// Found ESC — handle escape sequence.
			start := i
			i++ // skip ESC

			if i >= len(line) {
				out.WriteByte(line[start])
				break
			}

			if line[i] == ']' {
				// OSC sequence (ESC ] ... BEL/ST). Pass through.
				i++ // skip ']'
				for i < len(line) {
					if line[i] == '\x07' {
						i++
						break
					}
					if line[i] == '\x1b' && i+1 < len(line) && line[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
				out.WriteString(line[start:i])
				continue
			}

			if line[i] != '[' {
				// Non-CSI escape (charset, save cursor, etc.).
				if line[i] >= 0x20 && line[i] <= 0x2F {
					for i < len(line) && line[i] >= 0x20 && line[i] <= 0x2F {
						i++
					}
					if i < len(line) {
						i++ // final byte
					}
				} else {
					i++
				}
				out.WriteString(line[start:i])
				continue
			}

			i++ // skip '['

			// Collect CSI parameter + intermediate bytes.
			paramStart := i
			for i < len(line) && line[i] >= 0x20 && line[i] <= 0x3F {
				i++
			}

			if i >= len(line) {
				// Truncated sequence — emit as-is.
				out.WriteString(line[start:i])
				break
			}

			finalByte := line[i]
			i++ // skip final byte

			// Emit the original escape sequence.
			out.WriteString(line[start:i])

			if finalByte == 'm' {
				// SGR sequence — update color state.
				paramStr := line[paramStart : i-1]
				updateColorState(&colorState, paramStr)

				// If inside highlight region, re-emit lightened colors
				// after the original SGR (fixes style-boundary cutoff).
				if inHighlight {
					out.WriteString(emitHighlightSGR(&colorState, lightenFactor))
				}
			}
			continue
		}

		// Visible character.
		if !inHighlight && visCol >= startCol && visCol <= endCol {
			out.WriteString(emitHighlightSGR(&colorState, lightenFactor))
			inHighlight = true
		}

		r, size := utf8.DecodeRuneInString(line[i:])
		out.WriteRune(r)
		i += size
		visCol++

		if inHighlight && visCol > endCol {
			out.WriteString(emitRestoreSGR(&colorState))
			inHighlight = false
		}
	}

	// If endCol extends beyond line content, pad with highlighted spaces.
	if endCol >= visCol {
		if !inHighlight {
			out.WriteString(emitHighlightSGR(&colorState, lightenFactor))
			inHighlight = true
		}
		for visCol <= endCol {
			out.WriteByte(' ')
			visCol++
		}
	}

	if inHighlight {
		out.WriteString(emitRestoreSGR(&colorState))
	}

	return out.String()
}
