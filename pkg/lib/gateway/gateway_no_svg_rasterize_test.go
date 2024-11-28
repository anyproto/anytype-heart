//go:build !rasterizesvg

package gateway

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/fileobject"
	"github.com/anyproto/anytype-heart/core/block/editor/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
)

func TestGetImage_SVG(t *testing.T) {
	t.Run("svg image", func(t *testing.T) {
		// given
		fx := newFixture(t)

		const imageData = "image data"
		const fileObjectId = "fileObjectId"

		file := mock_files.NewMockFile(t)
		file.EXPECT().Reader(mock.Anything).Return(strings.NewReader(imageData), nil)
		file.EXPECT().Name().Return("image.svg")
		file.EXPECT().Meta().Return(&files.FileMeta{
			Name: "image.svg",
		})

		image := mock_files.NewMockImage(t)
		image.EXPECT().GetOriginalFile().Return(file, nil)

		fx.fileObjectService.EXPECT().DoFileWaitLoad(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, s string, f func(fileobject.FileObject) error) error {
			fileObject := mock_fileobject.NewMockFileObject(t)
			fileObject.EXPECT().GetImage().Return(image)
			return f(fileObject)
		})

		path := "http://" + fx.Addr() + "/image/" + fileObjectId

		// then
		resp, err := http.Get(path)

		// when
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "image/svg+xml", resp.Header.Get("Content-Type"))
		assert.Equal(t, "inline; filename=\"image.svg\"", resp.Header.Get("Content-Disposition"))

		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, imageData, string(data))
	})
}
