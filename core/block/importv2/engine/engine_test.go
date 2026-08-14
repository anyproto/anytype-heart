package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/identity"
	"github.com/anyproto/anytype-heart/core/block/importv2/persist"
	"github.com/anyproto/anytype-heart/core/block/importv2/resolve"
	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

// fakeIdentity assigns deterministic ids without store or space.
type fakeIdentity struct {
	events []string
	mu       sync.Mutex
	claims   []importv2.IdentityClaim
	assigned map[string]bool
	files    map[string]*fileState
	counter  int
}

type fileState struct {
	done chan struct{}
	id   string
	err  error
}

func newFakeIdentity() *fakeIdentity {
	return &fakeIdentity{files: map[string]*fileState{}, assigned: map[string]bool{}}
}

func (f *fakeIdentity) Claim(ctx context.Context, c importv2.IdentityClaim) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claims = append(f.claims, c)
	f.events = append(f.events, "claim:"+c.SourceKey)
	return nil
}

func (f *fakeIdentity) FlushClaims(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, "flush")
	return nil
}

func (f *fakeIdentity) Assign(sourceKey string) (identity.Assignment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counter++
	f.assigned[sourceKey] = true
	return identity.Assignment{Id: fmt.Sprintf("id-%s", sourceKey)}, nil
}

func (f *fakeIdentity) UnassignedClaims() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var keys []string
	for _, c := range f.claims {
		if !f.assigned[c.SourceKey] {
			keys = append(keys, c.SourceKey)
		}
	}
	return keys
}

func (f *fakeIdentity) AssignDerived(ctx context.Context, o *importv2.Object) (identity.Assignment, error) {
	return identity.Assignment{Id: "derived-" + o.SourceKey, InternalKey: o.Payload.Key}, nil
}

func (f *fakeIdentity) RegisterFile(sourceKey string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.files[sourceKey]; !ok {
		f.files[sourceKey] = &fileState{done: make(chan struct{})}
	}
}

func (f *fakeIdentity) CompleteFile(sourceKey, id string, err error) {
	f.mu.Lock()
	state, ok := f.files[sourceKey]
	f.mu.Unlock()
	if !ok {
		return
	}
	select {
	case <-state.done:
	default:
		state.id, state.err = id, err
		close(state.done)
	}
}

func (f *fakeIdentity) Resolve(sourceKey string) (string, bool) {
	return "id-" + sourceKey, true
}

// fakePersister counts persists; optional per-key behavior.
type fakePersister struct {
	mu        sync.Mutex
	persisted []string
	delay     time.Duration
	delayKeys map[string]time.Duration
	failKeys  map[string]error
	// failOnCancelKeys blocks the persist until the run ctx dies, then
	// returns the given error (NOT ctx.Err()) — the shape of a durable-
	// journal failure racing a shutdown.
	failOnCancelKeys map[string]error
	panicKeys        map[string]bool
	journal          *persist.Journal
	// filePaths/fileOpenSeen capture the file source as it arrives at
	// persist time — the pass-2 drain assertions read them.
	filePaths    map[string]string
	fileOpenSeen map[string]bool
	// observe, when set, fires at the top of every Persist call.
	observe func()
	// observeKeyed fires with the object's source key at the top of Persist.
	observeKeyed func(sourceKey string)
	// fileReady simulates reference resolution: objects whose key starts
	// with "ref-" block until the file object's persist closes the channel
	// (as resolver.ResolveRef blocks on the identity future in production).
	fileReady chan struct{}
}

func (f *fakePersister) Persist(ctx context.Context, o *importv2.Object, target persist.Target, report func(importv2.Issue)) (persist.Outcome, error) {
	if f.observe != nil {
		f.observe()
	}
	if f.observeKeyed != nil {
		f.observeKeyed(o.SourceKey)
	}
	// Fires outside the mutex: a recovered panic must not strand the lock.
	if f.panicKeys[o.SourceKey] {
		panic("injected persist panic: " + o.SourceKey)
	}
	if err, ok := f.failOnCancelKeys[o.SourceKey]; ok {
		<-ctx.Done()
		return persist.Outcome{}, err
	}
	if f.fileReady != nil {
		if o.File != nil {
			f.mu.Lock()
			select {
			case <-f.fileReady:
			default:
				close(f.fileReady)
			}
			f.mu.Unlock()
		} else if strings.HasPrefix(o.SourceKey, "ref-") {
			select {
			case <-f.fileReady:
			case <-ctx.Done():
				return persist.Outcome{}, ctx.Err()
			case <-time.After(5 * time.Second):
				return persist.Outcome{}, errors.New("deadlock: referencing object never saw its file complete")
			}
		}
	}
	delay := f.delay
	if d, ok := f.delayKeys[o.SourceKey]; ok {
		delay = d
	}
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return persist.Outcome{}, ctx.Err()
		}
	}
	f.mu.Lock()
	err := f.failKeys[o.SourceKey]
	if err == nil {
		f.persisted = append(f.persisted, o.SourceKey)
	}
	if o.File != nil {
		if f.filePaths == nil {
			f.filePaths = map[string]string{}
			f.fileOpenSeen = map[string]bool{}
		}
		f.filePaths[o.SourceKey] = o.File.Path
		f.fileOpenSeen[o.SourceKey] = o.File.Open != nil
	}
	f.mu.Unlock()
	if err != nil {
		return persist.Outcome{}, err
	}
	id := target.Id
	if id == "" {
		id = "file-" + o.SourceKey
	}
	if f.journal != nil {
		if err := f.journal.CreatedObject(o.SourceKey, id); err != nil {
			return persist.Outcome{}, err
		}
	}
	return persist.Outcome{Id: id, Action: persist.ActionCreated}, nil
}

