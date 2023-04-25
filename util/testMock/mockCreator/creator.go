//go:generate mockgen -package mockCreator -destination creator_mock.go github.com/anytypeio/go-anytype-middleware/core/block/object/creator Service
package mockCreator

import (
	"github.com/golang/mock/gomock"

	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/block/object/creator"
)

func RegisterMockCreator(ctrl *gomock.Controller, ta *testapp.TestApp) *MockService {
	ms := NewMockService(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(creator.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ta.Register(ms)
	return ms
}
