package filesync

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore/mock_rpcstore"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

func newNodeUsageStore(t *testing.T) keyvaluestore.Store[NodeUsage] {
	provider, err := anystoreprovider.NewInPath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = provider.Close(context.Background()) })
	return keyvaluestore.NewJsonFromCollection[NodeUsage](provider.GetSystemCollection())
}

// noopSender drops the usage events sent on a change
type noopSender struct{ event.Sender }

func (noopSender) Broadcast(*pb.Event) {}

func TestRequestNodeUsageUpdate(t *testing.T) {
	t.Run("bounded and non-blocking while the file node is blocked", func(t *testing.T) {
		rpc := mock_rpcstore.NewMockRpcStore(t)
		var calls atomic.Int32
		blocked := make(chan struct{})
		release := make(chan struct{})
		rpc.EXPECT().AccountInfo(mock.Anything).RunAndReturn(func(ctx context.Context) (*fileproto.AccountInfoResponse, error) {
			if calls.Add(1) == 2 {
				// the triggered update hangs on the file node
				close(blocked)
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
			return &fileproto.AccountInfoResponse{}, nil
		})

		loopCtx, cancel := context.WithCancel(context.Background())
		s := &fileSync{
			rpcStore:          rpc,
			loopCtx:           loopCtx,
			loopCancel:        cancel,
			closeWg:           &sync.WaitGroup{},
			nodeUsageUpdateCh: make(chan struct{}, 1),
		}
		// cached usage equal to the node's answer: no store writes or events
		s.setNodeUsageInMemory(NodeUsage{})
		s.closeWg.Add(1)
		go s.runNodeUsageUpdater()
		defer func() {
			cancel()
			s.closeWg.Wait()
		}()

		assert.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second, time.Millisecond, "precache")
		s.RequestNodeUsageUpdate()
		<-blocked

		sent := make(chan struct{})
		go func() {
			for range 1000 {
				s.RequestNodeUsageUpdate()
			}
			close(sent)
		}()
		select {
		case <-sent:
		case <-time.After(time.Second):
			t.Fatal("RequestNodeUsageUpdate blocked on a busy updater")
		}
		assert.Equal(t, 1, len(s.nodeUsageUpdateCh), "exactly one request pending")

		close(release)
		// the pending request runs once, the rest coalesced into it
		assert.Eventually(t, func() bool { return calls.Load() == 3 }, time.Second, time.Millisecond)
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, int32(3), calls.Load())
	})

	t.Run("no-op without a running updater", func(t *testing.T) {
		s := New().(*fileSync)
		done := make(chan struct{})
		go func() {
			s.RequestNodeUsageUpdate()
			s.RequestNodeUsageUpdate()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("RequestNodeUsageUpdate blocked")
		}
	})
}

func TestNodeUsageCadence(t *testing.T) {
	assert.Equal(t, 10*time.Second, nodeUsageInterval(false))
	assert.Equal(t, time.Minute, nodeUsageInterval(true))

	newSync := func(t *testing.T, infos ...*fileproto.AccountInfoResponse) *fileSync {
		rpc := mock_rpcstore.NewMockRpcStore(t)
		for _, info := range infos {
			if info == nil {
				rpc.EXPECT().AccountInfo(mock.Anything).Return(nil, errors.New("offline")).Once()
				continue
			}
			rpc.EXPECT().AccountInfo(mock.Anything).Return(info, nil).Once()
		}
		loopCtx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		s := &fileSync{
			rpcStore:       rpc,
			loopCtx:        loopCtx,
			nodeUsageStore: newNodeUsageStore(t),
			eventSender:    noopSender{},
		}
		s.setNodeUsageInMemory(NodeUsage{})
		return s
	}

	t.Run("unreachable node: slow", func(t *testing.T) {
		assert.True(t, newSync(t, nil).updateNodeUsageTick())
	})
	t.Run("usage moved: fast", func(t *testing.T) {
		assert.False(t, newSync(t, &fileproto.AccountInfoResponse{LimitBytes: 100, TotalUsageBytes: 10}).updateNodeUsageTick())
	})
	t.Run("usage idle: slow", func(t *testing.T) {
		s := newSync(t, &fileproto.AccountInfoResponse{}, &fileproto.AccountInfoResponse{})
		s.updateNodeUsageTick()
		assert.True(t, s.updateNodeUsageTick())
	})
}

type fakeTicker struct{ resets []time.Duration }

func (f *fakeTicker) Reset(d time.Duration) { f.resets = append(f.resets, d) }

func TestSwitchNodeUsageCadence(t *testing.T) {
	tk := &fakeTicker{}
	slow := switchNodeUsageCadence(tk, false, true)
	assert.True(t, slow)
	slow = switchNodeUsageCadence(tk, slow, true)
	assert.True(t, slow)
	slow = switchNodeUsageCadence(tk, slow, false)
	assert.False(t, slow)
	assert.Equal(t, []time.Duration{time.Minute, 10 * time.Second}, tk.resets, "reset only on a change")
}
