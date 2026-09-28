package pubsub

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	anysyncpubsub "github.com/anyproto/any-sync/commonspace/pubsub"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"storj.io/drpc"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

const testSpaceId = "space1"

type fixture struct {
	*service
	engine      *fakeEngine
	eventSender *mock_event.MockSender
	identity    crypto.PubKey
}

func newFixture(t *testing.T) *fixture {
	engine := &fakeEngine{subs: map[string]anysyncpubsub.Handler{}}
	keys, err := accountdata.NewRandom()
	require.NoError(t, err)
	fx := &fixture{
		service:     New().(*service),
		engine:      engine,
		eventSender: mock_event.NewMockSender(t),
		identity:    keys.SignKey.GetPublic(),
	}
	fx.service.engine = engine
	fx.spaces = &testSpaceLoader{}
	t.Cleanup(func() { require.NoError(t, fx.Close(context.Background())) })
	fx.service.eventSender = fx.eventSender
	return fx
}

// fakeEngine records subscriptions and lets tests fire messages at handlers.
type fakeEngine struct {
	anysyncpubsub.Service
	publish      func(ctx context.Context, spaceId, topic string, payload []byte) error
	subscribeErr func(spaceId, pattern string) error
	mu           sync.Mutex
	subs         map[string]anysyncpubsub.Handler // spaceId+"/"+pattern -> handler
	unsubs       []string
	closedSpaces []string
}

func (f *fakeEngine) Publish(ctx context.Context, spaceId, topic string, payload []byte) error {
	if f.publish != nil {
		return f.publish(ctx, spaceId, topic, payload)
	}
	return nil
}

func (f *fakeEngine) Subscribe(spaceId, pattern string, h anysyncpubsub.Handler) (func(), error) {
	if f.subscribeErr != nil {
		if err := f.subscribeErr(spaceId, pattern); err != nil {
			return nil, err
		}
	}
	if err := anysyncpubsub.ValidatePattern(pattern); err != nil {
		return nil, err
	}
	key := spaceId + "/" + pattern
	f.mu.Lock()
	f.subs[key] = h
	f.mu.Unlock()
	return func() {
		f.mu.Lock()
		delete(f.subs, key)
		f.unsubs = append(f.unsubs, key)
		f.mu.Unlock()
	}, nil
}

func (f *fakeEngine) CloseSpace(spaceId string) {
	f.mu.Lock()
	f.closedSpaces = append(f.closedSpaces, spaceId)
	f.mu.Unlock()
}

func (f *fakeEngine) receive(spaceId, topic string, identity crypto.PubKey, payload []byte) {
	f.mu.Lock()
	var matched []anysyncpubsub.Handler
	for key, h := range f.subs {
		if key == spaceId+"/"+topic {
			matched = append(matched, h)
		}
	}
	f.mu.Unlock()
	for _, h := range matched {
		h(spaceId, topic, identity, payload)
	}
}

func (f *fakeEngine) HandleStream(stream drpc.Stream) error { return nil }

func pubsubEvents(ev *pb.Event) (out []*pb.EventPubsubMessage) {
	for _, msg := range ev.Messages {
		if v, ok := msg.Value.(*pb.EventMessageValueOfPubsubMessage); ok {
			out = append(out, v.PubsubMessage)
		}
	}
	return
}

func TestSubscribe(t *testing.T) {
	t.Run("generates subId when empty", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		subId, err := fx.Subscribe(context.Background(), testSpaceId, []string{"typing/obj1"}, "")

		// then
		require.NoError(t, err)
		assert.NotEmpty(t, subId)
		assert.Len(t, fx.engine.subs, 1)
	})

	t.Run("invalid pattern rejected", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		_, err := fx.Subscribe(context.Background(), testSpaceId, []string{"/bad//topic"}, "sub1")

		// then
		require.Error(t, err)
		assert.Empty(t, fx.engine.subs)
	})

	t.Run("empty topics rejected", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		_, err := fx.Subscribe(context.Background(), testSpaceId, nil, "sub1")

		// then
		require.ErrorIs(t, err, ErrEmptyTopics)
	})

	t.Run("same pattern shared across subIds", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		_, err1 := fx.Subscribe(context.Background(), testSpaceId, []string{"typing/obj1"}, "sub1")
		_, err2 := fx.Subscribe(context.Background(), testSpaceId, []string{"typing/obj1"}, "sub2")

		// then
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Len(t, fx.engine.subs, 1, "one engine subscription per pattern")
	})

	t.Run("resubscribe replaces pattern set", func(t *testing.T) {
		// given
		fx := newFixture(t)
		_, err := fx.Subscribe(context.Background(), testSpaceId, []string{"typing/obj1"}, "sub1")
		require.NoError(t, err)

		// when
		_, err = fx.Subscribe(context.Background(), testSpaceId, []string{"typing/obj2"}, "sub1")

		// then
		require.NoError(t, err)
		assert.Len(t, fx.engine.subs, 1)
		assert.Contains(t, fx.engine.subs, testSpaceId+"/typing/obj2")
		assert.Equal(t, []string{testSpaceId + "/typing/obj1"}, fx.engine.unsubs)
	})
}

