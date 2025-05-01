package service

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrInvalidExportFormat = errors.New("format is not supported")
)

// GetObjectExport retrieves an object from a space and exports it as a specific format.
func (s *Service) GetObjectExport(ctx context.Context, spaceId string, objectId string, format string) (string, error) {
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
func (s *Service) mapStringToFormat(format string) model.ExportFormat {
	switch format {
	case "markdown":
		return model.Export_Markdown
	case "protobuf":
		return model.Export_Protobuf
	default:
		return model.Export_Markdown
	}
}
