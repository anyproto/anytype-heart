package builtintemplate

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockSource"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func Test_registerBuiltin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	s := mockSource.NewMockService(ctrl)
	s.EXPECT().Name().Return(source.CName).AnyTimes()
	s.EXPECT().Init(gomock.Any()).AnyTimes()
	s.EXPECT().NewStaticSource(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(id string, sbt model.SmartBlockType, s *state.State) {
		t.Log(s.StringDebug())
	}).AnyTimes()
	s.EXPECT().RegisterStaticSource(gomock.Any(), gomock.Any()).AnyTimes()

	a := testapp.New().With(s).With(New())
	require.NoError(t, a.Start())
	defer a.Close()
}
