// Package enginetest is the end-to-end harness: it runs the real engine
// (identity, resolver, persister, journal) over a real source and the real
// objectstore fixture, with deterministic fakes only at the space/upload
// boundary. Golden tests serialize the materialized object set.
package enginetest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/engine"
	"github.com/anyproto/anytype-heart/core/block/importv2/identity"
	"github.com/anyproto/anytype-heart/core/block/importv2/markdown"
	"github.com/anyproto/anytype-heart/core/block/importv2/persist"
	"github.com/anyproto/anytype-heart/core/block/importv2/resolve"
	"github.com/anyproto/anytype-heart/core/block/importv2/resume"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/block/importv2/source"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const SpaceId = "test-space"

// FakeSpace mints deterministic ids and materializes created trees as
// smarttest objects whose states stay inspectable.
type FakeSpace struct {
	mu      sync.Mutex
	counter int
	Created map[string]*state.State
	Objects map[string]smartblock.SmartBlock
	// Deleted records every DeleteObject in order — compensation coverage
	// is asserted on it (review Class H: no crash test compensated a file
	// at all, so inverting file-ownership classification left the whole
	// suite green).
	Deleted []string
	// BeforeCreate/AfterCreate are crash-injection hooks: BeforeCreate
	// returning an error skips the write and fails the create with it;
	// AfterCreate fires after a successful write. Both may cancel the run
	// context to model a process death at the create boundary.
	BeforeCreate func(id string) error
	AfterCreate  func(id string)
	// BeforeReset observes/injects at the update path (ResetToVersion).
	BeforeReset func(id string) error
}

// spaceObject overrides smarttest's no-op ResetToVersion so update and
// heal paths become visible in Created, the way the real space would show
// the reset state.
type spaceObject struct {
	*smarttest.SmartTest
	space *FakeSpace
	id    string
}

// Details/CombinedDetails serve the RECORDED state (what the create or the
// last reset applied): smarttest's own doc never receives the applied
// state, so without this every fixture object read as detail-less — which
// made persist's hollow-tree probe classify fully-formed objects as torn
// and heal (reset) them (fixture fidelity, review Class H).
func (o *spaceObject) Details() *domain.Details {
	o.space.mu.Lock()
	defer o.space.mu.Unlock()
	if st := o.space.Created[o.id]; st != nil {
		return st.CombinedDetails()
	}
	return o.SmartTest.Details()
}

func (o *spaceObject) CombinedDetails() *domain.Details {
	return o.Details()
}

func (o *spaceObject) ResetToVersion(s *state.State) error {
	if o.space.BeforeReset != nil {
		if err := o.space.BeforeReset(o.id); err != nil {
			return err
		}
	}
	o.space.mu.Lock()
	defer o.space.mu.Unlock()
	o.space.Created[o.id] = s
	return nil
}

func NewFakeSpace() *FakeSpace {
	return &FakeSpace{
		Created: map[string]*state.State{},
		Objects: map[string]smartblock.SmartBlock{},
	}
}

func (f *FakeSpace) CreateTreePayload(ctx context.Context, params payloadcreator.PayloadCreationParams) (treestorage.TreeStorageCreatePayload, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counter++
	return payloadWithId(fmt.Sprintf("obj-%03d", f.counter)), nil
}

func (f *FakeSpace) DeriveTreePayload(ctx context.Context, params payloadcreator.PayloadDerivationParams) (treestorage.TreeStorageCreatePayload, error) {
	return payloadWithId("drv-" + params.Key.Marshal()), nil
}

