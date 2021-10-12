package core

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestFile(t *testing.T) {
	_, mw, close := start(t, nil)
	defer close()
	t.Run("image_should_open_as_object", func(t *testing.T) {
		respUploadImage := mw.UploadFile(&pb.RpcUploadFileRequest{LocalPath: "./block/testdata/testdir/a.jpg"})
		require.Equal(t, 0, int(respUploadImage.Error.Code), respUploadImage.Error.Description)

		respOpenImage := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: respUploadImage.Hash})
		require.Equal(t, 0, int(respOpenImage.Error.Code), respOpenImage.Error.Description)
		require.Len(t, respOpenImage.Event.Messages, 1)
		show := getEventObjectShow(respOpenImage.Event.Messages)
		require.NotNil(t, show)
		require.GreaterOrEqual(t, len(show.Details), 2)
		det := getDetailsForContext(show.Details, respUploadImage.Hash)
		require.Equal(t, "a", pbtypes.GetString(det, "name"))
		require.Equal(t, "image/jpeg", pbtypes.GetString(det, "fileMimeType"))

		b := getBlockById("file", respOpenImage.Event.Messages[0].GetObjectShow().Blocks)
		require.NotNil(t, b)
		require.Equal(t, respUploadImage.Hash, b.GetFile().Hash)
	})

	t.Run("file_should_be_reused", func(t *testing.T) {
		respUploadFile1 := mw.UploadFile(&pb.RpcUploadFileRequest{LocalPath: "./block/testdata/testdir/a/a.txt"})
		require.Equal(t, 0, int(respUploadFile1.Error.Code), respUploadFile1.Error.Description)
		respUploadFile2 := mw.UploadFile(&pb.RpcUploadFileRequest{LocalPath: "./block/testdata/testdir/a/a.txt"})
		require.Equal(t, 0, int(respUploadFile1.Error.Code), respUploadFile1.Error.Description)
		require.Equal(t, respUploadFile1.Hash, respUploadFile2.Hash)
	})

	t.Run("image_should_be_reused", func(t *testing.T) {
		respUploadFile1 := mw.UploadFile(&pb.RpcUploadFileRequest{LocalPath: "./block/testdata/testdir/a.jpg"})
		require.Equal(t, 0, int(respUploadFile1.Error.Code), respUploadFile1.Error.Description)
		respUploadFile2 := mw.UploadFile(&pb.RpcUploadFileRequest{LocalPath: "./block/testdata/testdir/a.jpg"})
		require.Equal(t, 0, int(respUploadFile1.Error.Code), respUploadFile1.Error.Description)
		require.Equal(t, respUploadFile1.Hash, respUploadFile2.Hash)
	})
	t.Run("image_offload", func(t *testing.T) {
		coreService := mw.app.MustComponent(core.CName).(core.Service)
		f, err := os.OpenFile("../pkg/lib/mill/testdata/image.jpeg", os.O_RDONLY, 0600)
		require.NoError(t, err)

		i, err := coreService.ImageAdd(context.Background(), files.WithReader(f))
		require.NoError(t, err)

		bytesRemoved, err := coreService.FileOffload(i.Hash())
		require.NoError(t, err)
		require.Equal(t, uint64(503908), bytesRemoved)
	})

}
