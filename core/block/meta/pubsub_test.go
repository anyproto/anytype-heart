package meta

import (
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockSource"
	"github.com/golang/mock/gomock"
)

func TestSubscriber_Subscribe(t *testing.T) {
	fx := newFixture(t)
	defer fx.tearDown()

	var (
		blockId = "1"
	)
	fx.source.EXPECT().ReadDetails().Return(state.NewDoc("", nil), nil)
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
	s := mockSource.NewMockSource(ctrl)
	fx = &fixture{
		ctrl:    ctrl,
		anytype: at,
		Service: NewService(at),
		source:  s,
	}
	fx.PubSub().(*pubSub).newSource = func(id string) (source.Source, error) {
		return s, nil
	}
	return fx
}

type fixture struct {
	Service
	ctrl    *gomock.Controller
	anytype *testMock.MockService
	source  *mockSource.MockSource
}

func (fx *fixture) tearDown() {
	fx.ctrl.Finish()
}