func (f *FakeSpace) CreateTreeObjectWithPayload(ctx context.Context, payload treestorage.TreeStorageCreatePayload, initFunc smartblock.InitFunc) (smartblock.SmartBlock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err // the real space is ctx-respecting; the fake must be too
	}
	id := payload.RootRawChange.Id
	f.mu.Lock()
	if _, exists := f.Created[id]; exists {
		f.mu.Unlock()
		return nil, treestorage.ErrTreeExists
	}
	f.mu.Unlock()
	// HISTORY-FREE byte validation, like the real objecttree.CreateStorage
	// (review Class H: the old check compared against bytes THIS PROCESS
	// minted, so a modelled process death — an emptied map — let corrupted
	// rehydrated bytes import silently; the real check is id↔bytes, no
	// history). In this fake the bijection is bytes == payloadBytesFor(id),
	// standing in for id == CID(bytes).
	if !bytes.Equal(payload.RootRawChange.GetRawChange(), payloadBytesFor(id)) {
		return nil, fmt.Errorf("create %s: payload bytes are not the root the id names", id)
	}
	if f.BeforeCreate != nil {
		if err := f.BeforeCreate(id); err != nil {
			return nil, err
		}
	}
	initCtx := initFunc(id)
	// The real space stamps these; a wrong value anywhere fails every test
	// that creates anything (review Class H: InitContext.IsNewObject and
	// SpaceID previously reached no assertion in the tree).
	if !initCtx.IsNewObject {
		return nil, fmt.Errorf("create %s: InitContext.IsNewObject must be true", id)
	}
	if initCtx.SpaceID != SpaceId {
		return nil, fmt.Errorf("create %s: InitContext.SpaceID = %q, want %q", id, initCtx.SpaceID, SpaceId)
	}
	sb := smarttest.New(id)
	if initCtx.State != nil {
		if err := sb.Apply(initCtx.State); err != nil {
			return nil, fmt.Errorf("apply init state: %w", err)
		}
	}
	f.mu.Lock()
	if _, exists := f.Created[id]; exists {
		f.mu.Unlock()
		return nil, treestorage.ErrTreeExists
	}
	f.Created[id] = initCtx.State
	f.Objects[id] = &spaceObject{SmartTest: sb, space: f, id: id}
	f.mu.Unlock()
	if f.AfterCreate != nil {
		f.AfterCreate(id)
	}
	return sb, nil
}

func (f *FakeSpace) Do(objectId string, apply func(sb smartblock.SmartBlock) error) error {
	f.mu.Lock()
	sb, ok := f.Objects[objectId]
	f.mu.Unlock()
	if !ok {
		return errors.New("object not found")
	}
	return apply(sb)
}

// GetObject / GetObjectByFullID / DeleteObject implement persist.ObjectAccess.
func (f *FakeSpace) GetObject(ctx context.Context, objectId string) (smartblock.SmartBlock, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sb, ok := f.Objects[objectId]
	if !ok {
		return nil, fmt.Errorf("object %s not found", objectId)
	}
	return sb, nil
}

func (f *FakeSpace) GetObjectByFullID(ctx context.Context, id domain.FullID) (smartblock.SmartBlock, error) {
	return f.GetObject(ctx, id.ObjectID)
}

func (f *FakeSpace) DeleteObject(objectId string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Deleted = append(f.Deleted, objectId)
	delete(f.Objects, objectId)
	delete(f.Created, objectId)
	return nil
}

// payloadBytesFor is the fake's id↔bytes bijection: the history-free stand-
// in for "the id is the CID of the root bytes".
func payloadBytesFor(id string) []byte { return []byte("raw-" + id) }

func payloadWithId(id string) treestorage.TreeStorageCreatePayload {
	// Real payloads always carry root bytes — the id IS their hash — and
	// the durable claim ledger records exactly those bytes for the restart.
	// A fake payload without them models an impossible object and starves
	// the resume path of its payload rows.
	return treestorage.TreeStorageCreatePayload{
		RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: id, RawChange: payloadBytesFor(id)},
		Heads:         []string{id},
	}
}

// UploadRecord is everything one upload was asked to do — the observable
// surface for fields that previously reached no consumer any test watched
// (review Class H: ImageKind, EncryptionKeys, URL).
type UploadRecord struct {
	ContentHash    string
	ImageKind      model.ImageKind
	EncryptionKeys map[string]string
	Url            string
}

// FakeUploader models content addressing HONESTLY: it reads the bytes it
// is given — an upload from a path that does not exist fails like a real
// one (review Class H: the old fake recorded LocalPath unopened, which is
// exactly why a resumed run "succeeded" uploading from a deleted source
// tree) — and derives the file object id from the CONTENT hash, so a
// re-upload of the same bytes converges on the same id whatever path
// carried them, as real content addressing does.
type FakeUploader struct {
	mu      sync.Mutex
	Uploads []string // content hashes, golden-visible
	Records []UploadRecord
	// BeforeUpload is a crash-injection hook: a non-nil error fails the
	// upload with it before anything is recorded.
	BeforeUpload func(localPath string) error
}

