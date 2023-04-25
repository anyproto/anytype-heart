package block

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/miolini/datacounter"

	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	files2 "github.com/anytypeio/go-anytype-middleware/core/files"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

// TODO Move residual file methods here

// TODO Extract to a new service FileDownloader
func (s *Service) DownloadFile(req *pb.RpcFileDownloadRequest) (string, error) {
	if req.Path == "" {
		req.Path = s.tempDirProvider.TempDir() + string(os.PathSeparator) + "anytype-download"
	}

	err := os.MkdirAll(req.Path, 0755)
	if err != nil {
		return "", fmt.Errorf("mkdir -p %s: %w", req.Path, err)
	}
	progress := process.NewProgress(pb.ModelProcess_SaveFile)
	defer progress.Finish()

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

	f, err := s.getFileOrLargestImage(ctx, req.Hash)
	if err != nil {
		return "", fmt.Errorf("get file by hash: %w", err)
	}

	progress.SetTotal(f.Meta().Size)

	r, err := f.Reader()
	if err != nil {
		return "", fmt.Errorf("get file reader: %w", err)
	}
	countReader = datacounter.NewReaderCounter(r)
	fileName := f.Meta().Name
	if fileName == "" {
		fileName = f.Info().Name
	}

	path, err := files2.WriteReaderIntoFileReuseSameExistingFile(req.Path+string(os.PathSeparator)+fileName, countReader)
	if err != nil {
		return "", fmt.Errorf("save file: %w", err)
	}

	progress.SetDone(f.Meta().Size)
	return path, nil
}

func (s *Service) getFileOrLargestImage(ctx context.Context, hash string) (files2.File, error) {
	image, err := s.fileService.ImageByHash(ctx, hash)
	if err != nil {
		return s.fileService.FileByHash(ctx, hash)
	}

	f, err := image.GetOriginalFile(ctx)
	if err != nil {
		return s.fileService.FileByHash(ctx, hash)
	}

	return f, nil
}