// scriptConverter enumerates and emits a fixed object list.
type scriptConverter struct {
	objects  []*importv2.Object
	rootSpec importv2.RootSpec
}

func (c *scriptConverter) Name() string { return "script" }

func (c *scriptConverter) EnumerateIdentities(ctx context.Context, yield func(importv2.IdentityClaim) error) error {
	for _, o := range c.objects {
		if isDerivedClass(o.SbType) || isFileClass(o.SbType) {
			continue
		}
		if err := yield(importv2.IdentityClaim{SourceKey: o.SourceKey, SbType: o.SbType}); err != nil {
			return err
		}
	}
	return nil
}

func (c *scriptConverter) Convert(ctx context.Context, sink importv2.Sink) (importv2.RootSpec, error) {
	for _, o := range c.objects {
		if err := sink.Object(ctx, o); err != nil {
			return importv2.RootSpec{}, err
		}
	}
	return c.rootSpec, nil
}

// gapConverter claims extra keys in pass 1 that it never emits in pass 2 —
// the silent-drop shape the claims-reconciliation invariant exists to catch.
// Keys in issuedKeys get a warning issue instead (a loud skip).
type gapConverter struct {
	scriptConverter
	gapKeys  []string
	loudKeys []string
}

func (c *gapConverter) EnumerateIdentities(ctx context.Context, yield func(importv2.IdentityClaim) error) error {
	if err := c.scriptConverter.EnumerateIdentities(ctx, yield); err != nil {
		return err
	}
	for _, key := range append(append([]string{}, c.gapKeys...), c.loudKeys...) {
		if err := yield(importv2.IdentityClaim{SourceKey: key, SbType: coresb.SmartBlockTypePage}); err != nil {
			return err
		}
	}
	return nil
}

func (c *gapConverter) Convert(ctx context.Context, sink importv2.Sink) (importv2.RootSpec, error) {
	for _, key := range c.loudKeys {
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, key, "deliberately skipped"))
	}
	return c.scriptConverter.Convert(ctx, sink)
}

// issueConverter emits one fixed issue before streaming its objects.
type issueConverter struct {
	scriptConverter
	issue importv2.Issue
}

func (c *issueConverter) Convert(ctx context.Context, sink importv2.Sink) (importv2.RootSpec, error) {
	sink.Issue(c.issue)
	return c.scriptConverter.Convert(ctx, sink)
}

// panicConvertConverter emits its objects, then panics inside Convert.
type panicConvertConverter struct {
	scriptConverter
}

func (c *panicConvertConverter) Convert(ctx context.Context, sink importv2.Sink) (importv2.RootSpec, error) {
	if _, err := c.scriptConverter.Convert(ctx, sink); err != nil {
		return importv2.RootSpec{}, err
	}
	panic("injected converter panic")
}

// panicEnumConverter panics during the identity pass (main goroutine).
type panicEnumConverter struct{}

func (c *panicEnumConverter) Name() string { return "panic-enum" }

func (c *panicEnumConverter) EnumerateIdentities(ctx context.Context, yield func(importv2.IdentityClaim) error) error {
	panic("injected enumerate panic")
}

func (c *panicEnumConverter) Convert(ctx context.Context, sink importv2.Sink) (importv2.RootSpec, error) {
	return importv2.RootSpec{}, nil
}

type fakeCollectionFactory struct {
	name    string
	members []string
}

func (f *fakeCollectionFactory) MakeCollection(name string, memberSourceKeys []string) (*importv2.Object, error) {
	f.name = name
	f.members = memberSourceKeys
	return &importv2.Object{
		SourceKey: "root-collection",
		SbType:    coresb.SmartBlockTypePage,
		Payload:   &importv2.Snapshot{Details: domain.NewDetails()},
	}, nil
}

type engineFixture struct {
	identity  *fakeIdentity
	persister *fakePersister
	journal   *persist.Journal
	deps      Deps
}

func pageObj(key string, root bool) *importv2.Object {
	return &importv2.Object{
		SourceKey:       key,
		SbType:          coresb.SmartBlockTypePage,
		Payload:         &importv2.Snapshot{Details: domain.NewDetails()},
		IsRootCandidate: root,
	}
}

