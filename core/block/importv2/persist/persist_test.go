package persist

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/importv2"
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

const testSpaceId = "spaceId"

type fakeSpace struct {
	created     []treestorage.TreeStorageCreatePayload
	createErr   error
	existing    map[string]smartblock.SmartBlock
	initStates  []*state.State
	lastInitCtx *smartblock.InitContext
	// beforeCreate observes the moment of the tree write (write-ahead
	// ordering assertions).
	beforeCreate func()
}

func (f *fakeSpace) CreateTreeObjectWithPayload(ctx context.Context, payload treestorage.TreeStorageCreatePayload, initFunc smartblock.InitFunc) (smartblock.SmartBlock, error) {
	if f.beforeCreate != nil {
		f.beforeCreate()
	}
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, payload)
	initCtx := initFunc(payload.RootRawChange.Id)
	f.lastInitCtx = initCtx
	f.initStates = append(f.initStates, initCtx.State)
	sb := smarttest.New(payload.RootRawChange.Id)
	return sb, nil
}

func (f *fakeSpace) Do(objectId string, apply func(sb smartblock.SmartBlock) error) error {
	sb, ok := f.existing[objectId]
	if !ok {
		return errors.New("object not found")
	}
	return apply(sb)
}

type fakeObjects struct {
	objects map[string]smartblock.SmartBlock
	deleted []string
	failIds map[string]error
	// onDelete fires after a successful delete (compensation-bounds tests).
	onDelete func(id string)
}

func (f *fakeObjects) GetObject(ctx context.Context, objectId string) (smartblock.SmartBlock, error) {
	sb, ok := f.objects[objectId]
	if !ok {
		return nil, errors.New("not found")
	}
	return sb, nil
}

func (f *fakeObjects) GetObjectByFullID(ctx context.Context, id domain.FullID) (smartblock.SmartBlock, error) {
	return f.GetObject(ctx, id.ObjectID)
}

func (f *fakeObjects) DeleteObject(objectId string) error {
	if err, ok := f.failIds[objectId]; ok {
		return err
	}
	f.deleted = append(f.deleted, objectId)
	if f.onDelete != nil {
		f.onDelete(objectId)
	}
	return nil
}

type fakeUploader struct {
	uploadedPaths []string
	uploadedUrls  []string
	resultId      string
	err           error
}

func (f *fakeUploader) UploadFile(ctx context.Context, spaceId string, req block.FileUploadRequest) (string, model.BlockContentFileType, *domain.Details, error) {
	if f.err != nil {
		return "", 0, nil, f.err
	}
	f.uploadedPaths = append(f.uploadedPaths, req.LocalPath)
	f.uploadedUrls = append(f.uploadedUrls, req.Url)
	return f.resultId, model.BlockContentFile_File, domain.NewDetails(), nil
}

func (f *fakeUploader) CreateFromImport(fileId domain.FullFileId, origin objectorigin.ObjectOrigin, details *domain.Details) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.resultId, nil
}

type fakeFlags struct {
	favorites []string
	archived  []string
}

func (f *fakeFlags) SetIsFavorite(objectId string, isFavorite bool) error {
	f.favorites = append(f.favorites, objectId)
	return nil
}

func (f *fakeFlags) SetIsArchived(sctx session.Context, ctx context.Context, objectId string, isArchived bool, skipCascade bool) error {
	f.archived = append(f.archived, objectId)
	return nil
}

type noopRewriter struct{}

func (noopRewriter) RewriteState(ctx context.Context, st *state.State, report func(importv2.Issue)) error {
	return nil
}

type fakeInstaller struct {
	calls [][]string
	err   error
}

func (f *fakeInstaller) InstallBundledObjects(ctx context.Context, ids []string) error {
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, ids)
	return nil
}

type fixture struct {
	*Persister
	space     *fakeSpace
	objects   *fakeObjects
	uploader  *fakeUploader
	flags     *fakeFlags
	installer *fakeInstaller
	journal   *Journal
	checker   *fakeChecker
	issues    []importv2.Issue
}

