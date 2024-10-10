//go:build rasterizesvg

package svg

import (
	"bytes"
	"context"
	"image"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/files/mock_files"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
)

func TestProcessSvg(t *testing.T) {
	t.Run("process svg successful", func(t *testing.T) {
		// given
		svgContent := []byte(`<svg viewBox="0 0 100 100">
		<circle cx="50" cy="50" r="40" stroke="black" stroke-width="3" fill="red" />
	</svg>`)
		file := mock_files.NewMockFile(t)
		file.EXPECT().Reader(context.Background()).Return(bytes.NewReader(svgContent), nil)
		fileInfo := &storage.FileInfo{}
		file.EXPECT().Info().Return(fileInfo)

		// when
		result, err := ProcessSvg(context.Background(), file)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, result)

		assert.Equal(t, pngMedia, fileInfo.Media)

		buf := new(bytes.Buffer)
		buf.ReadFrom(result)
		img, _, err := image.Decode(buf)
		assert.NoError(t, err)
		assert.NotNil(t, img)
	})
}
