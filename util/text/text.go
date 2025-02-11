package text

import (
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

const TruncateEllipsis = " …"

func TruncateEllipsized(text string, length int) string {
	return Truncate(text, length, TruncateEllipsis)
}

func Truncate(str string, maxLen int, ending string) string {
	if isUtf16LengthLessOrEqual(str, maxLen) {
		return str
	}

	maxLen -= UTF16RuneCountString(ending)

	var (
		utf16Len      int
		lastWordIndex int
		lastNonSpace  int
	)

	for i, r := range str {
		runeSize := utf16.RuneLen(r)
		if unicode.IsSpace(r) {
			lastWordIndex = lastNonSpace
		} else {
			lastNonSpace = i + utf8.RuneLen(r)
		}

		utf16Len += runeSize
		if utf16Len > maxLen {
			var runeEnd int
			if lastWordIndex == 0 {
				runeEnd = i
			} else {
				runeEnd = lastWordIndex
			}
			if ending == "" {
				return str[:runeEnd]
			} else {
				return str[:runeEnd] + ending
			}
		}
	}

	return str
}

func isUtf16LengthLessOrEqual(str string, maxLen int) bool {
	var n int
	for _, s := range str {
		n += utf16.RuneLen(s)
		if n > maxLen {
			return false
		}
	}
	return true
}

func UTF16RuneCountString(str string) int {
	var n int
	for _, s := range str {
		n += utf16.RuneLen(s)
	}
	return n
}

func UTF16RuneCount(bStr []byte) int {
	str := string(bStr)
	return UTF16RuneCountString(str)
}

func StrToUTF16(str string) []uint16 {
	return utf16.Encode([]rune(str))
}

func UTF16ToStr(b []uint16) string {
	return string(utf16.Decode(b))
}