func newFixture(t *testing.T) *fixture {
	space := &fakeSpace{existing: map[string]smartblock.SmartBlock{}}
	objects := &fakeObjects{objects: map[string]smartblock.SmartBlock{}, failIds: map[string]error{}}
	uploader := &fakeUploader{resultId: "fileObj1"}
	flags := &fakeFlags{}
	installer := &fakeInstaller{}
	journal := NewJournal()
	fx := &fixture{
		space:     space,
		objects:   objects,
		uploader:  uploader,
		flags:     flags,
		installer: installer,
		journal:   journal,
		checker:   &fakeChecker{preExisting: map[string]bool{}},
	}
	fx.Persister = New(
		testSpaceId,
		objectorigin.Import(model.Import_Markdown),
		space,
		objects,
		uploader,
		flags,
		noopRewriter{},
		NewInstallCoordinator(installer),
		journal,
		fx.checker,
		t.TempDir(),
	)
	return fx
}

// fakeChecker marks ids as pre-existing in the space index.
type fakeChecker struct {
	preExisting map[string]bool
}

func (c *fakeChecker) Exists(id string) bool {
	return c.preExisting[id]
}

func (fx *fixture) report(i importv2.Issue) {
	fx.issues = append(fx.issues, i)
}

func pageObject(sourceKey string) *importv2.Object {
	return &importv2.Object{
		SourceKey: sourceKey,
		SbType:    coresb.SmartBlockTypePage,
		Payload: &importv2.Snapshot{
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName:        domain.String("Page"),
				bundle.RelationKeyCreatedDate: domain.Int64(1700000000),
			}),
			ObjectTypes: []string{bundle.TypeKeyPage.String()},
		},
	}
}

func typeObject(sourceKey string) *importv2.Object {
	return &importv2.Object{
		SourceKey: sourceKey,
		SbType:    coresb.SmartBlockTypeObjectType,
		Payload: &importv2.Snapshot{
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName:        domain.String("Meeting"),
				bundle.RelationKeyCreatedDate: domain.Int64(1700000000),
			}),
			ObjectTypes: []string{bundle.TypeKeyObjectType.String()},
		},
	}
}

func payloadFor(id string) treestorage.TreeStorageCreatePayload {
	return treestorage.TreeStorageCreatePayload{
		RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: id},
	}
}

func TestPersistCreate(t *testing.T) {
	t.Run("creates a new tree, stamps provenance, journals it", func(t *testing.T) {
		// given
		fx := newFixture(t)
		obj := pageObject("docs/page.md")
		obj.Favorite = true

		// when
		outcome, err := fx.Persist(context.Background(), obj, Target{Id: "newId", Payload: payloadFor("newId")}, fx.report)

		// then
		require.NoError(t, err)
		assert.Equal(t, ActionCreated, outcome.Action)
		assert.Equal(t, "newId", outcome.Id)
		require.Len(t, fx.space.initStates, 1)
		details := fx.space.initStates[0].CombinedDetails()
		assert.Equal(t, int64(model.ObjectOrigin_import), details.GetInt64(bundle.RelationKeyOrigin))
		assert.Equal(t, int64(model.Import_Markdown), details.GetInt64(bundle.RelationKeyImportType))
		assert.Equal(t, int64(1700000000), details.GetInt64(bundle.RelationKeyLastModifiedDate))
		assert.Equal(t, []string{"newId"}, fx.flags.favorites)
		result := fx.journal.Compensate(context.Background(), fx.objects)
		assert.Equal(t, 1, result.Compensated)
		assert.Equal(t, []string{"newId"}, fx.objects.deleted)
	})

	t.Run("oversized snapshot fails loudly with a typed code", func(t *testing.T) {
		// given — a single object over the sync ceiling would persist
		// locally but never replicate (§16 item 8).
		fx := newFixture(t)
		obj := pageObject("docs/war-and-peace.md")
		huge := strings.Repeat("a", maxSnapshotBytes/2+1<<20)
		for i := 0; i < 2; i++ {
			obj.Payload.Blocks = append(obj.Payload.Blocks, &model.Block{
				Id: fmt.Sprintf("b%d", i),
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: huge,
				}},
			})
		}

		// when
		_, err := fx.Persist(context.Background(), obj, Target{Id: "newId", Payload: payloadFor("newId")}, fx.report)

		// then
		require.Error(t, err)
		issue := importv2.AsIssue(err, importv2.SeverityFatal, importv2.IssueStoreError)
		assert.Equal(t, importv2.IssueObjectTooLarge, issue.Code)
		assert.Equal(t, importv2.SeverityObjectError, issue.Severity)
		assert.Equal(t, "docs/war-and-peace.md", issue.SourceKey)
		assert.Empty(t, fx.space.initStates, "nothing must be created")
	})

	t.Run("existing tree falls back to read, not journaled", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.space.createErr = treestorage.ErrTreeExists
		fx.space.existing["newId"] = smarttest.New("newId")

		// when
		outcome, err := fx.Persist(context.Background(), pageObject("docs/page.md"), Target{Id: "newId", Payload: payloadFor("newId")}, fx.report)

		// then
		require.NoError(t, err)
		assert.Equal(t, ActionSkipped, outcome.Action)
		result := fx.journal.Compensate(context.Background(), fx.objects)
		assert.Zero(t, result.Compensated)
		assert.Empty(t, fx.objects.deleted)
	})
}