func fileObj(key string) *importv2.Object {
	return &importv2.Object{
		SourceKey: key,
		SbType:    coresb.SmartBlockTypeFileObject,
		Payload:   &importv2.Snapshot{Details: domain.NewDetails()},
		File:      &importv2.FileSource{Path: "/tmp/x", Name: key},
	}
}

func newEngineFixture() *engineFixture {
	journal := persist.NewJournal()
	fx := &engineFixture{
		identity:  newFakeIdentity(),
		persister: &fakePersister{journal: journal, failKeys: map[string]error{}},
		journal:   journal,
	}
	fx.deps = Deps{
		Identity:  fx.identity,
		Persister: fx.persister,
		Journal:   journal,
		Objects:   &deleterFake{},
		Formats:   resolve.NewFormats(),
		Keys:      NewKeyTable(),
	}
	return fx
}

type deleterFake struct {
	mu       sync.Mutex
	deleted  []string
	panicIds map[string]bool
}

func (d *deleterFake) GetObject(ctx context.Context, objectId string) (smartblock.SmartBlock, error) {
	return nil, errors.New("not used")
}

func (d *deleterFake) GetObjectByFullID(ctx context.Context, id domain.FullID) (smartblock.SmartBlock, error) {
	return nil, errors.New("not used")
}

func (d *deleterFake) DeleteObject(objectId string) error {
	if d.panicIds[objectId] {
		panic("injected delete panic: " + objectId)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deleted = append(d.deleted, objectId)
	return nil
}

func TestRunHappyPath(t *testing.T) {
	t.Run("streams, persists, builds root collection", func(t *testing.T) {
		// given
		fx := newEngineFixture()
		factory := &fakeCollectionFactory{}
		fx.deps.Collection = factory
		converter := &scriptConverter{
			objects: []*importv2.Object{
				pageObj("a.md", true),
				pageObj("b.md", true),
			},
			rootSpec: importv2.RootSpec{CollectionName: "Markdown Import"},
		}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, int64(2), result.Created, "root collection is not counted")
		assert.Equal(t, "Markdown Import", factory.name)
		assert.Equal(t, []string{"a.md", "b.md"}, factory.members)
		assert.Equal(t, "id-root-collection", result.RootCollectionId)
	})

	t.Run("no objects is fatal", func(t *testing.T) {
		// given
		fx := newEngineFixture()
		converter := &scriptConverter{}

		// when
		result := Run(context.Background(), importv2.Request{}, converter, fx.deps)

		// then
		issue := importv2.AsIssue(result.Err, importv2.SeverityFatal, importv2.IssueStoreError)
		assert.Equal(t, importv2.IssueNoObjects, issue.Code)
	})

	t.Run("root object key routes the widget without a collection", func(t *testing.T) {
		// given
		fx := newEngineFixture()
		converter := &scriptConverter{
			objects:  []*importv2.Object{pageObj("dir", true)},
			rootSpec: importv2.RootSpec{RootObjectKey: "dir"},
		}

		// when
		result := Run(context.Background(), importv2.Request{}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, "id-dir", result.RootCollectionId)
	})
}

func TestRunModes(t *testing.T) {
	t.Run("continue-on-error skips the failed object", func(t *testing.T) {
		// given
		fx := newEngineFixture()
		fx.persister.failKeys["bad.md"] = assert.AnError
		converter := &scriptConverter{objects: []*importv2.Object{
			pageObj("a.md", false), pageObj("bad.md", false), pageObj("c.md", false),
		}}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, int64(2), result.Created)
		assert.Equal(t, int64(1), result.Failed)
		assert.Zero(t, result.Compensated)
	})

	t.Run("all-or-nothing aborts and compensates", func(t *testing.T) {
		// given — the failing object is delayed so earlier objects are
		// already committed when the abort fires.
		fx := newEngineFixture()
		fx.persister.failKeys["bad.md"] = assert.AnError
		fx.persister.delayKeys = map[string]time.Duration{"bad.md": 100 * time.Millisecond}
		objects := []*importv2.Object{pageObj("a.md", false), pageObj("b.md", false), pageObj("bad.md", false)}
		converter := &scriptConverter{objects: objects}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeAllOrNothing}, converter, fx.deps)

		// then
		require.Error(t, result.Err)
		assert.Positive(t, result.Compensated, "created objects must be deleted on abort")
		assert.Equal(t, result.Compensated, len(fx.deps.Objects.(*deleterFake).deleted))
		assert.Empty(t, result.RootCollectionId)
	})

	t.Run("OnCompensating fires before any deletion, and only on abort", func(t *testing.T) {
		// given — the durable manifest must say "compensating" BEFORE the
		// first delete, so a crash mid-cleanup is finished by the sweep
		// (spec §6.5).
		fx := newEngineFixture()
		deleter := fx.deps.Objects.(*deleterFake)
		var deletedWhenMarked int
		marked := 0
		fx.deps.OnCompensating = func() {
			marked++
			deleter.mu.Lock()
			deletedWhenMarked = len(deleter.deleted)
			deleter.mu.Unlock()
		}
		fx.persister.failKeys["bad.md"] = assert.AnError
		fx.persister.delayKeys = map[string]time.Duration{"bad.md": 100 * time.Millisecond}
		converter := &scriptConverter{objects: []*importv2.Object{
			pageObj("a.md", false), pageObj("bad.md", false),
		}}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeAllOrNothing}, converter, fx.deps)

		// then
		require.Error(t, result.Err)
		assert.Equal(t, 1, marked)
		assert.Zero(t, deletedWhenMarked, "state must be durable before the first delete")

		// and: a clean run never marks
		fx = newEngineFixture()
		fx.deps.OnCompensating = func() { marked += 10 }
		result = Run(context.Background(), importv2.Request{}, converter2(), fx.deps)
		require.NoError(t, result.Err)
		assert.Equal(t, 1, marked)
	})
}