func (f *FakeUploader) UploadFile(ctx context.Context, spaceId string, req block.FileUploadRequest) (string, model.BlockContentFileType, *domain.Details, error) {
	if f.BeforeUpload != nil {
		if err := f.BeforeUpload(req.LocalPath); err != nil {
			return "", 0, nil, err
		}
	}
	var hash string
	switch {
	case req.LocalPath != "":
		data, err := os.ReadFile(req.LocalPath)
		if err != nil {
			return "", 0, nil, fmt.Errorf("upload %q: %w", req.LocalPath, err)
		}
		hash = contentHash(data)
	case req.Url != "":
		hash = contentHash([]byte(req.Url)) // the fixture does not fetch
	default:
		return "", 0, nil, errors.New("upload with neither path nor url")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Uploads = append(f.Uploads, hash)
	f.Records = append(f.Records, UploadRecord{
		ContentHash:    hash,
		ImageKind:      req.ImageKind,
		EncryptionKeys: req.CustomEncryptionKeys,
		Url:            req.Url,
	})
	return "file-" + hash, model.BlockContentFile_File, domain.NewDetails(), nil
}

func (f *FakeUploader) CreateFromImport(fileId domain.FullFileId, origin objectorigin.ObjectOrigin, details *domain.Details) (string, error) {
	return "file-" + fileId.FileId.String(), nil
}

func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:4])
}

type nopFlags struct{}

func (nopFlags) SetIsFavorite(objectId string, isFavorite bool) error { return nil }
func (nopFlags) SetIsArchived(sctx session.Context, ctx context.Context, objectId string, isArchived bool, skipCascade bool) error {
	return nil
}

type nopInstaller struct{}

func (nopInstaller) InstallBundledObjects(ctx context.Context, ids []string) error { return nil }

// stubCollectionFactory builds a minimal collection payload without the
// collection service (dataview omitted; membership via the store slice).
type stubCollectionFactory struct{}

func (stubCollectionFactory) MakeCollection(name string, memberSourceKeys []string) (*importv2.Object, error) {
	details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName:           domain.String(name),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
	})
	st := state.NewDoc("root", nil).(*state.State)
	st.UpdateStoreSlice(template.CollectionStoreKey, memberSourceKeys)
	return &importv2.Object{
		SourceKey: "collection:" + name,
		SbType:    coresb.SmartBlockTypePage,
		Payload: &importv2.Snapshot{
			Details:     details,
			ObjectTypes: []string{bundle.TypeKeyCollection.String()},
			Collections: st.Store(),
		},
	}, nil
}

// Fixture wires the real engine over the fakes.
type Fixture struct {
	Space    *FakeSpace
	Store    *objectstore.StoreFixture
	Uploader *FakeUploader
	Journal  *persist.Journal
	// WrapSpool, when set, wraps the durable spool handed to the engine —
	// the crash-injection point for mid-crawl kills (an append boundary is
	// where a pass-2 suspend lands by construction).
	WrapSpool func(engine.Spool) engine.Spool
}

func NewFixture(t *testing.T) *Fixture {
	return &Fixture{
		Space:    NewFakeSpace(),
		Store:    objectstore.NewStoreFixture(t),
		Uploader: &FakeUploader{},
	}
}

// RunMarkdown imports a markdown directory through the full engine, always
// via the DURABLE spool. Scope of the claim, stated precisely: the golden
// fixtures in this package assert a JSON projection of the materialized
// object set (details, blocks, links, membership) over markdown-shaped
// inputs; field-level completeness of the spool round-trip — including the
// Notion-shaped fields no golden carries — is pinned separately by
// runstore's TestSpoolRoundTripEveryField, and the engine unit suite runs
// on a durable spool as well.
func (fx *Fixture) RunMarkdown(t *testing.T, root string, req importv2.Request) *importv2.Result {
	t.Helper()
	src, err := source.Open(root)
	require.NoError(t, err)
	defer src.Close()

	spillDir := t.TempDir()
	spool, err := runstore.OpenStandaloneSpool(context.Background(), spillDir)
	require.NoError(t, err)
	defer spool.Close()

	fx.Journal = persist.NewJournal()
	formats := resolve.NewFormats()
	keys := engine.NewKeyTable()
	identitySvc := identity.NewService(fx.Space, fx.Store.SpaceIndex(SpaceId), req.UpdateExisting, time.Unix(1700000000, 0))
	resolver := resolve.New(identitySvc, keys, formats)
	persister := persist.New(
		SpaceId, req.Origin, fx.Space, fx.Space, fx.Uploader, nopFlags{},
		resolver, persist.NewInstallCoordinator(nopInstaller{}), fx.Journal,
		&storeChecker{store: fx.Store.SpaceIndex(SpaceId)}, spillDir,
	)
	converter := markdown.New(src, markdown.Params{}, stubCollectionFactory{})
	return engine.Run(context.Background(), req, converter, engine.Deps{
		Identity:   identitySvc,
		Persister:  persister,
		Journal:    fx.Journal,
		Objects:    fx.Space,
		Formats:    formats,
		Keys:       keys,
		Collection: stubCollectionFactory{},
		Spool:      spool,
		SpillDir:   spillDir,
	})
}

