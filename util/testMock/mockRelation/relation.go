//go:generate mockgen -package mockRelation -destination relation_mock.go github.com/anytypeio/go-anytype-middleware/core/relation Service
package mockRelation

import (
	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/golang/mock/gomock"
)

func RegisterMockRelation(ctrl *gomock.Controller, ta *testapp.TestApp) *MockService {
	ms := NewMockService(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(relation.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ta.Register(ms)
	return ms
}