// converter2 is a fresh single-object converter (scriptConverter instances
// are stateless, but per-run construction is the contract).
func converter2() *scriptConverter {
	return &scriptConverter{objects: []*importv2.Object{pageObj("ok.md", false)}}
}

func TestRunCancellation(t *testing.T) {
	t.Run("cancel interrupts a slow run promptly and compensates", func(t *testing.T) {
		// given
		fx := newEngineFixture()
		fx.persister.delay = 50 * time.Millisecond
		objects := make([]*importv2.Object, 200)
		for i := range objects {
			objects[i] = pageObj(fmt.Sprintf("p-%03d.md", i), false)
		}
		converter := &scriptConverter{objects: objects}
		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan *importv2.Result, 1)
		go func() {
			done <- Run(ctx, importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)
		}()

		// when
		time.Sleep(100 * time.Millisecond)
		cancel()

		// then
		select {
		case result := <-done:
			issue := importv2.AsIssue(result.Err, importv2.SeverityFatal, importv2.IssueStoreError)
			assert.Equal(t, importv2.IssueCancelled, issue.Code)
			assert.Less(t, result.Created, int64(200), "cancellation must interrupt the stream")
		case <-time.After(5 * time.Second):
			t.Fatal("run did not stop after cancel")
		}
	})

	t.Run("suspend cause stops the run WITHOUT compensating", func(t *testing.T) {
		// given — a shutdown suspend must preserve the run's work for the
		// startup sweep instead of tearing it down (spec §6.4): compensation
		// is skipped, journal effects stay, OnCompensating never fires.
		fx := newEngineFixture()
		fx.persister.delay = 20 * time.Millisecond
		marked := false
		fx.deps.OnCompensating = func() { marked = true }
		objects := make([]*importv2.Object, 200)
		for i := range objects {
			objects[i] = pageObj(fmt.Sprintf("p-%03d.md", i), false)
		}
		converter := &scriptConverter{objects: objects}
		ctx, cancel := context.WithCancelCause(context.Background())

		done := make(chan *importv2.Result, 1)
		go func() {
			done <- Run(ctx, importv2.Request{Mode: importv2.ModeAllOrNothing}, converter, fx.deps)
		}()

		// when: suspend once at least one object is committed
		require.Eventually(t, func() bool {
			fx.persister.mu.Lock()
			defer fx.persister.mu.Unlock()
			return len(fx.persister.persisted) >= 1
		}, 5*time.Second, 5*time.Millisecond)
		cancel(importv2.ErrSuspended)

		// then
		select {
		case result := <-done:
			issue := importv2.AsIssue(result.Err, importv2.SeverityFatal, importv2.IssueStoreError)
			assert.Equal(t, importv2.IssueCancelled, issue.Code)
			assert.True(t, result.Suspended, "the engine owns the suspend verdict (P1-3)")
			assert.Zero(t, result.Compensated, "suspend must not compensate")
			assert.Empty(t, fx.deps.Objects.(*deleterFake).deleted, "no created object may be deleted on suspend")
			assert.False(t, marked, "the compensating state must not be marked on suspend")
		case <-time.After(5 * time.Second):
			t.Fatal("run did not stop after suspend")
		}
	})

	t.Run("cancel of a fully-buffered small import compensates and reports cancelled", func(t *testing.T) {
		// given — P0-2: the lanes hold 2*16+8 objects, so a small import is
		// entirely inside the channels when cancel fires: the converter has
		// already returned cleanly and cannot be the one to report the
		// cancellation. The run must still end cancelled and compensated,
		// never as a silent success.
		fx := newEngineFixture()
		fx.persister.delay = 20 * time.Millisecond
		objects := make([]*importv2.Object, 20)
		for i := range objects {
			objects[i] = pageObj(fmt.Sprintf("p-%03d.md", i), false)
		}
		converter := &scriptConverter{objects: objects}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan *importv2.Result, 1)
		go func() {
			done <- Run(ctx, importv2.Request{Mode: importv2.ModeAllOrNothing}, converter, fx.deps)
		}()

		// when: cancel once at least one object is committed
		require.Eventually(t, func() bool {
			fx.persister.mu.Lock()
			defer fx.persister.mu.Unlock()
			return len(fx.persister.persisted) >= 1
		}, 5*time.Second, 5*time.Millisecond)
		cancel()

		// then
		select {
		case result := <-done:
			require.Error(t, result.Err, "a cancelled import must never report success")
			issue := importv2.AsIssue(result.Err, importv2.SeverityFatal, importv2.IssueStoreError)
			assert.Equal(t, importv2.IssueCancelled, issue.Code)
			assert.False(t, result.Suspended, "a plain user cancel is not a suspend")
			assert.Positive(t, result.Compensated, "user cancel must compensate")
			assert.NotEmpty(t, fx.deps.Objects.(*deleterFake).deleted)
		case <-time.After(5 * time.Second):
			t.Fatal("run did not stop after cancel")
		}
	})

	t.Run("a fatal issue from an interrupted persist is never swallowed as skipped", func(t *testing.T) {
		// given — the interrupted-persist skip branch may only absorb pure
		// cancellation; a fatal issue that happens to wrap a context error
		// (a durable-journal write timing out during shutdown) must abort
		// loudly, or the effect goes unrecorded in silence.
		fx := newEngineFixture()
		fatal := importv2.Fatal(importv2.IssueStoreError,
			fmt.Errorf("journal effect: %w", context.DeadlineExceeded))
		fx.persister.failOnCancelKeys = map[string]error{"x.md": fatal}
		converter := &scriptConverter{objects: []*importv2.Object{
			pageObj("a.md", false), pageObj("x.md", false),
		}}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan *importv2.Result, 1)
		go func() {
			done <- Run(ctx, importv2.Request{Mode: importv2.ModeAllOrNothing}, converter, fx.deps)
		}()
		require.Eventually(t, func() bool {
			fx.persister.mu.Lock()
			defer fx.persister.mu.Unlock()
			return len(fx.persister.persisted) >= 1
		}, 5*time.Second, 5*time.Millisecond)

		// when
		cancel()

		// then
		select {
		case result := <-done:
			require.Error(t, result.Err)
			issue := importv2.AsIssue(result.Err, importv2.SeverityFatal, importv2.IssueObjectFailed)
			assert.Equal(t, importv2.IssueStoreError, issue.Code,
				"the fatal store issue must win over the generic cancellation")
		case <-time.After(5 * time.Second):
			t.Fatal("run did not stop")
		}
	})
}

