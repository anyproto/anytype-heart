package core

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/stretchr/testify/require"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

type Metrics struct {
	NumSST    int
	SizeSSTs  int64
	NumVLOG   int
	SizeVLOGs int64
}

func TestFile(t *testing.T) {
	rootPath, mw, close := start(t, nil)
	defer close()
	getMetrics := func(path string) (m Metrics, err error) {
		err = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			ext := filepath.Ext(info.Name())
			switch ext {
			case ".sst":
				m.NumSST++
				m.SizeSSTs += info.Size() / 1024
			case ".vlog":
				m.NumVLOG++
				m.SizeVLOGs += info.Size() / 1024
			}
			return nil
		})
		return
	}

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
		require.Equal(t, uint64(168368), bytesRemoved)
	})

	t.Run("offload_all", func(t *testing.T) {
		if os.Getenv("ANYTYPE_TEST_FULL") != "1" {
			//	return
		}
		coreService := mw.app.MustComponent(core.CName).(core.Service)
		for i := 1; i <= 200; i++ {
			r := rand.New(rand.NewSource(int64(i)))
			// lets the file not fit in the single block
			b := make([]byte, 1024*1024*3)
			r.Read(b)

			f, err := coreService.FileAdd(context.Background(), files.WithBytes(b))
			require.NoError(t, err)
			require.Equal(t, int64(1024*1024*3), f.Meta().Size)
		}
		m, err := getMetrics(filepath.Join(rootPath, coreService.Account(), "ipfslite"))
		require.NoError(t, err)
		require.Equal(t, 10, m.NumVLOG)
		fmt.Printf("BADGER METRICS AFTER ADD: %+v\n", m)
		resp := mw.FileListOffload(&pb.RpcFileListOffloadRequest{IncludeNotPinned: true})
		require.Equal(t, 0, int(resp.Error.Code), resp.Error.Description)
		require.Equal(t, uint64(1024*1024*3*200+247400), resp.BytesOffloaded) // 247400 is the overhead for the links and meta
		require.Equal(t, int32(200), resp.FilesOffloaded)

		m, err = getMetrics(filepath.Join(rootPath, coreService.Account(), "ipfslite"))
		require.NoError(t, err)
		fmt.Printf("BADGER METRICS AFTER OFFLOAD: %+v\n", m)
		require.Equal(t, 2, m.NumVLOG)

	})

}