// durableDeps wires the engine over a real run store the way the adapter
// does: ledger-backed journal, durable spool, MarkFetched at the pass
// boundary, the durable issue recorder.
func (fx *Fixture) durableDeps(t *testing.T, store *runstore.Store, req importv2.Request, identityOpts ...identity.Option) (engine.Deps, *persist.Persister) {
	t.Helper()
	journal := persist.NewJournalWithLedger(store)
	fx.Journal = journal
	formats := resolve.NewFormats()
	keys := engine.NewKeyTable()
	identitySvc := identity.NewService(fx.Space, fx.Store.SpaceIndex(SpaceId), req.UpdateExisting,
		time.Unix(1700000000, 0), identityOpts...)
	resolver := resolve.New(identitySvc, keys, formats)
	persister := persist.New(
		SpaceId, req.Origin, fx.Space, fx.Space, fx.Uploader, nopFlags{},
		resolver, persist.NewInstallCoordinator(nopInstaller{}), journal,
		&storeChecker{store: fx.Store.SpaceIndex(SpaceId)}, store.SpillDir(),
	)
	spool, err := store.Spool(context.Background())
	require.NoError(t, err)
	var engineSpool engine.Spool = spool
	if fx.WrapSpool != nil {
		engineSpool = fx.WrapSpool(spool)
	}
	deps := engine.Deps{
		Identity:   identitySvc,
		Persister:  persister,
		Journal:    journal,
		Objects:    fx.Space,
		Formats:    formats,
		Keys:       keys,
		Collection: stubCollectionFactory{},
		Spool:      engineSpool,
		SpillDir:   store.SpillDir(),
		OnFetched: func(rootSpec importv2.RootSpec) error {
			return store.MarkFetched(context.Background(), rootSpec)
		},
		OnIssue: resume.IssueRecorder(store),
	}
	return deps, persister
}

// planRecorder mirrors the adapter's: the sanitized plan lands in the run
// kv before any emission, so a crawl resume can reuse it (08-13 §6.3).
func planRecorder(t *testing.T, store *runstore.Store) func(schemaplan.Plan) error {
	t.Helper()
	return func(plan schemaplan.Plan) error {
		data, err := json.Marshal(plan)
		if err != nil {
			return err
		}
		return store.SetPlanJSON(context.Background(), data)
	}
}

// RunMarkdownDurable runs one incarnation over a real run store in dir.
// The store is closed but the dir is NOT settled — an interrupted run
// (ctx cancelled with importv2.ErrSuspended mid-materialize) leaves
// exactly the state a killed process leaves: manifest at materializing,
// partial effects journaled, spool whole.
func (fx *Fixture) RunMarkdownDurable(ctx context.Context, t *testing.T, root string, req importv2.Request, dir string) *importv2.Result {
	t.Helper()
	src, err := source.Open(root)
	require.NoError(t, err)
	defer src.Close()
	store, err := runstore.Create(context.Background(), dir, runstore.Manifest{
		RunId: filepath.Base(dir), SpaceId: req.SpaceID, Converter: "Markdown",
	})
	require.NoError(t, err)
	deps, _ := fx.durableDeps(t, store, req, resume.ClaimLedgerOption(store))
	converter := markdown.New(src, markdown.Params{
		PlanReuse: schemaplan.Reuse{Record: planRecorder(t, store)},
	}, stubCollectionFactory{})
	result := engine.Run(ctx, req, converter, deps)
	require.NoError(t, store.Close())
	return result
}

