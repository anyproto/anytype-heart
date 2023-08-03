//go:generate mockgen -package mockBuiltinTemplate -destination builtintemplate_mock.go github.com/anyproto/anytype-heart/util/builtintemplate BuiltinTemplate
package mockBuiltinTemplate

import (
	"context"

	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/app/testapp"
	"github.com/anyproto/anytype-heart/util/builtintemplate"
)

func RegisterMockBuiltinTemplate(ctrl *gomock.Controller, ta *testapp.TestApp) *MockBuiltinTemplate {
	ms := NewMockBuiltinTemplate(ctrl)
	ms.EXPECT().Name().AnyTimes().Return(builtintemplate.CName)
	ms.EXPECT().Init(gomock.Any()).AnyTimes()
	ms.EXPECT().Run(context.Background()).AnyTimes()
	ms.EXPECT().Close(context.Background()).AnyTimes()
	ta.Register(ms)
	return ms
}
