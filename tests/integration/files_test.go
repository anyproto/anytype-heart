package integration

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/gateway"
)

func TestFiles(t *testing.T) {
	ctx := context.Background()
	app := createAccountAndStartApp(t, pb.RpcObjectImportUseCaseRequest_GET_STARTED)

	t.Run("upload image", func(t *testing.T) {
		blockService := getService[*block.Service](app)
		objectId, _, details, err := blockService.UploadFile(ctx, app.personalSpaceId(), block.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{
				LocalPath: "./testdata/test_image.png",
			},
		})

		require.NoError(t, err)
		require.NotEmpty(t, objectId)

		fileId := details.GetString(bundle.RelationKeyFileId)
		assert.Equal(t, "test_image", details.GetString(bundle.RelationKeyName))
		assert.NotEmpty(t, fileId)
		assert.NotEmpty(t, details.GetString(bundle.RelationKeyFileMimeType))
		assert.True(t, details.GetInt64(bundle.RelationKeySizeInBytes) > 0)

		assertImageAvailableInGateway(t, app, objectId)
	})

	t.Run("upload file", func(t *testing.T) {
		blockService := getService[*block.Service](app)
		objectId, _, details, err := blockService.UploadFile(ctx, app.personalSpaceId(), block.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{
				LocalPath: "./files_test.go", // Upload itself :)
			},
		})

		require.NoError(t, err)
		require.NotEmpty(t, objectId)

		fileId := details.GetString(bundle.RelationKeyFileId)
		assert.Equal(t, "files_test", details.GetString(bundle.RelationKeyName))
		assert.NotEmpty(t, fileId)
		assert.True(t, details.GetInt64(bundle.RelationKeySizeInBytes) > 0)

		assertFileAvailableInGateway(t, app, objectId)
	})
}

func assertImageAvailableInGateway(t *testing.T, app *testApplication, id string) {
	assertAvailableInGateway(t, app, "image", id)
}

func assertFileAvailableInGateway(t *testing.T, app *testApplication, id string) {
	assertAvailableInGateway(t, app, "file", id)
}

func assertAvailableInGateway(t *testing.T, app *testApplication, method string, id string) {
	gw := getService[gateway.Gateway](app)
	host := gw.Addr()
	resp, err := http.Get("http://" + host + "/" + method + "/" + id)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.True(t, len(raw) > 0)
}