// resettableObject models the one real-smartblock behavior smarttest lacks:
// ResetToVersion receiving a state (smarttest's is a silent no-op, which
// would hide a heal that never applied anything).
type resettableObject struct {
	*smarttest.SmartTest
	resetTo *state.State
}

func (r *resettableObject) ResetToVersion(s *state.State) error {
	r.resetTo = s
	return nil
}

func TestPersistHeal(t *testing.T) {
	t.Run("a ledger-proven interrupted create heals by update, attributed created", func(t *testing.T) {
		// given — DM spec §8.1 (08-13 §6.2, D4): on a resumed incarnation a
		// tree written by an interrupted create may be HOLLOW (root change
		// present, state never applied). Skip-and-read would keep it hollow
		// and record ActionSkipped for an object this run made — wrong twice.
		fx := newFixture(t)
		fx.space.createErr = treestorage.ErrTreeExists
		hollow := &resettableObject{SmartTest: smarttest.New("newId")}
		fx.objects.objects["newId"] = hollow
		fx.SetResumeHeal(func(sourceKey string, derived bool) bool { return sourceKey == "docs/page.md" && !derived })

		// when
		outcome, err := fx.Persist(context.Background(), pageObject("docs/page.md"),
			Target{Id: "newId", Payload: payloadFor("newId")}, fx.report)

		// then: the imported state landed and the action says created — the
		// ledger's mode proves this run made the tree, so compensation
		// attribution stays exact
		require.NoError(t, err)
		assert.Equal(t, ActionCreated, outcome.Action)
		require.NotNil(t, hollow.resetTo, "the heal must reset the tree to the imported state")
		assert.Equal(t, "Page", hollow.resetTo.Details().GetString(bundle.RelationKeyName))
		result := fx.journal.Compensate(context.Background(), fx.objects)
		assert.Equal(t, 1, result.Compensated, "a healed create is journaled created (deletable)")
	})

	t.Run("a derived-class create records intent BEFORE the tree write", func(t *testing.T) {
		// given — review Class C: derived objects have no pass-1 claim, so
		// this intent row is their only pre-effect record; written after the
		// create it would protect nothing.
		fx := newFixture(t)
		ledger := &fakeLedger{}
		fx.Persister = New(
			testSpaceId, objectorigin.Import(model.Import_Markdown), fx.space, fx.objects,
			fx.uploader, fx.flags, noopRewriter{}, NewInstallCoordinator(fx.installer),
			NewJournalWithLedger(ledger), fx.checker, t.TempDir(),
		)
		var events []string
		fx.space.beforeCreate = func() { events = append(events, "tree-write") }

		// when
		_, err := fx.Persist(context.Background(), typeObject("type:Meeting"),
			Target{Id: "typeId", Payload: payloadFor("typeId")}, fx.report)

		// then
		require.NoError(t, err)
		require.NotEmpty(t, ledger.calls)
		assert.Equal(t, "intent", ledger.calls[0].kind)
		assert.Equal(t, []string{"tree-write"}, events,
			"exactly one tree write, after the intent record")

		// and: a minted-class page records no intent (its claim already is one)
		ledger.calls = nil
		_, err = fx.Persist(context.Background(), pageObject("docs/page.md"),
			Target{Id: "pageId", Payload: payloadFor("pageId")}, fx.report)
		require.NoError(t, err)
		for _, call := range ledger.calls {
			assert.NotEqual(t, "intent", call.kind, "pages are claim-covered; no intent row")
		}
	})

	t.Run("derived collisions heal only on derived-class proof", func(t *testing.T) {
		// given — the class guard: minted proof must never heal a derived
		// collision (a deterministic derived id can collide with a genuinely
		// pre-existing user object; ResetToVersion there is data loss)
		fx := newFixture(t)
		fx.space.createErr = treestorage.ErrTreeExists
		hollow := &resettableObject{SmartTest: smarttest.New("typeId")}
		fx.objects.objects["typeId"] = hollow
		fx.space.existing["typeId"] = hollow
		fx.SetResumeHeal(func(sourceKey string, derived bool) bool { return !derived }) // minted proof only

		// when
		outcome, err := fx.Persist(context.Background(), typeObject("type:Meeting"),
			Target{Id: "typeId", Payload: payloadFor("typeId")}, fx.report)

		// then: fallback, not heal
		require.NoError(t, err)
		assert.Equal(t, ActionSkipped, outcome.Action)
		assert.Nil(t, hollow.resetTo, "minted proof must not reset a derived-class collision")

		// and: derived proof does heal it
		fx.SetResumeHeal(func(sourceKey string, derived bool) bool { return derived })
		outcome, err = fx.Persist(context.Background(), typeObject("type:Meeting"),
			Target{Id: "typeId", Payload: payloadFor("typeId")}, fx.report)
		require.NoError(t, err)
		assert.Equal(t, ActionCreated, outcome.Action)
		assert.NotNil(t, hollow.resetTo)
	})

	t.Run("without ledger proof the fallback stays skip-and-read", func(t *testing.T) {
		// given — a deterministic derived id can collide with a genuinely
		// pre-existing tree (index lag, the fallback's designed case); with
		// no minted non-terminal row, healing would overwrite what might be
		// user data. Bias: leak, never delete or rewrite user data.
		fx := newFixture(t)
		fx.space.createErr = treestorage.ErrTreeExists
		fx.space.existing["newId"] = smarttest.New("newId")
		fx.SetResumeHeal(func(string, bool) bool { return false })

		// when
		outcome, err := fx.Persist(context.Background(), pageObject("docs/page.md"),
			Target{Id: "newId", Payload: payloadFor("newId")}, fx.report)

		// then
		require.NoError(t, err)
		assert.Equal(t, ActionSkipped, outcome.Action)
		assert.Empty(t, fx.objects.deleted)
	})
}

