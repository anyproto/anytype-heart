package builtintemplate

import (
	"context"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/anyproto/anytype-heart/app/testapp"
	"github.com/anyproto/anytype-heart/core/block/source"
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

	builtInTemplates := New()
	a := testapp.New().With(s).With(builtInTemplates).With(config.New()).App
	err := builtInTemplates.Init(a)
	assert.Nil(t, err)
	err = builtInTemplates.Run(context.Background())
	assert.Nil(t, err)

	defer a.Close(context.Background())
}