// cancellingFactory fires the run's cancellation from inside finalize — the
// window the review mutation-verified: after the post-stream guard, before
// the success return.
type cancellingFactory struct {
	inner  fakeCollectionFactory
	cancel context.CancelFunc
}

func (f *cancellingFactory) MakeCollection(name string, memberSourceKeys []string) (*importv2.Object, error) {
	f.cancel()
	return f.inner.MakeCollection(name, memberSourceKeys)
}

// blockingEnumConverter parks in pass 1 until the run context dies.
type blockingEnumConverter struct {
	started chan struct{}
}

func (c *blockingEnumConverter) Name() string { return "blocking" }

func (c *blockingEnumConverter) EnumerateIdentities(ctx context.Context, yield func(importv2.IdentityClaim) error) error {
	close(c.started)
	<-ctx.Done()
	return ctx.Err()
}

func (c *blockingEnumConverter) Convert(ctx context.Context, sink importv2.Sink) (importv2.RootSpec, error) {
	return importv2.RootSpec{}, nil
}

func TestRunStopClassification(t *testing.T) {
	t.Run("cancel during finalize can never yield success (IGNORE_ERRORS)", func(t *testing.T) {
		// given — Invariant 1 (CONFIRMED by review): in IGNORE_ERRORS a
		// cancel after the post-stream guard produced Err=nil, wire NULL,
		// zero compensation — and the adapter dropped the dir as completed.
		fx := newEngineFixture()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fx.deps.Collection = &cancellingFactory{cancel: cancel}
		converter := &scriptConverter{
			objects:  []*importv2.Object{pageObj("a.md", true), pageObj("b.md", true)},
			rootSpec: importv2.RootSpec{CollectionName: "Import"},
		}

		// when
		result := Run(ctx, importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.Error(t, result.Err, "a stopped run may never report success")
		issue := importv2.AsIssue(result.Err, importv2.SeverityFatal, importv2.IssueStoreError)
		assert.Equal(t, importv2.IssueCancelled, issue.Code)
		assert.False(t, result.Suspended)
		assert.Positive(t, result.Compensated, "user cancel during finalize must compensate")
		assert.NotEmpty(t, fx.deps.Objects.(*deleterFake).deleted)
	})

	t.Run("suspend during pass 1 carries the suspend verdict", func(t *testing.T) {
		// given — Invariant 1: identityPass's early return skipped the
		// classification, so a shutdown during pass 1 fired a spurious
		// cancelled notification instead of a quiet suspend.
		fx := newEngineFixture()
		converter := &blockingEnumConverter{started: make(chan struct{})}
		ctx, cancel := context.WithCancelCause(context.Background())
		done := make(chan *importv2.Result, 1)
		go func() {
			done <- Run(ctx, importv2.Request{}, converter, fx.deps)
		}()
		<-converter.started

		// when
		cancel(importv2.ErrSuspended)

		// then
		select {
		case result := <-done:
			require.Error(t, result.Err)
			assert.True(t, result.Suspended, "the engine owns the suspend verdict on every exit path")
			assert.Zero(t, result.Compensated)
		case <-time.After(5 * time.Second):
			t.Fatal("run did not stop")
		}
	})
}

func TestSuspendAfterCompletion(t *testing.T) {
	t.Run("a suspend landing after all stages completed is a completed import", func(t *testing.T) {
		// given — B3: the root collection and report page have persisted;
		// there is nothing left to stop. Classifying this as suspended made
		// the next sweep compensate a COMPLETE import — new data loss. A
		// user cancel in the same window still undoes (that is the cancel
		// contract); only the shutdown suspend is terminal-success.
		fx := newEngineFixture()
		ctx, cancel := context.WithCancelCause(context.Background())
		defer cancel(nil)
		// the report page is the last mutating stage: fire the suspend from
		// inside its persist, so it lands after every other stage
		fx.persister.observe = func() {}
		var suspendFired atomic.Bool
		fx.persister.observeKeyed = func(sourceKey string) {
			if sourceKey == "import-report" && !suspendFired.Swap(true) {
				cancel(importv2.ErrSuspended)
			}
		}
		converter := &issueScriptConverter{scriptConverter{objects: []*importv2.Object{pageObj("a.md", false)}}}

		// when
		result := Run(ctx, importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err, "a run that completed all its work is complete")
		assert.False(t, result.Suspended)
		assert.NotEmpty(t, result.ReportObjectId)
		assert.Zero(t, result.Compensated)
	})
}

// issueScriptConverter emits its objects plus one warning so the report
// page (the final mutating stage) is produced.
type issueScriptConverter struct {
	scriptConverter
}

func (c *issueScriptConverter) Convert(ctx context.Context, sink importv2.Sink) (importv2.RootSpec, error) {
	sink.Issue(importv2.Warning(importv2.IssueDataLoss, "a.md", "synthetic warning"))
	return c.scriptConverter.Convert(ctx, sink)
}

func TestLateClaimsFlush(t *testing.T) {
	t.Run("claims made during finalize reach the ledger", func(t *testing.T) {
		// given — E4: FlushClaims ran at the ends of passes 1 and 2 only;
		// the root-collection and report-page claims (made in finalize)
		// stayed buffered forever — write-ahead intent that never wrote.
		fx := newEngineFixture()
		fx.deps.Collection = &fakeCollectionFactory{}
		converter := &scriptConverter{
			objects:  []*importv2.Object{pageObj("a.md", true)},
			rootSpec: importv2.RootSpec{CollectionName: "Import"},
		}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then: no claim may remain unflushed at the end of the run
		require.NoError(t, result.Err)
		fx.identity.mu.Lock()
		events := append([]string(nil), fx.identity.events...)
		fx.identity.mu.Unlock()
		require.NotEmpty(t, events)
		assert.Equal(t, "flush", events[len(events)-1],
			"the run must end with a flush after the last claim, got %v", events)
	})
}

func TestCompensationEvidence(t *testing.T) {
	t.Run("a panic inside compensation reports leaked, never a clean zero", func(t *testing.T) {
		// given — A2 (CONFIRMED regression): the re-entrancy guard returned
		// silently with Leaked=0, so finishRun's leak gate failed and the
		// run dir was DROPPED while its objects remained in the space.
		fx := newEngineFixture()
		deleter := fx.deps.Objects.(*deleterFake)
		deleter.panicIds = map[string]bool{"id-b.md": true}
		fx.persister.failKeys["bad.md"] = assert.AnError
		fx.persister.delayKeys = map[string]time.Duration{"bad.md": 100 * time.Millisecond}
		converter := &scriptConverter{objects: []*importv2.Object{
			pageObj("a.md", false), pageObj("b.md", false), pageObj("bad.md", false),
		}}

		// when: the abort compensates; deleting id-b.md panics mid-pass
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeAllOrNothing}, converter, fx.deps)

		// then
		require.Error(t, result.Err)
		assert.Positive(t, result.Leaked,
			"an incomplete compensation must report leaked so the dir is kept")
	})
}

