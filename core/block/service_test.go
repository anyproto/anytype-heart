package block

import (
	"errors"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestService_OpenBlock(t *testing.T) {
	t.Run("error while open block", func(t *testing.T) {
		var (
			accountId = "123"
			blockId   = "456"
			expErr    = errors.New("test err")
		)
		fx := newFixture(t, accountId)
		defer fx.tearDown()

		fx.anytype.EXPECT().GetBlock(blockId).Return(nil, expErr)

		err := fx.OpenBlock(blockId)
		require.Equal(t, expErr, err)
	})
}

func newFixture(t *testing.T, accountId string) *fixture {
	ctrl := gomock.NewController(t)
	anytype := testMock.NewMockAnytype(ctrl)
	return &fixture{
		Service: NewService(accountId, anytype, nil),
		t:       t,
		ctrl:    ctrl,
		anytype: anytype,
	}
}

type fixture struct {
	Service
	t       *testing.T
	ctrl    *gomock.Controller
	anytype *testMock.MockAnytype
}

func (fx *fixture) tearDown() {
	require.NoError(fx.t, fx.Close())
	fx.ctrl.Finish()
}
