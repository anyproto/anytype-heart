//go:generate mockgen -package mockStatus -destination status_mock.go github.com/anytypeio/go-anytype-middleware/core/status Service
package mockStatus

import (
	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/golang/mock/gomock"
)

func RegisterMockStatus(ctrl *gomock.Controller, ta *testapp.TestApp) *MockService {
	ms := NewMockService(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(status.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ms.EXPECT().Run().AnyTimes()
	ms.EXPECT().Close().AnyTimes()
	ta.Register(ms)
	return ms
}