func TestRunMemoryBound(t *testing.T) {
	t.Run("in-flight heavy objects never exceed the pipeline bound", func(t *testing.T) {
		// given
		fx := newEngineFixture()
		fx.persister.delay = time.Millisecond
		var inFlight, maxInFlight atomic.Int64
		fx.deps.Gauge = func(delta int) {
			now := inFlight.Add(int64(delta))
			for {
				max := maxInFlight.Load()
				if now <= max || maxInFlight.CompareAndSwap(max, now) {
					break
				}
			}
		}
		objects := make([]*importv2.Object, 2000)
		for i := range objects {
			objects[i] = pageObj(fmt.Sprintf("p-%04d.md", i), false)
		}
		converter := &scriptConverter{objects: objects}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, int64(2000), result.Created)
		bound := int64(2*channelCapacity + workerCount + 1)
		assert.LessOrEqual(t, maxInFlight.Load(), bound,
			"reintroducing collect-then-process would blow this bound")
	})
}

func TestClaimsReconciliation(t *testing.T) {
	t.Run("silently dropped claim becomes an invariant issue", func(t *testing.T) {
		// given
		fx := newEngineFixture()
		converter := &gapConverter{
			scriptConverter: scriptConverter{objects: []*importv2.Object{pageObj("a.md", false)}},
			gapKeys:         []string{"dropped.md"},
		}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, int64(1), result.Created)
		assert.Equal(t, int64(1), result.Failed)
		require.Len(t, result.Issues, 1)
		assert.Equal(t, importv2.IssueInvariant, result.Issues[0].Code)
		assert.Equal(t, "dropped.md", result.Issues[0].SourceKey)
	})

	t.Run("loudly skipped claim is not double-flagged", func(t *testing.T) {
		// given
		fx := newEngineFixture()
		converter := &gapConverter{
			scriptConverter: scriptConverter{objects: []*importv2.Object{pageObj("a.md", false)}},
			loudKeys:        []string{"skipped.md"},
		}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Zero(t, result.Failed)
		require.Len(t, result.Issues, 1)
		assert.Equal(t, importv2.IssueDataLoss, result.Issues[0].Code, "only the converter's own warning")
	})

	t.Run("all-or-nothing aborts and compensates on a silent gap", func(t *testing.T) {
		// given
		fx := newEngineFixture()
		converter := &gapConverter{
			scriptConverter: scriptConverter{objects: []*importv2.Object{pageObj("a.md", false)}},
			gapKeys:         []string{"dropped.md"},
		}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeAllOrNothing}, converter, fx.deps)

		// then
		require.Error(t, result.Err)
		issue := importv2.AsIssue(result.Err, importv2.SeverityFatal, importv2.IssueStoreError)
		assert.Equal(t, importv2.IssueInvariant, issue.Code)
		assert.Equal(t, 1, result.Compensated)
	})
}