func TestPersistUpdate(t *testing.T) {
	t.Run("revision guard skips objects newer than the import", func(t *testing.T) {
		// given
		fx := newFixture(t)
		existing := smarttest.New("existingId")
		require.NoError(t, existing.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyRevision, Value: domain.Int64(5)},
		}, false))
		fx.objects.objects["existingId"] = existing
		obj := pageObject("docs/page.md")
		obj.Payload.Details.SetInt64(bundle.RelationKeyRevision, 3)

		// when
		outcome, err := fx.Persist(context.Background(), obj, Target{Id: "existingId", IsExisting: true}, fx.report)

		// then
		require.NoError(t, err)
		assert.Equal(t, ActionSkipped, outcome.Action)
		assert.Empty(t, fx.journal.Updated())
	})

	t.Run("matched object is overwritten and journaled as updated", func(t *testing.T) {
		// given
		fx := newFixture(t)
		existing := smarttest.New("existingId")
		fx.objects.objects["existingId"] = existing

		// when
		outcome, err := fx.Persist(context.Background(), pageObject("docs/page.md"), Target{Id: "existingId", IsExisting: true}, fx.report)

		// then
		require.NoError(t, err)
		assert.Equal(t, ActionUpdated, outcome.Action)
		assert.Equal(t, []string{"existingId"}, fx.journal.Updated())
		result := fx.journal.Compensate(context.Background(), fx.objects)
		assert.Equal(t, []string{"existingId"}, result.Uncovered)
	})

	t.Run("user-authored type is reused, never rewritten", func(t *testing.T) {
		// given — an existing type with no import origin and no revision:
		// the user made it in the UI
		fx := newFixture(t)
		existing := smarttest.New("typeId")
		fx.objects.objects["typeId"] = existing
		obj := typeObject("type:meeting")

		// when
		outcome, err := fx.Persist(context.Background(), obj, Target{Id: "typeId", IsExisting: true}, fx.report)

		// then
		require.NoError(t, err)
		assert.Equal(t, ActionSkipped, outcome.Action)
		assert.Empty(t, fx.journal.Updated())
	})

	t.Run("import-created type is updated on re-import", func(t *testing.T) {
		// given — the existing type carries an import origin
		fx := newFixture(t)
		existing := smarttest.New("typeId")
		require.NoError(t, existing.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyOrigin, Value: domain.Int64(int64(model.ObjectOrigin_import))},
		}, false))
		fx.objects.objects["typeId"] = existing
		obj := typeObject("type:meeting")

		// when
		outcome, err := fx.Persist(context.Background(), obj, Target{Id: "typeId", IsExisting: true}, fx.report)

		// then
		require.NoError(t, err)
		assert.Equal(t, ActionUpdated, outcome.Action)
		assert.Equal(t, []string{"typeId"}, fx.journal.Updated())
	})

	t.Run("matched relation is never updated in place", func(t *testing.T) {
		// given
		fx := newFixture(t)
		obj := &importv2.Object{
			SourceKey: "rel:x",
			SbType:    coresb.SmartBlockTypeRelation,
			Payload: &importv2.Snapshot{
				Key:     "x",
				Details: domain.NewDetails(),
			},
		}

		// when
		outcome, err := fx.Persist(context.Background(), obj, Target{Id: "relId", IsExisting: true}, fx.report)

		// then
		require.NoError(t, err)
		assert.Equal(t, ActionSkipped, outcome.Action)
	})
}

