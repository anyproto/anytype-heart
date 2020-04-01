package meta

import (
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/vclock"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/golang/mock/gomock"
)

func TestSubscriber_Subscribe(t *testing.T) {
	fx := newFixture(t)
	defer fx.tearDown()

	var (
		blockId   = "1"
		state     = vclock.New()
		mockBlock = testMock.NewMockSmartBlock(fx.ctrl)
		snapshot  = testMock.NewMockSmartBlockSnapshot(fx.ctrl)
		meta      = &core.SmartBlockMeta{}
	)
	mockBlock.EXPECT().GetLastSnapshot().Return(snapshot, nil)
	snapshot.EXPECT().State().Return(state)
	snapshot.EXPECT().Meta().Return(meta, nil)
	mockBlock.EXPECT().SubscribeForMetaChanges(state, gomock.Any())
	fx.anytype.EXPECT().GetBlock(blockId).Return(mockBlock, nil)
	s := fx.PubSub().NewSubscriber()
	var mch = make(chan Meta, 1)
	f := func(m Meta) {
		mch <- m
	}
	s.Callback(f).Subscribe(blockId)
	select {
	case <-time.After(time.Second):
		t.Errorf("timeout")
	case <-mch:
	}
}

func newFixture(t *testing.T) (fx *fixture) {
	ctrl := gomock.NewController(t)
	at := testMock.NewMockService(ctrl)
	return &fixture{
		ctrl:    ctrl,
		anytype: at,
		Service: NewService(at),
	}
}

type fixture struct {
	Service
	ctrl    *gomock.Controller
	anytype *testMock.MockService
}

func (fx *fixture) tearDown() {
	fx.ctrl.Finish()
}