func TestImportReport(t *testing.T) {
	t.Run("run with issues persists a report page listed in the root collection", func(t *testing.T) {
		// given
		fx := newEngineFixture()
		factory := &fakeCollectionFactory{}
		fx.deps.Collection = factory
		fx.persister.failKeys["bad.md"] = assert.AnError
		converter := &scriptConverter{
			objects:  []*importv2.Object{pageObj("a.md", true), pageObj("bad.md", true)},
			rootSpec: importv2.RootSpec{CollectionName: "Markdown Import"},
		}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, "id-import-report", result.ReportObjectId)
		assert.Contains(t, fx.persister.persisted, "import-report")
		assert.Equal(t, []string{"a.md", "import-report"}, factory.members,
			"failed page filtered, report listed after the content")
	})

	t.Run("clean run creates no report", func(t *testing.T) {
		// given
		fx := newEngineFixture()
		converter := &scriptConverter{objects: []*importv2.Object{pageObj("a.md", false)}}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Empty(t, result.ReportObjectId)
		assert.NotContains(t, fx.persister.persisted, "import-report")
	})

	t.Run("info-only diagnostics do not cause a report page", func(t *testing.T) {
		// given — flavour/type-suggestion info fires on most imports; a
		// report page must mean something actually went wrong.
		fx := newEngineFixture()
		converter := &issueConverter{
			scriptConverter: scriptConverter{objects: []*importv2.Object{pageObj("a.md", false)}},
			issue:           importv2.Info(importv2.IssueFlavourDetected, "notion-export"),
		}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		require.Len(t, result.Issues, 1, "the info issue itself is still reported")
		assert.Empty(t, result.ReportObjectId)
		assert.NotContains(t, fx.persister.persisted, "import-report")
	})

	t.Run("report persist failure degrades to a warning, never aborts", func(t *testing.T) {
		// given
		fx := newEngineFixture()
		fx.persister.failKeys["bad.md"] = assert.AnError
		fx.persister.failKeys["import-report"] = assert.AnError
		converter := &scriptConverter{objects: []*importv2.Object{pageObj("a.md", false), pageObj("bad.md", false)}}

		// when — all-or-nothing would abort on any new ObjectError
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Empty(t, result.ReportObjectId)
		var reportIssue *importv2.Issue
		for i := range result.Issues {
			if result.Issues[i].SourceKey == "import-report" {
				reportIssue = &result.Issues[i]
			}
		}
		require.NotNil(t, reportIssue)
		assert.Equal(t, importv2.SeverityWarning, reportIssue.Severity)
	})
}