func TestPersistFile(t *testing.T) {
	t.Run("path-backed file uploads and journals", func(t *testing.T) {
		// given
		fx := newFixture(t)
		path := filepath.Join(t.TempDir(), "img.png")
		require.NoError(t, os.WriteFile(path, []byte("img"), 0o644))
		obj := &importv2.Object{
			SourceKey: "docs/img.png",
			SbType:    coresb.SmartBlockTypeFileObject,
			Payload:   &importv2.Snapshot{Details: domain.NewDetails()},
			File:      &importv2.FileSource{Path: path, Name: "img.png"},
		}

		// when
		outcome, err := fx.Persist(context.Background(), obj, Target{}, fx.report)

		// then
		require.NoError(t, err)
		assert.Equal(t, "fileObj1", outcome.Id)
		assert.Equal(t, []string{path}, fx.uploader.uploadedPaths)
		assert.Equal(t, []string{""}, fx.uploader.uploadedUrls,
			"a path-backed upload must not also carry a url")
	})

	t.Run("url-only source — the replayed-spool shape — uploads by url", func(t *testing.T) {
		// given — the shape every remote file has after a spool round-trip:
		// runstore rebuilds FileSource with URL but no Open closure (closures
		// don't serialize) and no local path, so materialize yields "" and
		// req.Url is the ONLY thing letting the uploader re-fetch. This
		// assertion is the branch's first reader: deleting it left the whole
		// suite green while every url-only upload lost its source.
		fx := newFixture(t)
		obj := &importv2.Object{
			SourceKey: "file:abc123",
			SbType:    coresb.SmartBlockTypeFileObject,
			Payload:   &importv2.Snapshot{Details: domain.NewDetails()},
			File:      &importv2.FileSource{Name: "img.png", URL: "https://example.org/img.png"},
		}

		// when
		outcome, err := fx.Persist(context.Background(), obj, Target{}, fx.report)

		// then
		require.NoError(t, err)
		assert.Equal(t, "fileObj1", outcome.Id)
		assert.Equal(t, []string{""}, fx.uploader.uploadedPaths,
			"no local path exists for a url-only source")
		assert.Equal(t, []string{"https://example.org/img.png"}, fx.uploader.uploadedUrls,
			"the url must reach the upload request — it is the only fetch carrier")
	})

	t.Run("streamed file spills to temp and cleans up", func(t *testing.T) {
		// given
		fx := newFixture(t)
		obj := &importv2.Object{
			SourceKey: "docs/img.png",
			SbType:    coresb.SmartBlockTypeFileObject,
			Payload:   &importv2.Snapshot{Details: domain.NewDetails()},
			File: &importv2.FileSource{
				Name: "img.png",
				Open: func(ctx context.Context) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("bytes")), nil
				},
			},
		}

		// when
		outcome, err := fx.Persist(context.Background(), obj, Target{}, fx.report)

		// then
		require.NoError(t, err)
		assert.Equal(t, "fileObj1", outcome.Id)
		require.Len(t, fx.uploader.uploadedPaths, 1)
		assert.True(t, strings.Contains(fx.uploader.uploadedPaths[0], "img.png"))
		_, statErr := os.Stat(fx.uploader.uploadedPaths[0])
		assert.True(t, os.IsNotExist(statErr), "spilled temp file must be removed after upload")
	})

	t.Run("upload failure is an object error issue", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.uploader.err = assert.AnError
		obj := &importv2.Object{
			SourceKey: "docs/img.png",
			SbType:    coresb.SmartBlockTypeFileObject,
			Payload:   &importv2.Snapshot{Details: domain.NewDetails()},
			File:      &importv2.FileSource{Path: "/nonexistent-but-uploader-fails-first", Name: "img.png"},
		}

		// when
		_, err := fx.Persist(context.Background(), obj, Target{}, fx.report)

		// then
		issue := importv2.AsIssue(err, importv2.SeverityFatal, importv2.IssueStoreError)
		assert.Equal(t, importv2.IssueFileFetchFailed, issue.Code)
		assert.Equal(t, importv2.SeverityObjectError, issue.Severity)
	})
}

