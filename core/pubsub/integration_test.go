package pubsub

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/acl/syncacl"
	anysyncpubsub "github.com/anyproto/any-sync/commonspace/pubsub"
	"github.com/anyproto/any-sync/commonspace/pubsub/pubsubproto"
	"github.com/anyproto/any-sync/testutil/accounttest"
	"github.com/stretchr/testify/require"
	"storj.io/drpc"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

type testACL struct {
	syncacl.SyncAcl
	acl list.AclList
}

func (a testACL) RLock()                   { a.acl.RLock() }
func (a testACL) RUnlock()                 { a.acl.RUnlock() }
func (a testACL) AclState() *list.AclState { return a.acl.AclState() }

type testCoreSpace struct {
	commonspace.Space
	acl syncacl.SyncAcl
}

func (s testCoreSpace) Acl() syncacl.SyncAcl { return s.acl }
func (s testCoreSpace) Close() error         { return nil }

type testSpaceCore struct {
	spacecore.SpaceCoreService
	cache ocache.OCache
	loads atomic.Int32
}

func (s *testSpaceCore) Get(ctx context.Context, id string) (*spacecore.AnySpace, error) {
	v, err := s.cache.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return v.(*spacecore.AnySpace), nil
}

func (s *testSpaceCore) Pick(ctx context.Context, id string) (*spacecore.AnySpace, error) {
	v, err := s.cache.Pick(ctx, id)
	if err != nil {
		return nil, err
	}
	return v.(*spacecore.AnySpace), nil
}

type testEvents struct {
	event.Sender
	ch chan *pb.Event
}

func (s testEvents) Broadcast(e *pb.Event) { s.ch <- e }

func startTestEngine(t *testing.T, acc *accountdata.AccountKeys, deps anysyncpubsub.Deps) anysyncpubsub.Service {
	t.Helper()
	engine := anysyncpubsub.New(deps)
	a := new(app.App)
	a.Register(accounttest.NewWithAcc(acc)).Register(engine)
	require.NoError(t, a.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, a.Close(context.Background())) })
	return engine
}

func newCryptoService(t *testing.T) (*service, *testSpaceCore, *accountdata.AccountKeys, testEvents) {
	t.Helper()
	acc, err := accountdata.NewRandom()
	require.NoError(t, err)
	acl, err := list.NewInMemoryDerivedAcl(testSpaceId, acc)
	require.NoError(t, err)
	core := &testSpaceCore{}
	core.cache = ocache.New(func(ctx context.Context, id string) (ocache.Object, error) {
		core.loads.Add(1)
		return &spacecore.AnySpace{Space: testCoreSpace{acl: testACL{acl: acl}}}, nil
	})
	t.Cleanup(func() { require.NoError(t, core.cache.Close()) })
	events := testEvents{ch: make(chan *pb.Event, 16)}
	s := New().(*service)
	s.spaceCore = core
	s.spaces = &testSpaceLoader{load: func(ctx context.Context, id string) error {
		_, err := core.Get(ctx, id)
		return err
	}}
	s.eventSender = events
	deps := s.EngineDeps()
	deps.Peers = nil
	s.engine = startTestEngine(t, acc, deps)
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	return s, core, acc, events
}

// Feed an actual signed wire frame through the released engine's inbound path.
func receiveEncrypted(t *testing.T, engine anysyncpubsub.Service, acc *accountdata.AccountKeys, keyID string, payload []byte) {
	t.Helper()
	frame := &pubsubproto.Publish{
		SpaceId: testSpaceId, Topic: "typing/object", MsgId: make([]byte, 16),
		KeyId: keyID, Payload: payload, TimestampMilli: time.Now().UnixMilli(),
	}
	var err error
	frame.Identity, err = acc.SignKey.GetPublic().Marshall()
	require.NoError(t, err)
	data := []byte("anysync:pubsub:v1")
	for _, field := range [][]byte{[]byte(frame.SpaceId), []byte(frame.Topic), frame.MsgId, []byte(frame.KeyId)} {
		data = binary.LittleEndian.AppendUint32(data, uint32(len(field)))
		data = append(data, field...)
	}
	data = binary.LittleEndian.AppendUint64(data, uint64(frame.TimestampMilli))
	data = append(data, frame.Payload...)
	frame.Signature, err = acc.SignKey.Sign(data)
	require.NoError(t, err)
	handler := engine.(interface {
		HandleMessage(context.Context, string, drpc.Message) error
	})
	require.NoError(t, handler.HandleMessage(context.Background(), "remote", &pubsubproto.PubSubMessage{
		Content: &pubsubproto.PubSubMessage_Publish{Publish: frame},
	}))
}

func nextPubsubEvent(t *testing.T, events testEvents) *pb.EventPubsubMessage {
	t.Helper()
	select {
	case ev := <-events.ch:
		return ev.Messages[0].GetPubsubMessage()
	case <-time.After(2 * time.Second):
		t.Fatal("pubsub message was not delivered")
		return nil
	}
}

