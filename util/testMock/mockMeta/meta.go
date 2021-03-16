//go:generate mockgen -package mockMeta -destination meta_mock.go github.com/anytypeio/go-anytype-middleware/core/block/meta Service,PubSub,Subscriber
package mockMeta

import (
	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/golang/mock/gomock"
)

func RegisterMockMeta(ctrl *gomock.Controller, ta *testapp.TestApp) *MockService {
	ms := NewMockService(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(meta.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ms.EXPECT().Run().AnyTimes()
	ms.EXPECT().Close().AnyTimes()
	ta.Register(ms)
	return ms
}

func GetMockMeta(ta *testapp.TestApp) *MockService {
	return ta.MustComponent(meta.CName).(*MockService)
}
