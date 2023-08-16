package builtintemplate

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/relation/mock_relation"
	"github.com/anyproto/anytype-heart/util/testMock/mockSource"
	"go.uber.org/mock/gomock"
)

func Test_registerBuiltin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s := mockSource.NewMockService(ctrl)
	s.EXPECT().Name().Return(source.CName).AnyTimes()
	s.EXPECT().Init(gomock.Any()).AnyTimes()
	s.EXPECT().NewStaticSource(gomock.Any(), gomock.Any(), gomock.Any(), nil).AnyTimes()
	s.EXPECT().RegisterStaticSource(gomock.Any(), gomock.Any()).AnyTimes()

	relationService := mock_relation.NewMockService(t)
	relationService.EXPECT().Name().Return("relation")

	builtInTemplates := New()
	a := new(app.App)
	a.Register(s).
		Register(builtInTemplates).
		Register(config.New()).
		Register(relationService)
	err := builtInTemplates.Init(a)
	assert.NoError(t, err)
	err = builtInTemplates.Run(context.Background())
	assert.NoError(t, err)

	defer a.Close(context.Background())
}
