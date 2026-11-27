package files

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	extRe = regexp.MustCompile(`^\.[A-Za-z0-9]{1,16}$`)
	// device names Windows keeps in every folder, whatever follows a dot
	reservedRe = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[0-9¹²³]|lpt[0-9¹²³]|conin\$|conout\$)$`)
)

// extensionOf is a name's extension when it looks like one (".pdf",
// ".gz"), else empty.
func extensionOf(name string) string {
	if ext := filepath.Ext(name); extRe.MatchString(ext) && len(ext) < len(name) {
		return ext
	}
	return ""
}

const (
	// file systems allow 255 bytes per name (ext4, APFS) or 255 UTF-16
	// units (NTFS), which bytes bound too
	maxNameBytes = 255
	// room for the collision suffix: " (100)" is 6 bytes, "-xxxxxxxx" 9
	suffixBytes = 10
	// and a short name keeps the whole path within Windows' default limit
	maxNameUnits = 120
)

// unsafeRune reports what no file name may hold on some system: control
// characters, the separators and wildcards Windows reserves, and the
// characters that reorder how a name is displayed.
func unsafeRune(r rune) bool {
	switch {
	case unicode.IsControl(r), r == utf8.RuneError:
		return true
	case strings.ContainsRune(`<>:"/\|?*`, r):
		return true
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069, r == 0xFEFF:
		return true
	}
	return false
}

func trimEdge(r rune) bool { return unicode.IsSpace(r) || r == '.' }

// SafeName is a file name safe on every system: no separators, no device
// names, nothing hidden. The stem is shortened by whole characters so that
// it, the extension and a collision suffix stay within 255 UTF-8 bytes and
// 120 UTF-16 units.
func SafeName(name string) string {
	var b strings.Builder
	replaced := false
	for _, r := range strings.ToValidUTF8(name, "_") {
		if unsafeRune(r) {
			if !replaced {
				b.WriteByte('_')
			}
			replaced = true
			continue
		}
		replaced = false
		b.WriteRune(r)
	}
	cleaned := strings.TrimFunc(b.String(), trimEdge)
	ext := extensionOf(cleaned)
	stem := shorten(strings.TrimSuffix(cleaned, ext), maxNameBytes-suffixBytes-len(ext), maxNameUnits-len(ext))
	stem = strings.TrimRightFunc(stem, trimEdge)
	first, _, _ := strings.Cut(stem, ".")
	if stem == "" || reservedRe.MatchString(strings.TrimSpace(first)) {
		stem = "download"
	}
	return stem + ext
}

// shorten is the longest prefix within both limits that splits no
// character.
func shorten(text string, maxBytes, maxUnits int) string {
	bytes, units := 0, 0
	for i, r := range text {
		size, width := utf8.RuneLen(r), 1
		if r > 0xFFFF {
			width = 2
		}
		if bytes+size > maxBytes || units+width > maxUnits {
			return text[:i]
		}
		bytes += size
		units += width
	}
	return text
}
