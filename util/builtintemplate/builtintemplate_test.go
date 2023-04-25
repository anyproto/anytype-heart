package builtintemplate

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockSource"
	"github.com/golang/mock/gomock"
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
