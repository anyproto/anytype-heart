package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	"github.com/anyproto/anytype-heart/pb"
)

type mockServerSender struct {
	grpc.ServerStream
	callback func()
}

func (m *mockServerSender) Send(e *pb.Event) error {
	m.callback()
	return nil
}

func TestGrpcSender_Flush(t *testing.T) {
	t.Run("events already flushed", func(t *testing.T) {
		// given
		sender := NewGrpcSender()
		sender.flushed = true
		server := &mockServerSender{}
		var called bool
		server.callback = func() {
			called = true
		}
		sender.Servers = map[string]SessionServer{"token": {
			Token:  "token",
			Server: server,
		}}

		// when
		sender.Flush()

		// then
		assert.False(t, called)
	})
	t.Run("no event", func(t *testing.T) {
		// given
		sender := NewGrpcSender()
		server := &mockServerSender{}
		var called bool
		server.callback = func() {
			called = true
		}
		sender.Servers = map[string]SessionServer{"token": {
			Token:  "token",
			Server: server,
		}}

		// when
		sender.Flush()

		// then
		assert.False(t, called)
		assert.True(t, sender.flushed)
	})
	t.Run("flush event", func(t *testing.T) {
		// given
		sender := NewGrpcSender()
		server := &mockServerSender{}
		done := make(chan struct{})
		server.callback = func() {
			close(done)
		}
		sender.Servers = map[string]SessionServer{"token": {
			Token:  "token",
			Server: server,
		}}
		sender.sessionEvents = map[string][]*pb.Event{"token": {{}}}

		// when
		sender.Flush()
		<-done

		// then
		assert.True(t, sender.flushed)
	})
	t.Run("no servers", func(t *testing.T) {
		// given
		sender := NewGrpcSender()
		server := &mockServerSender{}
		var called bool
		server.callback = func() {
			called = true
		}
		sender.sessionEvents = map[string][]*pb.Event{"token": {{}}}

		// when
		sender.Flush()

		// then
		assert.True(t, sender.flushed)
		assert.False(t, called)
	})
}

func TestCallbackSender_Flush(t *testing.T) {
	t.Run("events already flushed", func(t *testing.T) {
		// given
		var called bool
		sender := NewCallbackSender(func(event *pb.Event) {
			called = true
		})
		sender.flushed = true

		// when
		sender.Flush()

		// then
		assert.False(t, called)
	})
	t.Run("no event", func(t *testing.T) {
		// given
		var called bool
		sender := NewCallbackSender(func(event *pb.Event) {
			called = true
		})

		// when
		sender.Flush()

		// then
		assert.False(t, called)
		assert.True(t, sender.flushed)
	})

	t.Run("flush event", func(t *testing.T) {
		// given
		done := make(chan struct{})
		sender := NewCallbackSender(func(event *pb.Event) {
			close(done)

		})
		sender.events = append(sender.events, &pb.Event{})

		// when
		sender.Flush()
		<-done

		// then
		assert.True(t, sender.flushed)
	})
}
