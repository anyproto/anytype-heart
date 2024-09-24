package gallery

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/anyproto/anytype-heart/pb"
)

// TODO: GO-4131 Remove all embeds when clients support cache
//
//go:embed builtin/get_started.zip
var getStartedZip []byte

//go:embed builtin/personal_projects.zip
var personalProjectsZip []byte

//go:embed builtin/knowledge_base.zip
var knowledgeBaseZip []byte

//go:embed builtin/notes_diary.zip
var notesDiaryZip []byte

//go:embed builtin/strategic_writing.zip
var strategicWritingZip []byte

//go:embed builtin/empty.zip
var emptyZip []byte

var archives = map[pb.RpcObjectImportUseCaseRequestUseCase][]byte{
	pb.RpcObjectImportUseCaseRequest_GET_STARTED:       getStartedZip,
	pb.RpcObjectImportUseCaseRequest_PERSONAL_PROJECTS: personalProjectsZip,
	pb.RpcObjectImportUseCaseRequest_KNOWLEDGE_BASE:    knowledgeBaseZip,
	pb.RpcObjectImportUseCaseRequest_NOTES_DIARY:       notesDiaryZip,
	pb.RpcObjectImportUseCaseRequest_STRATEGIC_WRITING: strategicWritingZip,
	pb.RpcObjectImportUseCaseRequest_EMPTY:             emptyZip,
}

// TODO: GO-4131 Remove this method when clients support cache
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
