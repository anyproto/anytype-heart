package strutil

import (
	"strings"
	"unicode"
)

const maxInt = int(^uint(0) >> 1)

func CapitalizeFirstLetter(str string) string {
	var runes = []rune(str)
	runes = append([]rune{unicode.ToUpper(runes[0])}, runes[1:]...)
	return string(runes)
}

// JoinWithTrailingEnd copy of strings.Join
func JoinWithTrailingEnd(elems []string, sep string) string {
	switch len(elems) {
	case 0:
		return ""
	case 1:
		return elems[0] + sep
	}

	var n int
	if len(sep) > 0 {
		if len(sep) >= maxInt/(len(elems)-1) {
			panic("strings: Join output length overflow")
		}
		n += len(sep) * (len(elems) - 1)
	}
	for _, elem := range elems {
		if len(elem) > maxInt-n {
			panic("strings: Join output length overflow")
		}
		n += len(elem)
	}

	var builder strings.Builder
	builder.Grow(n)
	for _, s := range elems {
		builder.WriteString(s)
		builder.WriteString(sep)
	}
	return builder.String()
}
