package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/mock_space"
)

type fixture struct {
	eventSender  *mock_event.MockSender
	spaceService *mock_space.MockService

	*Service
}

func newFixture(t *testing.T) *fixture {
	eventSender := mock_event.NewMockSender(t)
	spaceService := mock_space.NewMockService(t)

	s := New()
	s.eventSender = eventSender
	s.spaceService = spaceService

	return &fixture{
		eventSender:  eventSender,
		spaceService: spaceService,
		Service:      s,
	}
}

func TestServiceClose(t *testing.T) {
	t.Run("broadcast close events on all opened objects", func(t *testing.T) {
		objects := map[string]bool{
			"obj1": true,
			"obj2": true,
			"obj3": true,
		}
		fx := newFixture(t)
		fx.eventSender.EXPECT().Broadcast(mock.Anything).RunAndReturn(func(e *pb.Event) {
			assert.Len(t, e.Messages, 1)
			msg := e.Messages[0].GetObjectClose()
			assert.NotNil(t, msg)
			assert.Equal(t, pb.EventObjectClose_Middle, msg.Closer)
			assert.Contains(t, []string{"obj1", "obj2", "obj3"}, msg.Id)
		})
		fx.openedObjs.objects = objects

		_ = fx.Close(nil)
	})
}
