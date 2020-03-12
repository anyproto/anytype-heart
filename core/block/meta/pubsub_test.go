package meta

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/golang/mock/gomock"
)

func TestSubscriber_Subscribe(t *testing.T) {
	fx := newFixture(t)
	defer fx.tearDown()
	s := fx.PubSub().NewSubscriber()
	var mch = make(chan Meta)
	f := func(m Meta) {
		mch <- m
	}
	s.Callback(f).Subscribe("1")
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
