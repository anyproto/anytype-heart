//go:generate mockgen -package mockDoc -destination doc_mock.go github.com/anytypeio/go-anytype-middleware/core/block/doc Service
package mockDoc

import (
	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/block/doc"
	"github.com/golang/mock/gomock"
)

func RegisterMockDoc(ctrl *gomock.Controller, ta *testapp.TestApp) *MockService {
	ms := NewMockService(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(doc.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ms.EXPECT().Run().AnyTimes()
	ms.EXPECT().Close().AnyTimes()
	ta.Register(ms)
	return ms
}
