//go:generate mockgen -package testMock -destination anytype_mock.go github.com/anytypeio/go-anytype-middleware/pkg/lib/core Service,SmartBlock,SmartBlockSnapshot,File,Image
//go:generate mockgen -package testMock -destination objectstore_mock.go github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore ObjectStore
//go:generate mockgen -package testMock -destination history_mock.go github.com/anytypeio/go-anytype-middleware/core/block/undo History
package testMock

import (
	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/golang/mock/gomock"
)

func RegisterMockAnytype(ctrl *gomock.Controller, ta *testapp.TestApp) *MockService {
	ms := NewMockService(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(core.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ms.EXPECT().Run().AnyTimes()
	ms.EXPECT().Close().AnyTimes()
	ta.Register(ms)
	return ms
}

func RegisterMockObjectStore(ctrl *gomock.Controller, ta *testapp.TestApp) *MockObjectStore {
	ms := NewMockObjectStore(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(objectstore.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ta.Register(ms)
	return ms
}

func GetMockAnytype(ta *testapp.TestApp) *MockService {
	return ta.MustComponent(core.CName).(*MockService)
}
