package terminal

import (
	"unicode/utf8"
)

// DisplayWidth returns the terminal column width of text, ignoring ANSI SGR
// sequences and treating wide East-Asian / emoji runes as two columns.
func DisplayWidth(text string) int {
	width := 0
	inEscape := false
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		i += size
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		if r == 0x1b {
			inEscape = true
			continue
		}
		width += charWidth(r)
	}
	return width
}

func charWidth(codePoint rune) int {
	if codePoint == 0 {
		return 0
	}
	if (codePoint >= 0x0300 && codePoint <= 0x036f) ||
		(codePoint >= 0x200b && codePoint <= 0x200f) ||
		codePoint == 0xfeff {
		return 0
	}
	if (codePoint >= 0x1100 && codePoint <= 0x115f) ||
		(codePoint >= 0x2e80 && codePoint <= 0x303e) ||
		(codePoint >= 0x3041 && codePoint <= 0x33ff) ||
		(codePoint >= 0x3400 && codePoint <= 0x4dbf) ||
		(codePoint >= 0x4e00 && codePoint <= 0x9fff) ||
		(codePoint >= 0xa000 && codePoint <= 0xa4cf) ||
		(codePoint >= 0xac00 && codePoint <= 0xd7a3) ||
		(codePoint >= 0xf900 && codePoint <= 0xfaff) ||
		(codePoint >= 0xfe30 && codePoint <= 0xfe4f) ||
		(codePoint >= 0xff00 && codePoint <= 0xff60) ||
		(codePoint >= 0xffe0 && codePoint <= 0xffe6) ||
		(codePoint >= 0x1f300 && codePoint <= 0x1faff) ||
		(codePoint >= 0x20000 && codePoint <= 0x3fffd) {
		return 2
	}
	return 1
}
