package internal

import (
	"github.com/anyproto/anytype-heart/core/api/apicore"
)

type Service struct {
	mw            apicore.ClientCommands
	exportService apicore.ExportService
	gatewayUrl    string
	techSpaceId   string
}

func NewService(mw apicore.ClientCommands, exportService apicore.ExportService, gatewayUrl string, techspaceId string) *Service {
	return &Service{mw: mw, exportService: exportService, gatewayUrl: gatewayUrl, techSpaceId: techspaceId}
}
