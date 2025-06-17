package anymark

import (
	"strings"
	"sync"
)

var protectedRunes = []rune{
	'[', // Markdown link opener
	']', // Markdown link closer
	'*', // Markdown strong opener
	'`', // Markdown code opener
	'_', // Markdown em opener
	'|', // Markdown table column separator
}

// Starting at the first BMP PUA code point (U+E000)
const puaStart = 0xE000

// Generated on init()
var (
	escapeReplacer   *strings.Replacer
	unescapeReplacer *strings.Replacer
	once             sync.Once
)

// buildReplacers constructs two *strings.Replacer instances—one for the
// forward direction (escape) and one for the reverse (unescape).
func buildReplacers() {
	escPairs := make([]string, 0, len(protectedRunes)*2)
	uniPairs := make([]string, 0, len(protectedRunes)*2)

	for i, r := range protectedRunes {
		s := string(r)
		pua := string(rune(puaStart + i))

		escPairs = append(escPairs, s, pua)
		uniPairs = append(uniPairs, pua, s)
	}

	escapeReplacer = strings.NewReplacer(escPairs...)
	unescapeReplacer = strings.NewReplacer(uniPairs...)
}

// Escape replaces each “protected” character with its PUA sentinel.
func Escape(text string) string {
	once.Do(buildReplacers)
	return escapeReplacer.Replace(text)
}

// Unescape restores the original characters.
func Unescape(text string) string {
	once.Do(buildReplacers)
	return unescapeReplacer.Replace(text)
}
