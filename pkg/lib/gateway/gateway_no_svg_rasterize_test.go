//go:build !rasterizesvg

package gateway

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/fileobject"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
)

func TestGetImage_SVG(t *testing.T) {
	t.Run("svg image", func(t *testing.T) {
		// given
		fx := newFixture(t)

		const imageData = "image data"
		const fileObjectId = "fileObjectId"
		fullFileId := domain.FullFileId{
			SpaceId: "space1",
			FileId:  "fileId1",
		}

		fx.fileObjectService.EXPECT().DoFileWaitLoad(mock.Anything, fileObjectId, func(_ fileobject.FileObject) error {
			return nil
		}).Return(nil)

		file := mock_files.NewMockFile(t)
		file.EXPECT().Reader(mock.Anything).Return(strings.NewReader(imageData), nil)
		file.EXPECT().Name().Return("image.svg")
		file.EXPECT().Meta().Return(&files.FileMeta{
			Name:  info.Name,
			Media: info.Media,
		})

		image := mock_files.NewMockImage(t)
		image.EXPECT().GetOriginalFile().Return(file, nil)
		fx.fileService.EXPECT().ImageByHash(mock.Anything, fullFileId).Return(image, nil)

		path := "http://" + fx.Addr() + "/image/" + fileObjectId

		// then
		resp, err := http.Get(path)

		// when
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "image/svg+xml", info.Media)
		assert.Equal(t, "inline; filename=\"image.svg\"", resp.Header.Get("Content-Disposition"))

		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, imageData, string(data))
	})
}
