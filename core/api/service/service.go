package service

import (
	apicore "github.com/anyproto/anytype-heart/core/api/core"
)

type Service struct {
	mw          apicore.ClientCommands
	gatewayUrl  string
	techSpaceId string
}

func NewService(mw apicore.ClientCommands, gatewayUrl string, techspaceId string) *Service {
	return &Service{mw: mw, gatewayUrl: gatewayUrl, techSpaceId: techspaceId}
}
