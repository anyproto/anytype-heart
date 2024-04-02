package files

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestGetImageForWidth(t *testing.T) {
	fx := newFixture(t)
	res := testAddImage(t, fx)

	fullId := domain.FullFileId{
		SpaceId: spaceId,
		FileId:  res.FileId,
	}
	ctx := context.Background()

	img, err := fx.ImageByHash(ctx, fullId)
	require.NoError(t, err)

	for _, testCase := range []struct {
		name           string
		requestedWidth int
		expectedWidth  int64
	}{
		{
			name:           "original",
			requestedWidth: 1920,
			expectedWidth:  1024,
		},
		{
			name:           "large",
			requestedWidth: 1920,
			expectedWidth:  1024,
		},
		{
			name:           "small",
			requestedWidth: 320,
			expectedWidth:  320,
		},
		{
			name:           "thumbnail",
			requestedWidth: 100,
			expectedWidth:  100,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			file, err := img.GetFileForWidth(ctx, testCase.requestedWidth)
			require.NoError(t, err)

			assertWidth(t, file, testCase.expectedWidth)
		})
	}
}

func assertWidth(t *testing.T, file File, width int64) {
	require.NotNil(t, file)
	require.NotNil(t, file.Info().GetMeta())

	meta := file.Info().GetMeta()

	assert.Equal(t, width, pbtypes.GetInt64(meta, "width"))
}
