//go:build rasterizesvg

package gateway

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
)

func TestGetImage_SVG(t *testing.T) {
	t.Run("svg image", func(t *testing.T) {
		// given
		fx := newFixture(t)

		imageData := []byte(`<svg viewBox="0 0 100 100">
		<circle cx="50" cy="50" r="40" stroke="black" stroke-width="3" fill="red" />
	</svg>`)
		const fileObjectId = "fileObjectId"
		fullFileId := domain.FullFileId{
			SpaceId: "space1",
			FileId:  "fileId1",
		}

		fx.fileObjectService.EXPECT().GetFileIdFromObjectWaitLoad(mock.Anything, fileObjectId).Return(fullFileId, nil)

		file := mock_files.NewMockFile(t)
		file.EXPECT().Reader(mock.Anything).Return(bytes.NewReader(imageData), nil)
		info := &storage.FileInfo{Name: "image.svg"}
		file.EXPECT().Info().Return(info)
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
		assert.Equal(t, "image/png", info.Media)
		assert.Equal(t, "inline; filename=\"image.svg\"", resp.Header.Get("Content-Disposition"))
	})
}
