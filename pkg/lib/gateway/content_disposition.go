package gateway

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

// attachmentParam asks the gateway to serve a file as a download rather than
// something to render in place. The save-to-disk flow sets it; image and
// preview rendering leaves it off.
const attachmentParam = "attachment"

// fallbackFilename names a file whose own name survives nothing of the
// sanitizing below — the real name still travels in the filename* parameter.
const fallbackFilename = "file"

// rfc5987AttrChars are the characters RFC 5987 lets an ext-value carry
// literally. Everything else is percent-encoded.
const rfc5987AttrChars = "!#$&+-.^_`|~"

// maxFilenameBytes caps the name both parameters are built from. No filesystem
// takes a longer element, and an uncapped name is a response-header problem
// before it is a filename problem: percent-encoding triples every non-ASCII
// byte, so a 40k-character name produced a 280KB header, past the 256KB cap
// browsers enforce — the download then fails for a reason that looks nothing
// like a bad name.
const maxFilenameBytes = 255

// attachmentRequested reports whether the caller asked for a download. A bare
// ?attachment counts, so a hand-written URL does the obvious thing.
func attachmentRequested(r *http.Request) bool {
	values, ok := r.URL.Query()[attachmentParam]
	if !ok {
		return false
	}
	if len(values) == 0 || values[0] == "" {
		return true
	}
	requested, err := strconv.ParseBool(values[0])
	return err == nil && requested
}

// contentDisposition builds an RFC 6266 Content-Disposition value for a file
// name that came from file metadata — that is, from whoever shared the file.
//
// The name reaches us as arbitrary text, so it cannot go into the header as-is:
// a quote or backslash would break out of the quoted form, a control character
// would corrupt the header, and raw non-ASCII bytes are not valid in a header
// value and arrive mangled. So the quoted filename carries a sanitized ASCII
// fallback, and whenever that differs from the real name, filename* carries the
// name itself, percent-encoded as UTF-8. Clients prefer filename*; the ones
// that do not still get something sane.
func contentDisposition(disposition, name string) string {
	name = sanitizeFilename(name)
	fallback := asciiFilename(name)
	switch fallback {
	case "":
		// Nothing of the name survived, so there is no real name worth
		// advertising either — an empty filename* helps no client.
		return fmt.Sprintf("%s; filename=%q", disposition, fallbackFilename)
	case name:
		return fmt.Sprintf("%s; filename=%q", disposition, fallback)
	default:
		return fmt.Sprintf("%s; filename=%q; filename*=UTF-8''%s", disposition, fallback, encodeRFC5987(name))
	}
}

// sanitizeFilename reduces an untrusted name to a single, bounded path element,
// before either parameter is built from it.
//
// Both parameters are advisory, but a client that saves under the name we hand
// it should not be handed a path: the write path already confines the name with
// filepath.Base, and a header saying "../../../.bashrc" invites exactly what
// that guards against. Control characters go here rather than only in the ASCII
// fallback, so a NUL cannot ride through filename* percent-encoded either.
func sanitizeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)

	// Both separators, on every platform: the name came from whoever shared the
	// file, not from this filesystem.
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	if name == "." || name == ".." {
		return ""
	}
	return capFilename(name)
}

// capFilename bounds the name to maxFilenameBytes, keeping the extension, which
// is the part a client needs to open the file with the right application.
func capFilename(name string) string {
	if len(name) <= maxFilenameBytes {
		return name
	}

	ext := filepath.Ext(name)
	if len(ext) > maxFilenameBytes/2 {
		// Not an extension so much as the tail of a long name.
		ext = ""
	}
	return truncateToBytes(strings.TrimSuffix(name, ext), maxFilenameBytes-len(ext)) + ext
}

// truncateToBytes cuts s to at most limit bytes without splitting a rune.
func truncateToBytes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	for i := range s {
		if i > limit {
			return s[:lastRuneBoundary(s, limit)]
		}
	}
	return s[:lastRuneBoundary(s, limit)]
}

func lastRuneBoundary(s string, limit int) int {
	end := limit
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return end
}

// asciiFilename reduces a name to something safe inside a quoted-string: one
// underscore per character it cannot represent. It works per rune, so a
// non-ASCII name does not swell into a run of underscores, one per UTF-8 byte.
// An empty result means nothing of the name survived; naming it is the caller's
// decision. Control characters and separators are already gone — see
// sanitizeFilename.
func asciiFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r > 0x7f, r == '"', r == '\\':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// encodeRFC5987 percent-encodes a name for the filename* parameter, over UTF-8
// bytes as the RFC requires.
func encodeRFC5987(name string) string {
	var b strings.Builder
	for _, c := range []byte(name) {
		if isAlphaNum(c) || strings.IndexByte(rfc5987AttrChars, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

func isAlphaNum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