func TestInstallCoordinator(t *testing.T) {
	t.Run("installs each id once, retries after failure", func(t *testing.T) {
		// given
		installer := &fakeInstaller{}
		c := NewInstallCoordinator(installer)

		// when / then
		require.NoError(t, c.Ensure(context.Background(), []string{"a", "b"}))
		require.NoError(t, c.Ensure(context.Background(), []string{"a", "b"})) // no-op
		require.NoError(t, c.Ensure(context.Background(), []string{"b", "c"}))
		assert.Equal(t, [][]string{{"a", "b"}, {"c"}}, installer.calls)

		installer.err = assert.AnError
		assert.Error(t, c.Ensure(context.Background(), []string{"d"}))
		installer.err = nil
		require.NoError(t, c.Ensure(context.Background(), []string{"d"}))
		assert.Equal(t, [][]string{{"a", "b"}, {"c"}, {"d"}}, installer.calls)
	})
}

func TestCompensate(t *testing.T) {
	t.Run("deletes newest first, never touches pre-existing files, reports leaks", func(t *testing.T) {
		// given
		objects := &fakeObjects{failIds: map[string]error{"bad": assert.AnError}}
		journal := NewJournal()
		require.NoError(t, journal.CreatedObject("page1", "obj1"))
		require.NoError(t, journal.CreatedObject("page2", "obj2"))
		require.NoError(t, journal.CreatedObject("page3", "bad"))
		require.NoError(t, journal.CreatedFile("file1", "preExistingFile", true))
		require.NoError(t, journal.CreatedFile("file2", "ownedFile", false))

		// when
		result := journal.Compensate(context.Background(), objects)

		// then
		assert.Equal(t, []string{"obj2", "obj1", "ownedFile"}, objects.deleted)
		assert.Equal(t, 3, result.Compensated)
		assert.Equal(t, 1, result.Leaked)
		require.Len(t, result.Issues, 1)
		assert.Equal(t, importv2.IssueStoreError, result.Issues[0].Code)
	})

	t.Run("deduped upload of a pre-existing file survives an aborted run", func(t *testing.T) {
		// given — the store fixture is the real classifier: an indexed file
		// object means the upload deduped onto data that pre-dates the run.
		// (An inbound-link guard cannot pin this: the run's just-deleted
		// referencers linger in the index either way.)
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:   domain.String("userFile1"),
			bundle.RelationKeyName: domain.String("vacation photo"),
		}})
		checker := &storeChecker{store: store.SpaceIndex(testSpaceId)}
		fx := newFixture(t)

		journal := NewJournal()
		require.NoError(t, journal.CreatedFile("file1", "userFile1", checker.Exists("userFile1"))) // dedup hit
		require.NoError(t, journal.CreatedFile("file2", "runFile1", checker.Exists("runFile1")))   // fresh upload

		// when
		result := journal.Compensate(context.Background(), fx.objects)

		// then
		assert.NotContains(t, fx.objects.deleted, "userFile1",
			"pre-existing user data must never be compensation-deleted")
		assert.Contains(t, fx.objects.deleted, "runFile1")
		assert.Equal(t, 1, result.Compensated)
	})
}

// storeChecker mirrors the adapter's classifier over a real space index.
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
