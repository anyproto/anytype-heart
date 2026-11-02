package files

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ftypHeader(brand string) []byte {
	box := []byte{0x00, 0x00, 0x00, 0x18}
	box = append(box, "ftyp"...)
	box = append(box, brand...)
	box = append(box, 0x00, 0x00, 0x02, 0x00)
	box = append(box, brand...)
	box = append(box, "mp41"...)
	return append(box, make([]byte, 64)...)
}

func TestNormalizeOptionsMedia(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		brand    string
		expected string
	}{
		{"m4a with generic isom brand", "song.m4a", "isom", "audio/mp4"},
		{"m4b with generic mp42 brand", "book.m4b", "mp42", "audio/mp4"},
		{"mp4 with isom brand stays video", "clip.mp4", "isom", "video/mp4"},
		{"m4a with M4A brand", "song.m4a", "M4A ", "audio/x-m4a"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := &AddOptions{
				Reader: bytes.NewReader(ftypHeader(tc.brand)),
				Name:   tc.fileName,
			}

			err := (&service{}).normalizeOptions(opts)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, opts.Media)
		})
	}
}
