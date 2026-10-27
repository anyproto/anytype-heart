package mill

import (
	"path/filepath"
	"strings"
)

// MPEG-4 audio shares the ftyp container with video, so content sniffing reports
// video/mp4 for audio files whose brand is generic (isom, mp42, ...).
var mp4AudioExts = map[string]struct{}{
	".m4a": {},
	".m4b": {},
}

// FixMIMEByExt corrects a sniffed MIME type using the file extension when sniffing is known to be ambiguous.
func FixMIMEByExt(name, mime string) string {
	if mime != "video/mp4" {
		return mime
	}
	if _, ok := mp4AudioExts[strings.ToLower(filepath.Ext(name))]; ok {
		return "audio/mp4"
	}
	return mime
}
