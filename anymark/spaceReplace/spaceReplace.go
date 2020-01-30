package spaceReplace

import (
	"golang.org/x/text/unicode/norm"
	"strings"
)

var (
	// Unicode ditch table.
	unicodeWhitespaceDitch = map[rune]struct{}{
		'\r':     struct{}{},
		'\u200B': struct{}{}, // Zero width space.
		'\u200C': struct{}{}, // Zero width non-joiner.
		'\u200D': struct{}{}, // Zero width joiner.
		'\u2060': struct{}{}, // Word joiner.
		'\uFEFF': struct{}{}, // Zero width no-break.
	}

	// Whitespace replacement character map.
	unicodeWhitespaceRepl = map[rune]struct{}{
		'\u0009': struct{}{}, // Character tabulation (HT.)
		'\u00A0': struct{}{}, // No-break space.
		'\u180E': struct{}{}, // Mongolian vowel separator.
		'\u2000': struct{}{}, // En quad.
		'\u2001': struct{}{}, // Em quad.
		'\u2002': struct{}{}, // En space.
		'\u2003': struct{}{}, // Em space.
		'\u2004': struct{}{}, // Three-per-em space.
		'\u2005': struct{}{}, // Four-per-em space.
		'\u2006': struct{}{}, // Six-per-em space.
		'\u2007': struct{}{}, // Figure space.
		'\u2008': struct{}{}, // Punctuation space.
		'\u2009': struct{}{}, // Thin space.
		'\u200A': struct{}{}, // Hair space.
		'\u2028': struct{}{}, // Line separator.
		'\u2029': struct{}{}, // Paragraph separator.
		'\u202F': struct{}{}, // Narrow no-break space.
		'\u205F': struct{}{}, // Medium mathemtical space.
		'\u3000': struct{}{}, // Ideographic space.
	}

	// Whitespace line replacement character map.
	unicodeWhitespaceLineRepl = map[rune]struct{}{
		'\u0009': struct{}{}, // Character tabulation (HT.)
		'\u000A': struct{}{}, // Line feed.
		'\u0020': struct{}{}, // Space.
		'\u00A0': struct{}{}, // No-break space.
		'\u180E': struct{}{}, // Mongolian vowel separator.
		'\u2000': struct{}{}, // En quad.
		'\u2001': struct{}{}, // Em quad.
		'\u2002': struct{}{}, // En space.
		'\u2003': struct{}{}, // Em space.
		'\u2004': struct{}{}, // Three-per-em space.
		'\u2005': struct{}{}, // Four-per-em space.
		'\u2006': struct{}{}, // Six-per-em space.
		'\u2007': struct{}{}, // Figure space.
		'\u2008': struct{}{}, // Punctuation space.
		'\u2009': struct{}{}, // Thin space.
		'\u200A': struct{}{}, // Hair space.
		'\u2028': struct{}{}, // Line separator.
		'\u2029': struct{}{}, // Paragraph separator.
		'\u202F': struct{}{}, // Narrow no-break space.
		'\u205F': struct{}{}, // Medium mathemtical space.
		'\u3000': struct{}{}, // Ideographic space.
	}
)

// Normalize string.
//
// Normalizes the string to NFC and replaces characters in the provided Unicode
// whitespace character table with regular spaces, and ditches any character
// in the ditch table.
func normalizeString(in string, spaceTbl, ditchTbl map[rune]struct{}) string {
	result := make([]rune, 0, len(in))

	for _, r := range norm.NFC.String(in) {
		if r == '\u000B' || r == '\u000C' {
			// Translate form feed and line tabulation.
			r = '\n'
		}

		if _, ws := spaceTbl[r]; ws {
			result = append(result, ' ')
		} else if _, ditch := ditchTbl[r]; !ditch {
			result = append(result, r)
		}
	}

	return string(result)
}

// Normalize string.
//
// Normalizes the string to NFC and ditches any zero-width whitespace.
func NormalizeString(in string) string {
	return normalizeString(in, map[rune]struct{}{}, unicodeWhitespaceDitch)
}

// Whitespace normalize string.
//
// Normalizes the string to NFC and replaces all non-line-breaking Unicode
// whitespace characters with regular spaces, and ditches any carriage returns.
func WhitespaceNormalizeString(in string) string {
	return normalizeString(in, unicodeWhitespaceRepl, unicodeWhitespaceDitch)
}

// Whitespace normalize line.
//
// Normalizes the string to NFC and replaces all Unicode whitespace characters
// with regular spaces.
func WhitespaceNormalizeLine(in string) string {
	return normalizeString(in, unicodeWhitespaceLineRepl, unicodeWhitespaceDitch)
}

// Trim whitespace normalize line.
//
// Normalizes the string to NFC and replaces all Unicode whitespace characters
// with regular spaces, trimming the final result.
func TrimWhitespaceNormalizeLine(in string) string {
	return strings.TrimSpace(WhitespaceNormalizeLine(in))
}

// Trim single-whitespace normalize line.
//
// Normalizes the string to NFC and replaces all Unicode whitespace characters
// with regular spaces, trimming the final result. Multiple whitespaces are
// combined into one whitespace.
func TrimSingleWhitespaceNormalizeLine(in string) string {
	norm := TrimWhitespaceNormalizeLine(in)
	result := make([]rune, 0, len(norm))

	lastWs := false
	for _, r := range norm {
		if r == ' ' {
			if !lastWs {
				result = append(result, r)
				lastWs = true
			}
		} else {
			lastWs = false
			result = append(result, r)
		}
	}

	return string(result)
}