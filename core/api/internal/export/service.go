package export

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrInvalidExportFormat = errors.New("format is not supported")
)

type Service interface {
	GetObjectExport(ctx context.Context, spaceId string, objectId string, format string) (string, error)
}

type service struct {
	mw            apicore.ClientCommands
	exportService apicore.ExportService
}

func NewService(mw apicore.ClientCommands, exportService apicore.ExportService) Service {
	return &service{mw: mw, exportService: exportService}
}

// GetObjectExport retrieves an object from a space and exports it as a specific format.
func (s *service) GetObjectExport(ctx context.Context, spaceId string, objectId string, format string) (string, error) {
	if format != "markdown" {
		return "", ErrInvalidExportFormat
	}

	result, err := s.exportService.ExportSingleInMemory(ctx, spaceId, objectId, s.mapStringToFormat(format))
	if err != nil {
		return "", err
	}

	return result, nil
}

// mapStringToFormat maps a format string to an ExportFormat enum.
func (s *service) mapStringToFormat(format string) model.ExportFormat {
	switch format {
	case "markdown":
		return model.Export_Markdown
	case "protobuf":
		return model.Export_Protobuf
	default:
		return model.Export_Markdown
	}
}
