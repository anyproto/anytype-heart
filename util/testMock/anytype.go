//go:generate mockgen -package testMock -destination anytype_mock.go github.com/anyproto/anytype-heart/pkg/lib/core Service
//go:generate mockgen -package testMock -destination history_mock.go github.com/anyproto/anytype-heart/core/block/undo History
//go:generate mockgen -package testMock -destination sbt_provider_mock.go github.com/anyproto/anytype-heart/space/spacecore/typeprovider SmartBlockTypeProvider
//go:generate mockgen -package testMock -destination file_service_mock.go -mock_names Service=MockFileService github.com/anyproto/anytype-heart/core/files Service,Image,File
package testMock

import (
	"github.com/anyproto/any-sync/app"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/util/testMock/mockKanban"
)

type App interface {
	Register(component app.Component) *app.App
}

func RegisterMockKanban(ctrl *gomock.Controller, ta App) *mockKanban.MockService {
	ms := mockKanban.NewMockService(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(kanban.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ta.Register(ms)
	return ms
}
