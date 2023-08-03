//go:generate mockgen -package mockCreator -destination creator_mock.go github.com/anyproto/anytype-heart/core/block/object/objectcreator Service
package mockCreator

import (
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/app/testapp"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
)

func RegisterMockCreator(ctrl *gomock.Controller, ta *testapp.TestApp) *MockService {
	ms := NewMockService(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(objectcreator.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ta.Register(ms)
	return ms
}
