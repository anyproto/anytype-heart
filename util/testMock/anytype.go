//go:generate mockgen -package testMock -destination anytype_mock.go github.com/anytypeio/go-anytype-middleware/pkg/lib/core Service
//go:generate mockgen -package testMock -destination objectstore_mock.go github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore ObjectStore
//go:generate mockgen -package testMock -destination history_mock.go github.com/anytypeio/go-anytype-middleware/core/block/undo History
//go:generate mockgen -package testMock -destination sbt_provider_mock.go github.com/anytypeio/go-anytype-middleware/space/typeprovider SmartBlockTypeProvider
//go:generate mockgen -package testMock -destination file_service_mock.go -mock_names Service=MockFileService github.com/anytypeio/go-anytype-middleware/core/files Service,Image,File
package testMock

import (
	"context"

	"github.com/anytypeio/any-sync/app"
	"github.com/golang/mock/gomock"

	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/kanban"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockKanban"
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
