// Package enginetest is the end-to-end harness: it runs the real engine
// (identity, resolver, persister, journal) over a real source and the real
// objectstore fixture, with deterministic fakes only at the space/upload
// boundary. Golden tests serialize the materialized object set.
package enginetest

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
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
	id := payload.RootRawChange.Id
	initCtx := initFunc(id)
	sb := smarttest.New(id)
	if initCtx.State != nil {
		if err := sb.Apply(initCtx.State); err != nil {
			return nil, fmt.Errorf("apply init state: %w", err)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.Created[id]; exists {
		return nil, treestorage.ErrTreeExists
	}
	f.Created[id] = initCtx.State
	f.Objects[id] = sb
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
	delete(f.Objects, objectId)
	delete(f.Created, objectId)
	return nil
}

func payloadWithId(id string) treestorage.TreeStorageCreatePayload {
	return treestorage.TreeStorageCreatePayload{
		RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: id},
	}
}

// FakeUploader assigns deterministic file-object ids by file name and
// records uploads (content dedup by name, mirroring content addressing for
// fixture purposes).
type FakeUploader struct {
	mu      sync.Mutex
	Uploads []string
}

func (f *FakeUploader) UploadFile(ctx context.Context, spaceId string, req block.FileUploadRequest) (string, model.BlockContentFileType, *domain.Details, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Uploads = append(f.Uploads, req.LocalPath)
	return "file-" + sanitizeId(req.LocalPath), model.BlockContentFile_File, domain.NewDetails(), nil
}

func (f *FakeUploader) CreateFromImport(fileId domain.FullFileId, origin objectorigin.ObjectOrigin, details *domain.Details) (string, error) {
	return "file-" + fileId.FileId.String(), nil
}

func sanitizeId(localPath string) string {
	// keep only the base name so spill temp prefixes don't leak into ids
	base := localPath
	for i := len(localPath) - 1; i >= 0; i-- {
		if localPath[i] == '/' {
			base = localPath[i+1:]
			break
		}
	}
	return base
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
