package strutil

import "unicode"

func CapitalizeFirstLetter(str string) string {
	var runes = []rune(str)
	runes = append([]rune{unicode.ToUpper(runes[0])}, runes[1:]...)
	return string(runes)
}
