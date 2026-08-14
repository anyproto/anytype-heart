package adapter

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

// The §13.4 lifecycle harness: a real adapter service over fakes, able to
// drive Import, Close mid-flight, and the startup sweep — the paths two
// review rounds could only reason about. The engine itself is scripted (its
// behavior has its own suite); what runs for real here is everything the
// adapter owns: run-dir lifecycle, the active registry, suspend, events.

type fakeSpaceGetter struct {
	spc clientspace.Space
	err error
}

func (f *fakeSpaceGetter) Get(ctx context.Context, spaceId string) (clientspace.Space, error) {
	return f.spc, f.err
}

type fakeProcesses struct{}

func (fakeProcesses) ProcessAdd(p process.Process) error { return nil }

// fakeFileSync embeds the interface: only the two import-event methods are
// real, anything else panics loudly if the adapter starts calling it.
type fakeFileSync struct {
	filesync.FileSync
	mu      sync.Mutex
	sent    int
	cleared int
}

func (f *fakeFileSync) SendImportEvents() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent++
}

func (f *fakeFileSync) ClearImportEvents() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleared++
}

type lifecycleFixture struct {
	t       *testing.T
	service *service
	repo    string

	eventsMu sync.Mutex
	events   []*pb.Event
}

func newLifecycleFixture(t *testing.T) *lifecycleFixture {
	fx := &lifecycleFixture{t: t, repo: t.TempDir()}
	componentCtx, componentCancel := context.WithCancel(context.Background())
	fx.service = &service{
		config:          &config.Config{RepoPath: fx.repo},
		spaceService:    &fakeSpaceGetter{spc: mock_clientspace.NewMockSpace(t)},
		processes:       fakeProcesses{},
		objects:         &sweepDeleter{},
		fileSync:        &fakeFileSync{},
		componentCtx:    componentCtx,
		componentCancel: componentCancel,
		eventSender: event.NewCallbackSender(func(e *pb.Event) {
			fx.eventsMu.Lock()
			defer fx.eventsMu.Unlock()
			fx.events = append(fx.events, e)
		}),
	}
	t.Cleanup(componentCancel)
	return fx
}

// script installs the scripted engine and returns the markdown request that
// drives one run through it.
func (fx *lifecycleFixture) script(run engineRunFn) *pb.RpcObjectImportRequest {
	fx.service.engineRunner = run
	dir := fx.t.TempDir()
	require.NoError(fx.t, os.WriteFile(filepath.Join(dir, "page.md"), []byte("# hi"), 0o600))
	return &pb.RpcObjectImportRequest{
		SpaceId:    "space-1",
		Type:       model.Import_Markdown,
		NoProgress: true,
		Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
			MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{dir}},
		},
	}
}

func (fx *lifecycleFixture) finishEvents() int {
	fx.eventsMu.Lock()
	defer fx.eventsMu.Unlock()
	count := 0
	for _, e := range fx.events {
		for _, msg := range e.Messages {
			if msg.GetImportFinish() != nil {
				count++
			}
		}
	}
	return count
}

func (fx *lifecycleFixture) waitRuns() {
	done := make(chan struct{})
	go func() {
		fx.service.runs.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		fx.t.Fatal("runs did not drain")
	}
}

func runDirs(t *testing.T, repo string) []string {
	dirs, err := runstore.ListRunDirs(runstore.RunsRoot(repo))
	require.NoError(t, err)
	return dirs
}

func TestLifecycleHappyPath(t *testing.T) {
	t.Run("a completed import disposes its dir and broadcasts one finish event", func(t *testing.T) {
		// given
		fx := newLifecycleFixture(t)
		var dir string
		req := fx.script(func(ctx context.Context, request importv2.Request, converter importv2.Converter, spc clientspace.Space, lc *runLifecycle, progress process.Progress) *importv2.Result {
			dir = lc.store.Dir()
			require.NoError(t, lc.store.RecordCreated(ctx, "page.md", "obj-1"))
			return &importv2.Result{Created: 1}
		})

		// when
		fx.service.Import(req)
		fx.waitRuns()

		// then
		_, err := os.Stat(dir)
		assert.True(t, os.IsNotExist(err), "completed run dir must be disposed")
		assert.Equal(t, 1, fx.finishEvents())
		assert.False(t, runstore.IsActive(dir), "registry must be released")
	})

	t.Run("two concurrent imports settle independently", func(t *testing.T) {
		// given
		fx := newLifecycleFixture(t)
		var mu sync.Mutex
		dirs := map[string]struct{}{}
		barrier := make(chan struct{})
		started := make(chan struct{}, 2)
		req := fx.script(func(ctx context.Context, request importv2.Request, converter importv2.Converter, spc clientspace.Space, lc *runLifecycle, progress process.Progress) *importv2.Result {
			mu.Lock()
			dirs[lc.store.Dir()] = struct{}{}
			mu.Unlock()
			started <- struct{}{}
			<-barrier // both runs live at once
			return &importv2.Result{Created: 1}
		})

		// when
		fx.service.Import(req)
		fx.service.Import(req)
		for i := 0; i < 2; i++ {
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("second import never started")
			}
		}
		close(barrier)
		fx.waitRuns()

		// then: two distinct dirs, both disposed, both events delivered
		assert.Len(t, dirs, 2)
		for dir := range dirs {
			_, err := os.Stat(dir)
			assert.True(t, os.IsNotExist(err))
		}
		assert.Equal(t, 2, fx.finishEvents())
		assert.Empty(t, runDirs(t, fx.repo))
	})
}

