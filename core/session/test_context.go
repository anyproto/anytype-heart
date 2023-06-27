package session

import (
	"context"
	"testing"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
)

type TestContext struct {
	Sender *mock_event.MockSender
	*sessionContext
}

func NewTestContext(t *testing.T) *TestContext {
	es := mock_event.NewMockSender(t)
	return &TestContext{
		Sender:         es,
		sessionContext: NewContext(context.Background(), es, "").(*sessionContext),
	}
}
