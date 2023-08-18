//go:generate mockgen -package testMock -destination anytype_mock.go github.com/anyproto/anytype-heart/pkg/lib/core Service
//go:generate mockgen -package testMock -destination objectstore_mock.go github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore ObjectStore
//go:generate mockgen -package testMock -destination history_mock.go github.com/anyproto/anytype-heart/core/block/undo History
//go:generate mockgen -package testMock -destination sbt_provider_mock.go github.com/anyproto/anytype-heart/space/typeprovider SmartBlockTypeProvider
//go:generate mockgen -package testMock -destination file_service_mock.go -mock_names Service=MockFileService github.com/anyproto/anytype-heart/core/files Service,Image,File
package testMock

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/app/testapp"
	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/util/testMock/mockKanban"
)

type App interface {
	Register(component app.Component) *app.App
}

func RegisterMockAnytype(ctrl *gomock.Controller, ta *testapp.TestApp) *MockService {
	ms := NewMockService(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(core.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ms.EXPECT().Run(context.Background()).AnyTimes()
	ms.EXPECT().Close(context.Background()).AnyTimes()
	ms.EXPECT().ProfileID().AnyTimes().Return("profileId")
	ta.Register(ms)
	return ms
}

func RegisterMockObjectStore(ctrl *gomock.Controller, ta App) *MockObjectStore {
	ms := NewMockObjectStore(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(objectstore.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ms.EXPECT().Run(context.Background()).AnyTimes()
	ms.EXPECT().Close(context.Background()).AnyTimes()
	ta.Register(ms)
	return ms
}

func RegisterMockKanban(ctrl *gomock.Controller, ta App) *mockKanban.MockService {
	ms := mockKanban.NewMockService(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(kanban.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ta.Register(ms)
	return ms
}

func GetMockAnytype(ta *testapp.TestApp) *MockService {
	return ta.MustComponent(core.CName).(*MockService)
}
