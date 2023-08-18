package builtintemplate

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/mock_objectstore"
	"github.com/anyproto/anytype-heart/util/testMock/mockSource"
)

func Test_registerBuiltin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s := mockSource.NewMockService(ctrl)
	s.EXPECT().Name().Return(source.CName).AnyTimes()
	s.EXPECT().Init(gomock.Any()).AnyTimes()
	s.EXPECT().NewStaticSource(gomock.Any(), gomock.Any(), gomock.Any(), nil).AnyTimes()
	s.EXPECT().RegisterStaticSource(gomock.Any(), gomock.Any()).AnyTimes()

	objectStore := mock_objectstore.NewMockObjectStore(t)
	objectStore.EXPECT().Name().Return("objectstore")

	builtInTemplates := New()
	a := new(app.App)
	a.Register(s).
		Register(builtInTemplates).
		Register(config.New()).
		Register(objectStore)
	err := builtInTemplates.Init(a)
	assert.NoError(t, err)
	err = builtInTemplates.Run(context.Background())
	assert.NoError(t, err)
}
