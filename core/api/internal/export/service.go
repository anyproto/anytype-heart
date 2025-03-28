package export

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrFailedExportObjectAsMarkdown = errors.New("failed to export object as markdown")
	ErrBadInput                     = errors.New("bad input")
)

type Service interface {
	GetObjectExport(ctx context.Context, spaceId string, objectId string, format string, path string) (string, error)
}

type ExportService struct {
	mw apicore.ClientCommands
}

func NewService(mw apicore.ClientCommands) *ExportService {
	return &ExportService{mw: mw}
}

// GetObjectExport retrieves an object from a space and exports it as a specific format.
func (s *ExportService) GetObjectExport(ctx context.Context, spaceId string, objectId string, format string, path string) (string, error) {
	resp := s.mw.ObjectListExport(ctx, &pb.RpcObjectListExportRequest{
		SpaceId:         spaceId,
		Path:            path,
		ObjectIds:       []string{objectId},
		Format:          s.mapStringToFormat(format),
		Zip:             false,
		IncludeNested:   false,
		IncludeFiles:    true,
		IsJson:          false,
		IncludeArchived: false,
		NoProgress:      true,
	})

	if resp.Error.Code != pb.RpcObjectListExportResponseError_NULL {
		return "", ErrFailedExportObjectAsMarkdown
	}

	return resp.Path, nil
}

// mapStringToFormat maps a format string to an ExportFormat enum.
func (s *ExportService) mapStringToFormat(format string) model.ExportFormat {
	switch format {
	case "markdown":
		return model.Export_Markdown
	case "protobuf":
		return model.Export_Protobuf
	default:
		return model.Export_Markdown
	}
}
