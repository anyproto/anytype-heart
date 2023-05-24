//go:generate mockgen -package mockStatus -destination status_mock.go github.com/anyproto/anytype-heart/core/syncstatus Service
package mockStatus

import (
	"context"

	"github.com/golang/mock/gomock"

	"github.com/anyproto/anytype-heart/app/testapp"
	"github.com/anyproto/anytype-heart/core/syncstatus"
)

func RegisterMockStatus(ctrl *gomock.Controller, ta *testapp.TestApp) *MockService {
	ms := NewMockService(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(syncstatus.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ms.EXPECT().Run(context.Background()).AnyTimes()
	ms.EXPECT().Close(context.Background()).AnyTimes()
	ta.Register(ms)
	return ms
}