func TestReceive(t *testing.T) {
	t.Run("message emitted once with all matching subIds", func(t *testing.T) {
		// given
		fx := newFixture(t)
		_, err := fx.Subscribe(context.Background(), testSpaceId, []string{"typing/obj1"}, "sub1")
		require.NoError(t, err)
		_, err = fx.Subscribe(context.Background(), testSpaceId, []string{"typing/obj1"}, "sub2")
		require.NoError(t, err)
		want := &pb.EventPubsubMessage{
			Topic:    "typing/obj1",
			Payload:  []byte(`{"active":true}`),
			Identity: fx.identity.Account(),
			SubIds:   []string{"sub1", "sub2"},
		}
		var got []*pb.EventPubsubMessage
		fx.eventSender.EXPECT().Broadcast(mock.Anything).Run(func(ev *pb.Event) {
			got = append(got, pubsubEvents(ev)...)
			assert.Equal(t, testSpaceId, ev.Messages[0].SpaceId)
		}).Once()

		// when
		fx.engine.receive(testSpaceId, "typing/obj1", fx.identity, []byte(`{"active":true}`))

		// then
		require.Len(t, got, 1)
		sort.Strings(got[0].SubIds)
		assert.Equal(t, want, got[0])
	})

	t.Run("no event after unsubscribe", func(t *testing.T) {
		// given
		fx := newFixture(t)
		subId, err := fx.Subscribe(context.Background(), testSpaceId, []string{"typing/obj1"}, "sub1")
		require.NoError(t, err)
		require.NoError(t, fx.Unsubscribe(subId))

		// when
		fx.engine.receive(testSpaceId, "typing/obj1", fx.identity, []byte("x"))

		// then: eventSender mock has no Broadcast expectation — a call would fail
		assert.Empty(t, fx.engine.subs)
		assert.Equal(t, []string{testSpaceId + "/typing/obj1"}, fx.engine.unsubs)
	})
}

func TestUnsubscribe(t *testing.T) {
	t.Run("unknown subId", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		err := fx.Unsubscribe("missing")

		// then
		require.ErrorIs(t, err, ErrSubscriptionNotFound)
	})

	t.Run("shared pattern survives until last subId", func(t *testing.T) {
		// given
		fx := newFixture(t)
		_, err := fx.Subscribe(context.Background(), testSpaceId, []string{"typing/obj1"}, "sub1")
		require.NoError(t, err)
		_, err = fx.Subscribe(context.Background(), testSpaceId, []string{"typing/obj1"}, "sub2")
		require.NoError(t, err)

		// when
		require.NoError(t, fx.Unsubscribe("sub1"))

		// then
		assert.Len(t, fx.engine.subs, 1)

		// when
		require.NoError(t, fx.Unsubscribe("sub2"))

		// then
		assert.Empty(t, fx.engine.subs)
	})
}

func TestCloseSpace(t *testing.T) {
	t.Run("drops all space subscriptions", func(t *testing.T) {
		// given
		fx := newFixture(t)
		_, err := fx.Subscribe(context.Background(), testSpaceId, []string{"typing/obj1", "typing/obj2"}, "sub1")
		require.NoError(t, err)
		_, err = fx.Subscribe(context.Background(), "space2", []string{"typing/obj3"}, "sub2")
		require.NoError(t, err)

		// when
		fx.CloseSpace(testSpaceId)

		// then
		assert.Len(t, fx.engine.subs, 1, "other space subscription kept")
		assert.Contains(t, fx.engine.subs, "space2/typing/obj3")
		assert.Equal(t, []string{testSpaceId}, fx.engine.closedSpaces)
		require.ErrorIs(t, fx.Unsubscribe("sub1"), ErrSubscriptionNotFound)
		require.NoError(t, fx.Unsubscribe("sub2"))
	})
}

