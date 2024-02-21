package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestFiles(t *testing.T) {
	ctx := context.Background()
	app, acc := createAccountAndStartApp(t)

	t.Run("upload image", func(t *testing.T) {
		blockService := getService[*block.Service](app)
		objectId, details, err := blockService.UploadFile(ctx, acc.Info.AccountSpaceId, block.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{
				LocalPath: "./testdata/test_image.png",
			},
		})

		require.NoError(t, err)
		require.NotEmpty(t, objectId)

		assert.Equal(t, "test_image", pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.NotEmpty(t, pbtypes.GetString(details, bundle.RelationKeyFileId.String()))
		assert.NotEmpty(t, pbtypes.GetString(details, bundle.RelationKeyFileMimeType.String()))
		assert.True(t, pbtypes.GetInt64(details, bundle.RelationKeySizeInBytes.String()) > 0)
	})
}
