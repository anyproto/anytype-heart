package gallery

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pb"
)

//go:embed builtin/get_started.zip
var getStartedZip []byte

//go:embed builtin/empty.zip
var emptyZip []byte

var archives = map[pb.RpcObjectImportUseCaseRequestUseCase][]byte{
	pb.RpcObjectImportUseCaseRequest_GET_STARTED: getStartedZip,
	pb.RpcObjectImportUseCaseRequest_EMPTY:       emptyZip,
}

func (s *service) ImportBuiltInUseCase(
	ctx context.Context,
	spaceID string,
	useCase pb.RpcObjectImportUseCaseRequestUseCase,
) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error) {
	if useCase == pb.RpcObjectImportUseCaseRequest_NONE {
		return pb.RpcObjectImportUseCaseResponseError_NULL, nil
	}

	start := time.Now()

	info, found := ucCodeToInfo[useCase]
	if !found {
		return pb.RpcObjectImportUseCaseResponseError_BAD_INPUT,
			fmt.Errorf("failed to import built-in usecase: invalid Use Case value: %v", useCase)
	}

	if code, err = s.importUseCase(ctx, spaceID, info.Title, useCase); err != nil {
		return code, fmt.Errorf("failed to import built-in usecase %s: %w",
			pb.RpcObjectImportUseCaseRequestUseCase_name[int32(useCase)], err)
	}

	spent := time.Since(start)
	if spent > injectionTimeout {
		log.Debug("built-in objects injection time exceeded timeout", zap.String("timeout", injectionTimeout.String()), zap.String("spent", spent.String()))
	}

	return pb.RpcObjectImportUseCaseResponseError_NULL, nil
}

func (s *service) importUseCase(
	ctx context.Context, spaceID, title string, useCase pb.RpcObjectImportUseCaseRequestUseCase,
) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error) {
	archive, found := archives[useCase]
	if !found {
		return pb.RpcObjectImportUseCaseResponseError_BAD_INPUT,
			fmt.Errorf("failed to import built-in usecase: invalid Use Case value: %v", useCase)
	}

	path, remove, err := s.saveArchiveToTempFile(archive)
	if err != nil {
		return pb.RpcObjectImportUseCaseResponseError_UNKNOWN_ERROR, fmt.Errorf("failed to save built-in usecase in temp file: %w", err)
	}

	err = s.importArchive(ctx, spaceID, path, title, nil, true)
	remove(path)

	if err != nil {
		return pb.RpcObjectImportUseCaseResponseError_UNKNOWN_ERROR, err
	}
	return pb.RpcObjectImportUseCaseResponseError_NULL, nil
}

func (s *service) saveArchiveToTempFile(archive []byte) (path string, removeFunc func(string), err error) {
	path = filepath.Join(s.tempDirService.TempDir(), time.Now().Format("tmp.20060102.150405.99")+".zip")
	if err = os.WriteFile(path, archive, 0600); err != nil {
		return "", nil, fmt.Errorf("failed to save archive to temporary file: %w", err)
	}
	return path, removeTempFile, nil
}
