//go:generate mockgen -package mockRelation -destination relation_mock.go github.com/anyproto/anytype-heart/core/relation Service
package mockRelation

import (
	"github.com/anyproto/anytype-heart/app/testapp"
	"github.com/anyproto/anytype-heart/core/relation"
	"go.uber.org/mock/gomock"
)

func RegisterMockRelation(ctrl *gomock.Controller, ta *testapp.TestApp) *MockService {
	ms := NewMockService(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(relation.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ta.Register(ms)
	return ms
}