func TestSubscribeLoadsColdSpaceForInboundDelivery(t *testing.T) {
	s, core, acc, events := newCryptoService(t)
	// Remote membership probes must not load a Space.
	require.Error(t, s.CheckMember(context.Background(), testSpaceId, acc.SignKey.GetPublic()))
	require.EqualValues(t, 0, core.loads.Load())
	_, err := s.Subscribe(context.Background(), testSpaceId, []string{"typing/object"}, "sub")
	require.NoError(t, err)
	require.EqualValues(t, 1, core.loads.Load())
	keyID, encrypted, err := s.Encrypt(testSpaceId, []byte("hello"))
	require.NoError(t, err)
	receiveEncrypted(t, s.engine, acc, keyID, encrypted)
	message := nextPubsubEvent(t, events)
	require.Equal(t, []byte("hello"), message.Payload)
	require.Equal(t, []string{"sub"}, message.SubIds)
}

func TestEncryptedPayloadBoundary(t *testing.T) {
	for _, size := range []int{65508, 65509, 65536} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			s, _, acc, events := newCryptoService(t)
			_, err := s.Subscribe(context.Background(), testSpaceId, []string{"typing/object"}, "sub")
			require.NoError(t, err)
			payload := make([]byte, size)
			err = s.Publish(context.Background(), testSpaceId, "typing/object", payload)
			if size > 65508 {
				require.ErrorIs(t, err, pubsubproto.ErrInvalidMessage)
				select {
				case <-events.ch:
					t.Fatal("rejected oversized payload was echoed")
				case <-time.After(30 * time.Millisecond):
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, payload, nextPubsubEvent(t, events).Payload)
			keyID, encrypted, err := s.Encrypt(testSpaceId, payload)
			require.NoError(t, err)
			require.Len(t, encrypted, 65536)
			receiveEncrypted(t, s.engine, acc, keyID, encrypted)
			require.Equal(t, payload, nextPubsubEvent(t, events).Payload)
		})
	}
}

func TestReplaceSubscriptionAtCapacity(t *testing.T) {
	s, _, _, events := newCryptoService(t)
	patterns := make([]string, 99)
	for i := range patterns {
		patterns[i] = fmt.Sprintf("other/%d", i)
	}
	_, err := s.Subscribe(context.Background(), testSpaceId, patterns, "other")
	require.NoError(t, err)
	_, err = s.Subscribe(context.Background(), testSpaceId, []string{"typing/old"}, "replace")
	require.NoError(t, err)
	_, err = s.Subscribe(context.Background(), testSpaceId, []string{"typing/old", "typing/new"}, "replace")
	require.ErrorIs(t, err, pubsubproto.ErrTooManyTopics)
	require.NoError(t, s.Publish(context.Background(), testSpaceId, "typing/old", []byte("still subscribed")))
	require.Equal(t, []string{"replace"}, nextPubsubEvent(t, events).SubIds)
	// Equal-size replacements and duplicate patterns must work at capacity.
	_, err = s.Subscribe(context.Background(), testSpaceId, []string{"typing/new", "typing/new"}, "replace")
	require.NoError(t, err)
	require.NoError(t, s.Publish(context.Background(), testSpaceId, "typing/new", []byte("replacement")))
	require.Equal(t, []string{"replace"}, nextPubsubEvent(t, events).SubIds)
	require.NoError(t, s.Unsubscribe("replace"))
}

type closeBarrierEngine struct {
	anysyncpubsub.Service
	entered, resume chan struct{}
}

func (e *closeBarrierEngine) CloseSpace(id string) {
	close(e.entered)
	<-e.resume
	e.Service.CloseSpace(id)
}

func TestCloseSpaceSerializesWithSubscribe(t *testing.T) {
	s, _, _, events := newCryptoService(t)
	barrier := &closeBarrierEngine{Service: s.engine, entered: make(chan struct{}), resume: make(chan struct{})}
	s.engine = barrier
	_, err := s.Subscribe(context.Background(), testSpaceId, []string{"typing/object"}, "old")
	require.NoError(t, err)
	closed := make(chan struct{})
	go func() { s.CloseSpace(testSpaceId); close(closed) }()
	<-barrier.entered
	subscribed := make(chan error, 1)
	go func() {
		_, err := s.Subscribe(context.Background(), testSpaceId, []string{"typing/object"}, "new")
		subscribed <- err
	}()
	var returnedEarly bool
	select {
	case <-subscribed:
		returnedEarly = true
	case <-time.After(30 * time.Millisecond):
	}
	close(barrier.resume)
	<-closed
	require.False(t, returnedEarly, "Subscribe returned before the old engine subscriptions were closed")
	require.NoError(t, <-subscribed)
	require.NoError(t, s.Publish(context.Background(), testSpaceId, "typing/object", []byte("after close")))
	require.Equal(t, []string{"new"}, nextPubsubEvent(t, events).SubIds)
}
