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
	panicKeys map[string]bool
	journal   *persist.Journal
	// fileReady simulates reference resolution: objects whose key starts
	// with "ref-" block until the file object's persist closes the channel
	// (as resolver.ResolveRef blocks on the identity future in production).
	fileReady chan struct{}
}

func (f *fakePersister) Persist(ctx context.Context, o *importv2.Object, target persist.Target, report func(importv2.Issue)) (persist.Outcome, error) {
	// Fires outside the mutex: a recovered panic must not strand the lock.
	if f.panicKeys[o.SourceKey] {
		panic("injected persist panic: " + o.SourceKey)
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
	f.mu.Unlock()
	if err != nil {
		return persist.Outcome{}, err
	}
	id := target.Id
	if id == "" {
		id = "file-" + o.SourceKey
	}
	if f.journal != nil {
		if err := f.journal.CreatedObject(ctx, o.SourceKey, id); err != nil {
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
	mu      sync.Mutex
	deleted []string
}

func (d *deleterFake) GetObject(ctx context.Context, objectId string) (smartblock.SmartBlock, error) {
	return nil, errors.New("not used")
}

func (d *deleterFake) GetObjectByFullID(ctx context.Context, id domain.FullID) (smartblock.SmartBlock, error) {
	return nil, errors.New("not used")
}

func (d *deleterFake) DeleteObject(objectId string) error {
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

	t.Run("converter panic aborts with an invariant issue and compensates", func(t *testing.T) {
		// given
		fx := newEngineFixture()
		converter := &panicConvertConverter{scriptConverter{objects: []*importv2.Object{pageObj("a.md", false)}}}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.Error(t, result.Err)
		issue := importv2.AsIssue(result.Err, importv2.SeverityFatal, importv2.IssueStoreError)
		assert.Equal(t, importv2.IssueInvariant, issue.Code)
		assert.Contains(t, issue.Error(), "injected converter panic")
		assert.Equal(t, 1, result.Compensated, "the already-created object must be compensated")
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