func TestPanicFirewall(t *testing.T) {
	t.Run("persist panic fails one object, run continues", func(t *testing.T) {
		// given
		fx := newEngineFixture()
		fx.persister.panicKeys = map[string]bool{"bad.md": true}
		converter := &scriptConverter{objects: []*importv2.Object{
			pageObj("a.md", false), pageObj("bad.md", false), pageObj("c.md", false),
		}}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, int64(2), result.Created)
		assert.Equal(t, int64(1), result.Failed)
		require.NotEmpty(t, result.Issues)
		issue := result.Issues[0]
		assert.Equal(t, importv2.IssueInvariant, issue.Code)
		assert.Equal(t, "bad.md", issue.SourceKey)
		assert.Contains(t, issue.Error(), "injected persist panic")
	})

	t.Run("persist panic on a file still completes the future", func(t *testing.T) {
		// given
		fx := newEngineFixture()
		fx.persister.panicKeys = map[string]bool{"img.png": true}
		// A page rides along: file objects are never claimed in pass 1, so a
		// file-only source would trip the noObjects gate before the stream.
		converter := &scriptConverter{objects: []*importv2.Object{pageObj("a.md", false), fileObj("img.png")}}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, int64(1), result.Failed)
		state := fx.identity.files["img.png"]
		require.NotNil(t, state)
		select {
		case <-state.done:
			require.Error(t, state.err, "waiters must observe the failure, not hang")
		default:
			t.Fatal("file future was not completed after a persist panic")
		}
	})

	t.Run("converter panic aborts with an invariant issue — and nothing to compensate", func(t *testing.T) {
		// given — under deferred materialization the converter runs in
		// pass 2, BEFORE anything enters the space: a converter panic (or
		// any pass-2 abort) now strands zero objects by construction. This
		// test previously asserted the emitted object was compensated; the
		// stronger property is that there is nothing to compensate at all.
		fx := newEngineFixture()
		converter := &panicConvertConverter{scriptConverter{objects: []*importv2.Object{pageObj("a.md", false)}}}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.Error(t, result.Err)
		issue := importv2.AsIssue(result.Err, importv2.SeverityFatal, importv2.IssueStoreError)
		assert.Equal(t, importv2.IssueInvariant, issue.Code)
		assert.Contains(t, issue.Error(), "injected converter panic")
		assert.Zero(t, result.Created, "pass 2 must not have touched the space")
		assert.Zero(t, result.Compensated, "an abort during fetch/convert has nothing to undo")
		assert.Empty(t, fx.deps.Objects.(*deleterFake).deleted)
	})

	t.Run("identity-pass panic returns a fatal result instead of crashing", func(t *testing.T) {
		// when
		result := Run(context.Background(), importv2.Request{}, &panicEnumConverter{}, newEngineFixture().deps)

		// then
		require.Error(t, result.Err)
		issue := importv2.AsIssue(result.Err, importv2.SeverityFatal, importv2.IssueStoreError)
		assert.Equal(t, importv2.IssueInvariant, issue.Code)
		assert.Contains(t, issue.Error(), "injected enumerate panic")
	})
}

func TestFileLane(t *testing.T) {
	t.Run("queued file progresses while every shared worker waits on it", func(t *testing.T) {
		// given — the deadlock regime the dedicated file lane exists for:
		// referencing objects are emitted BEFORE their file, fill the
		// shared workers and the queue, and each blocks until the file
		// persists. Routing files through the shared lane would park all
		// shared workers behind a file they queued in front of — this test
		// times out with per-object failures if the lane is removed.
		fx := newEngineFixture()
		fx.persister.fileReady = make(chan struct{})
		var objects []*importv2.Object
		// 20 referencing pages ≤ workers-1 + channel capacity, so the
		// converter still reaches the file emission.
		for i := 0; i < 20; i++ {
			objects = append(objects, pageObj(fmt.Sprintf("ref-%02d.md", i), false))
		}
		objects = append(objects, fileObj("img.png"))
		converter := &scriptConverter{objects: objects}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Zero(t, result.Failed, "a stalled file lane fails the referencing objects")
		assert.Equal(t, int64(21), result.Created)
		state := fx.identity.files["img.png"]
		require.NotNil(t, state)
		select {
		case <-state.done:
			assert.Equal(t, "file-img.png", state.id)
		default:
			t.Fatal("file future was not completed")
		}
	})
}
