//go:generate mockgen -package mockSpace -destination space_mock.go github.com/anyproto/anytype-heart/space Service
package mockSpace

import (
	"context"

	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/app/testapp"
	"github.com/anyproto/anytype-heart/space"
)

func RegisterMockSpace(ctrl *gomock.Controller, ta *testapp.TestApp) *MockService {
	ms := NewMockService(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(space.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ms.EXPECT().Run(context.Background()).AnyTimes()
	ms.EXPECT().Close(context.Background()).AnyTimes()
	ta.Register(ms)
	return ms
}