func TestLifecycleClose(t *testing.T) {
	t.Run("Close mid-import suspends: dir kept, no finish event, prompt return", func(t *testing.T) {
		// given — the scripted engine emulates the real contract: on a
		// suspend cause it stops without compensating and says so.
		fx := newLifecycleFixture(t)
		var dir string
		started := make(chan struct{})
		req := fx.script(func(ctx context.Context, request importv2.Request, converter importv2.Converter, spc clientspace.Space, lc *runLifecycle, progress process.Progress) *importv2.Result {
			dir = lc.store.Dir()
			require.NoError(t, lc.store.RecordCreated(ctx, "page.md", "obj-1"))
			close(started)
			<-ctx.Done()
			return &importv2.Result{
				Err:       importv2.Fatal(importv2.IssueCancelled, context.Cause(ctx)),
				Suspended: errors.Is(context.Cause(ctx), importv2.ErrSuspended),
			}
		})
		fx.service.Import(req)
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("import never started")
		}

		// when
		closeStart := time.Now()
		require.NoError(t, fx.service.Close(context.Background()))

		// then
		assert.Less(t, time.Since(closeStart), 10*time.Second, "close must not burn the grace period")
		assert.Zero(t, fx.finishEvents(), "a suspended run is not over — no finish event")
		store, err := runstore.Open(context.Background(), dir)
		require.NoError(t, err, "the dir must survive for the startup sweep")
		defer store.Close()
		manifest, err := store.Manifest(context.Background())
		require.NoError(t, err)
		assert.Equal(t, runstore.StateSuspended, manifest.State)
		inputs, err := store.CompensationInputs(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-1"}, inputs.Created, "effects must survive the suspend")
	})
}

func TestLifecycleInvariants(t *testing.T) {
	t.Run("an in-process compensation that leaked keeps the dir for the sweep", func(t *testing.T) {
		// given — Invariant 2: the sweep retains leaked dirs; the in-process
		// finishRun path must follow the same rule, or a retryable leak
		// becomes a permanent orphan.
		fx := newLifecycleFixture(t)
		var dir string
		req := fx.script(func(ctx context.Context, request importv2.Request, converter importv2.Converter, spc clientspace.Space, lc *runLifecycle, progress process.Progress) *importv2.Result {
			dir = lc.store.Dir()
			require.NoError(t, lc.store.RecordCreated(ctx, "page.md", "obj-1"))
			// the engine compensated and could not delete obj-1
			return &importv2.Result{
				Err:         importv2.Fatal(importv2.IssueStoreError, assert.AnError),
				Compensated: 0,
				Leaked:      1,
			}
		})

		// when
		fx.service.Import(req)
		fx.waitRuns()

		// then: the dir survives in the compensating state; the failure is
		// still delivered (the event carries the run's end)
		store, err := runstore.Open(context.Background(), dir)
		require.NoError(t, err, "a leaked compensation must keep the dir for retry")
		defer store.Close()
		manifest, err := store.Manifest(context.Background())
		require.NoError(t, err)
		assert.Equal(t, runstore.StateCompensating, manifest.State)
		assert.Equal(t, 1, fx.finishEvents())
	})

	t.Run("a panicking engine run cannot leak the active registry", func(t *testing.T) {
		// given — Invariant 3: release is deferred, so a panic between
		// beginRun and finishRun must not leave the dir marked active (which
		// would block it from ever being swept).
		fx := newLifecycleFixture(t)
		var dir string
		req := fx.script(func(ctx context.Context, request importv2.Request, converter importv2.Converter, spc clientspace.Space, lc *runLifecycle, progress process.Progress) *importv2.Result {
			dir = lc.store.Dir()
			panic("injected engine panic")
		})

		// when
		fx.service.Import(req)
		fx.waitRuns()

		// then
		require.NotEmpty(t, dir)
		assert.False(t, runstore.IsActive(dir),
			"the registry entry must be released on the panic path")
		assert.DirExists(t, dir, "the dir itself survives for the sweep")
	})
}

func TestLifecycleSweep(t *testing.T) {
	t.Run("the service-level sweep settles a crashed run end to end", func(t *testing.T) {
		// given: a dir a previous process left behind in the running state
		fx := newLifecycleFixture(t)
		ctx := context.Background()
		dir := filepath.Join(runstore.RunsRoot(fx.repo), "crashed")
		store, err := runstore.Create(ctx, dir, runstore.Manifest{RunId: "crashed", SpaceId: "space-1"})
		require.NoError(t, err)
		require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-1"))
		require.NoError(t, store.Close())
		deleter := fx.service.objects.(*sweepDeleter)

		// when: driven through the service entry point, not sweepRuns directly
		fx.service.sweepAbandoned()

		// then
		assert.Equal(t, []string{"obj-1"}, deleter.deleted)
		_, err = os.Stat(dir)
		assert.True(t, os.IsNotExist(err))
	})
}
