//go:generate mockgen -package mockBuiltinTemplate -destination builtintemplate_mock.go github.com/anytypeio/go-anytype-middleware/util/builtintemplate BuiltinTemplate
package mockBuiltinTemplate

import (
	"context"

	"github.com/golang/mock/gomock"

	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/util/builtintemplate"
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