// ResumeDurable restarts pass 3 from the dir alone, through the same glue
// the adapter's sweep uses: Load, rehydrated identity, the heal policy,
// engine.Resume. No source, no network.
func (fx *Fixture) ResumeDurable(ctx context.Context, t *testing.T, dir string, req importv2.Request) *importv2.Result {
	t.Helper()
	store, err := runstore.Open(context.Background(), dir)
	require.NoError(t, err)
	defer store.Close()
	state, err := resume.Load(ctx, store)
	require.NoError(t, err)
	deps, persister := fx.durableDeps(t, store, req,
		resume.ClaimLedgerOption(store), state.IdentityOption())
	persister.SetResumeHeal(state.Heal())
	state.SeedJournal(deps.Journal)
	return engine.Resume(ctx, req, deps, &state.Engine)
}

// ResumeCrawlDurable restarts a run killed MID-CRAWL,
// through the same glue the adapter's sweep uses: LoadCrawl, reclaimable
// identity, a converter rebuilt over the live source with the RECORDED
// plan preset (review P2: the fixture previously discarded the loaded
// PlanJSON, so plan reuse was pinned only at unit level — here the planner
// is poisoned whenever a recording exists, making the reuse load-bearing
// in every crawl-crash test), engine.ResumeCrawl. Unlike ResumeDurable
// this needs the source — that is the class's defining property, and why
// the manifest stores the request for it.
func (fx *Fixture) ResumeCrawlDurable(ctx context.Context, t *testing.T, dir, root string, req importv2.Request) *importv2.Result {
	t.Helper()
	store, err := runstore.Open(context.Background(), dir)
	require.NoError(t, err)
	defer store.Close()
	state, err := resume.LoadCrawl(ctx, store)
	require.NoError(t, err)
	src, err := source.Open(root)
	require.NoError(t, err)
	defer src.Close()
	deps, _ := fx.durableDeps(t, store, req,
		resume.ClaimLedgerOption(store), state.IdentityOption())
	params := markdown.Params{PlanReuse: schemaplan.Reuse{Record: planRecorder(t, store)}}
	if len(state.PlanJSON) > 0 {
		preset := &schemaplan.Plan{}
		require.NoError(t, json.Unmarshal(state.PlanJSON, preset))
		params.PlanReuse.Preset = preset
		params.Planner = schemaplan.PlannerFunc(func(context.Context, []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
			t.Error("a resumed crawl with a recorded plan must never replan (08-13 §6.3)")
			return schemaplan.Plan{}, nil
		})
	}
	converter := markdown.New(src, params, stubCollectionFactory{})
	return engine.ResumeCrawl(ctx, req, converter, deps, &state.Engine)
}

// OriginalTimestamps maps object name → the persisted state's original
// created timestamp. The value is mtime-derived for markdown, so it can
// never live in a golden — but two runs over one tree must agree, which
// makes it a crash-equivalence observable (review Class H: the field
// previously reached no consumer any test watched).
func (fx *Fixture) OriginalTimestamps() map[string]int64 {
	fx.Space.mu.Lock()
	defer fx.Space.mu.Unlock()
	out := map[string]int64{}
	for _, st := range fx.Space.Created {
		if st == nil {
			continue
		}
		if name := st.CombinedDetails().GetString(bundle.RelationKeyName); name != "" {
			out[name] = st.OriginalCreatedTimestamp()
		}
	}
	return out
}

// storeChecker classifies deduped uploads against the real store fixture.
type storeChecker struct {
	store spaceindex.Store
}

func (c *storeChecker) Exists(id string) bool {
	ids, _, err := c.store.QueryObjectIds(database.Query{Filters: []database.FilterRequest{{
		Condition:   model.BlockContentDataviewFilter_Equal,
		RelationKey: bundle.RelationKeyId,
		Value:       domain.String(id),
	}}})
	return err == nil && len(ids) > 0
}

// IndexCreated pushes every created object's details into the store fixture,
// simulating the indexer so a follow-up run can dedup against them.
func (fx *Fixture) IndexCreated(t *testing.T) {
	t.Helper()
	fx.Space.mu.Lock()
	defer fx.Space.mu.Unlock()
	for id, st := range fx.Space.Created {
		if st == nil {
			continue
		}
		object := objectstore.TestObject{bundle.RelationKeyId: domain.String(id)}
		for key, value := range st.CombinedDetails().Iterate() {
			object[key] = value
		}
		fx.Store.AddObjects(t, SpaceId, []objectstore.TestObject{object})
	}
}
