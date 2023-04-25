//go:generate mockgen -package mockSpace -destination space_mock.go github.com/anytypeio/go-anytype-middleware/space Service
package mockSpace

import (
	"context"

	"github.com/golang/mock/gomock"

	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/space"
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
