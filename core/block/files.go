package block

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/miolini/datacounter"

	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/anyerror"
)

// TODO Move residual file methods here

// TODO Extract to a new service FileDownloader
func (s *Service) DownloadFile(ctx context.Context, req *pb.RpcFileDownloadRequest) (string, error) {
	if req.Path == "" {
		req.Path = s.tempDirProvider.TempDir() + string(os.PathSeparator) + "anytype-download"
	}

	err := os.MkdirAll(req.Path, 0755)
	if err != nil {
		return "", fmt.Errorf("mkdir -p: %w", anyerror.CleanupError(err))
	}
	progress := process.NewProgress(&pb.ModelProcessMessageOfSaveFile{SaveFile: &pb.ModelProcessSaveFile{}})
	defer progress.Finish(nil)

	err = s.ProcessAdd(progress)
	if err != nil {
		return "", fmt.Errorf("add process: %w", err)
	}

	progress.SetProgressMessage("saving file")
	var countReader *datacounter.ReaderCounter
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-progress.Canceled():
				cancel()
			case <-time.After(time.Second):
				if countReader != nil {
					progress.SetDone(int64(countReader.Count()))
				}
			}
		}
	}()

	f, err := s.fileObjectService.GetFileData(ctx, req.ObjectId)
	if err != nil {
		return "", fmt.Errorf("get file by hash: %w", err)
	}

	progress.SetTotal(f.Meta().Size)

	r, err := f.Reader(rpcstore.ContextWithWaitAvailable(ctx))
	if err != nil {
		return "", fmt.Errorf("get file reader: %w", err)
	}
	countReader = datacounter.NewReaderCounter(r)
	fileName := f.Meta().Name
	if fileName == "" {
		fileName = f.Name()
	}

	path, err := files.WriteReaderIntoFileReuseSameExistingFile(req.Path+string(os.PathSeparator)+fileName, countReader)
	if err != nil {
		return "", fmt.Errorf("save file: %w", err)
	}

	progress.SetDone(f.Meta().Size)
	return path, nil
}
