package mill

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFixMIMEByExt(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		mime     string
		expected string
	}{
		{"m4a sniffed as video", "song.m4a", "video/mp4", "audio/mp4"},
		{"uppercase extension", "SONG.M4A", "video/mp4", "audio/mp4"},
		{"m4b audiobook", "book.m4b", "video/mp4", "audio/mp4"},
		{"mp4 video stays video", "clip.mp4", "video/mp4", "video/mp4"},
		{"m4a already audio", "song.m4a", "audio/x-m4a", "audio/x-m4a"},
		{"m4a with other video mime", "song.m4a", "video/quicktime", "video/quicktime"},
		{"no extension", "song", "video/mp4", "video/mp4"},
		{"empty mime", "song.m4a", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, FixMIMEByExt(tc.fileName, tc.mime))
		})
	}
}
