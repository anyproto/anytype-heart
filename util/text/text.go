package text

import (
	"unicode"
	"unicode/utf8"
)

const TruncateEllipsis = " …"

func Truncate(text string, length int) string {
	if utf8.RuneCountInString(text) <= length {
		return text
	}
	var lastWordIndex, lastNonSpace, currentLen, endTextPos int
	for i, r := range text {
		currentLen++
		if unicode.IsSpace(r) {
			lastWordIndex = lastNonSpace
		} else if unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana) {
			lastWordIndex = i
		} else {
			lastNonSpace = i + utf8.RuneLen(r)
		}
		if currentLen > length {
			if lastWordIndex == 0 {
				endTextPos = i
			} else {
				endTextPos = lastWordIndex
			}
			out := text[0:endTextPos]
			return out + TruncateEllipsis
		}
	}
	return text
}
