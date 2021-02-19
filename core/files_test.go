package core

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/stretchr/testify/require"
)

func TestFile(t *testing.T) {
	_, mw := start(t)
	t.Run("image_should_open_as_object", func(t *testing.T) {
		respUploadImage := mw.UploadFile(&pb.RpcUploadFileRequest{LocalPath: "./block/testdata/testdir/a.jpg"})
		require.Equal(t, 0, int(respUploadImage.Error.Code), respUploadImage.Error.Description)

		respOpenImage := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: respUploadImage.Hash})
		require.Equal(t, 0, int(respOpenImage.Error.Code), respOpenImage.Error.Description)
		require.Len(t, respOpenImage.Event.Messages, 1)
		show := getEventBlockShow(respOpenImage.Event.Messages)
		require.NotNil(t, show)
		require.GreaterOrEqual(t, len(show.Details), 2)
		det := getDetailsForContext(show.Details, respUploadImage.Hash)
		require.Equal(t, "a", pbtypes.GetString(det, "name"))
		require.Equal(t, "image/jpeg", pbtypes.GetString(det, "fileMimeType"))

		b := getBlockById("file", respOpenImage.Event.Messages[0].GetBlockShow().Blocks)
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

}