func TestPublish(t *testing.T) {
	t.Run("rejects canceled requests before enqueueing", func(t *testing.T) {
		fx := newFixture(t)
		fx.engine.publish = func(context.Context, string, string, []byte) error {
			t.Fatal("canceled request reached engine")
			return nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.ErrorIs(t, fx.Publish(ctx, testSpaceId, "typing/object", nil), context.Canceled)
	})
	t.Run("delegates to engine", func(t *testing.T) {
		// given
		fx := newFixture(t)
		published := false
		fx.engine.publish = func(ctx context.Context, spaceId, topic string, payload []byte) error {
			published = true
			assert.Equal(t, testSpaceId, spaceId)
			assert.Equal(t, "typing/obj1", topic)
			return nil
		}

		// when
		err := fx.Publish(context.Background(), testSpaceId, "typing/obj1", []byte("x"))

		// then
		require.NoError(t, err)
		assert.True(t, published)
	})
}

func TestSubscribeLoadingFailurePreservesExistingSubscription(t *testing.T) {
	fx := newFixture(t)
	_, err := fx.Subscribe(context.Background(), testSpaceId, []string{"typing/old"}, "sub")
	require.NoError(t, err)
	loadErr := errors.New("space unavailable")
	fx.spaces = &testSpaceLoader{load: func(context.Context, string) error { return loadErr }}
	_, err = fx.Subscribe(context.Background(), "other-space", []string{"typing/new"}, "sub")
	require.ErrorIs(t, err, loadErr)
	require.Contains(t, fx.engine.subs, testSpaceId+"/typing/old")
	require.Equal(t, testSpaceId, fx.subs["sub"].spaceId)
}

func TestSubscribeLoadingCanceledOnClose(t *testing.T) {
	fx := newFixture(t)
	started := make(chan struct{})
	fx.spaces = &testSpaceLoader{load: func(ctx context.Context, _ string) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}
	done := make(chan error, 1)
	go func() {
		_, err := fx.Subscribe(context.Background(), testSpaceId, []string{"typing/object"}, "sub")
		done <- err
	}()
	<-started
	require.NoError(t, fx.Close(context.Background()))
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel Space loading")
	}
	require.Empty(t, fx.engine.subs)
}

func TestSubscribeRollsBackUnexpectedEngineFailure(t *testing.T) {
	for _, targetSpace := range []string{testSpaceId, "other-space"} {
		t.Run(targetSpace, func(t *testing.T) {
			fx := newFixture(t)
			_, err := fx.Subscribe(context.Background(), testSpaceId, []string{"typing/old", "typing/shared"}, "sub")
			require.NoError(t, err)
			_, err = fx.Subscribe(context.Background(), testSpaceId, []string{"typing/shared"}, "other-sub")
			require.NoError(t, err)
			subscribeErr := errors.New("engine refused pattern")
			fx.engine.subscribeErr = func(_, pattern string) error {
				if pattern == "typing/fail" {
					return subscribeErr
				}
				return nil
			}
			_, err = fx.Subscribe(context.Background(), targetSpace, []string{"typing/new", "typing/shared", "typing/fail"}, "sub")
			require.ErrorIs(t, err, subscribeErr)
			require.Len(t, fx.engine.subs, 2)
			require.Contains(t, fx.engine.subs, testSpaceId+"/typing/old")
			require.Contains(t, fx.engine.subs, testSpaceId+"/typing/shared")
			require.Equal(t, testSpaceId, fx.subs["sub"].spaceId)
			require.Len(t, fx.patterns[testSpaceId]["typing/shared"].subIds, 2)
			require.NoError(t, fx.Unsubscribe("sub"))
			require.Len(t, fx.engine.subs, 1)
			require.NoError(t, fx.Unsubscribe("other-sub"))
			require.Empty(t, fx.engine.subs)
		})
	}
}

// testSpaceLoader substitutes only the local Space loading boundary.
type testSpaceLoader struct {
	space.Service
	load func(context.Context, string) error
}

func (s *testSpaceLoader) Get(ctx context.Context, id string) (clientspace.Space, error) {
	if s.load != nil {
		return nil, s.load(ctx, id)
	}
	return nil, ctx.Err()
}
