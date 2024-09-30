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
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestFiles(t *testing.T) {
	ctx := context.Background()
	app := createAccountAndStartApp(t, pb.RpcObjectImportUseCaseRequest_GET_STARTED)

	t.Run("upload image", func(t *testing.T) {
		blockService := getService[*block.Service](app)
		objectId, details, err := blockService.UploadFile(ctx, app.personalSpaceId(), block.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{
				LocalPath: "./testdata/test_image.png",
			},
		})

		require.NoError(t, err)
		require.NotEmpty(t, objectId)

		fileId := pbtypes.GetString(details, bundle.RelationKeyFileId.String())
		assert.Equal(t, "test_image", pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.NotEmpty(t, fileId)
		assert.NotEmpty(t, pbtypes.GetString(details, bundle.RelationKeyFileMimeType.String()))
		assert.True(t, pbtypes.GetInt64(details, bundle.RelationKeySizeInBytes.String()) > 0)

		// Image is available either by object ID or file ID
		assertImageAvailableInGateway(t, app, objectId)
		assertImageAvailableInGateway(t, app, fileId)
	})

	t.Run("upload file", func(t *testing.T) {
		blockService := getService[*block.Service](app)
		objectId, details, err := blockService.UploadFile(ctx, app.personalSpaceId(), block.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{
				LocalPath: "./files_test.go", // Upload itself :)
			},
		})

		require.NoError(t, err)
		require.NotEmpty(t, objectId)

		fileId := pbtypes.GetString(details, bundle.RelationKeyFileId.String())
		assert.Equal(t, "files_test", pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.NotEmpty(t, fileId)
		assert.True(t, pbtypes.GetInt64(details, bundle.RelationKeySizeInBytes.String()) > 0)

		// File is available either by object ID or file ID
		assertFileAvailableInGateway(t, app, objectId)
		assertFileAvailableInGateway(t, app, fileId)
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
