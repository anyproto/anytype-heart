package filecontent

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
)

func TestImageVariants(t *testing.T) {
	svc := mock_apicore.NewMockFileObjectService(t)
	img := mock_files.NewMockImage(t)
	original := mock_files.NewMockFile(t)
	variant := mock_files.NewMockFile(t)
	svc.EXPECT().GetImageData(mock.Anything, "image-object").Return(img, nil)
	img.EXPECT().GetOriginalFile().Return(original, nil)
	img.EXPECT().GetFileForWidth(100).Return(variant, nil).Once()
	original.EXPECT().Name().Return("image.png")
	for file, bytes := range map[*mock_files.MockFile]string{original: "original", variant: "thumbnail"} {
		file.EXPECT().FileId().Return(domain.FileId("image-cid"))
		file.EXPECT().MimeType().Return("image/png")
		file.EXPECT().Meta().Return(&files.FileMeta{Name: "image.png", Media: "image/png"})
		file.EXPECT().Reader(mock.Anything).Return(strings.NewReader(bytes), nil).Once()
	}
	full, err := Get(context.Background(), svc, "image-object", 0)
	require.NoError(t, err)
	thumb, err := Get(context.Background(), svc, "image-object", 100)
	require.NoError(t, err)
	for content, want := range map[*Content]string{full: "original", thumb: "thumbnail"} {
		body, err := io.ReadAll(content.Reader)
		require.NoError(t, err)
		assert.Equal(t, want, string(body))
	}
	assert.NotEqual(t, full.ETag, thumb.ETag, "different representations must not share a validator")
}

func TestSvgProcessing(t *testing.T) {
	svc := mock_apicore.NewMockFileObjectService(t)
	img := mock_files.NewMockImage(t)
	file := mock_files.NewMockFile(t)
	svc.EXPECT().GetImageData(mock.Anything, "svg-object").Return(img, nil).Once()
	img.EXPECT().GetOriginalFile().Return(file, nil).Once()
	file.EXPECT().Name().Return("icon.svg")
	file.EXPECT().FileId().Return(domain.FileId("svg-cid"))
	file.EXPECT().Meta().Return(&files.FileMeta{Name: "icon.svg", Media: "image/svg+xml"})
	const source = `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8" viewBox="0 0 8 8"><rect width="8" height="8" fill="red"/></svg>`
	file.EXPECT().Reader(mock.Anything).Return(strings.NewReader(source), nil).Once()
	content, err := Get(context.Background(), svc, "svg-object", 100)
	require.NoError(t, err)
	body, err := io.ReadAll(content.Reader)
	require.NoError(t, err)
	// The build's rasterizesvg flag chooses whether the SVG pipeline returns
	// a PNG or the original SVG. Both paths bypass raster width variants.
	if content.MimeType == "image/png" {
		assert.True(t, strings.HasPrefix(string(body), "\x89PNG\r\n\x1a\n"))
	} else {
		assert.Equal(t, "image/svg+xml", content.MimeType)
		assert.Equal(t, source, string(body))
	}
	require.NotEmpty(t, content.ETag)
}
