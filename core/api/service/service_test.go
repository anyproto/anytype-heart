package service

import (
	"testing"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
)

const (
	offset              = 0
	limit               = 100
	gatewayUrl          = "http://localhost:31006"
	techSpaceId         = "tech-space-id"
	mockedSpaceId       = "mocked-space-id"
	mockedObjectId      = "mocked-object-id"
	mockedNewObjectId   = "mocked-new-object-id"
	mockedObjectName    = "mocked-object-name"
	mockedObjectSnippet = "mocked-object-snippet"
	mockedObjectIcon    = "🔍"
	mockedParticipantId = "mocked-participant-id"
	mockedTypeKey       = "page"
	mockedTypeId        = "mocked-type-id"
	mockedTypeName      = "mocked-type-name"
	mockedTypeIcon      = "📝"
	mockedTemplateId    = "mocked-template-id"
	mockedTemplateName  = "mocked-template-name"
	mockedTemplateIcon  = "📃"
)

type fixture struct {
	service *Service
	mwMock  *mock_apicore.MockClientCommands
}

func newFixture(t *testing.T) *fixture {
	mwMock := mock_apicore.NewMockClientCommands(t)
	service := NewService(mwMock, gatewayUrl, techSpaceId, nil, nil)

	return &fixture{
		service: service,
		mwMock:  mwMock,
	}
}
