package persist

import (
	"context"
	"errors"
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
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const testSpaceId = "spaceId"

type fakeSpace struct {
	created     []treestorage.TreeStorageCreatePayload
	createErr   error
	existing    map[string]smartblock.SmartBlock
	initStates  []*state.State
	lastInitCtx *smartblock.InitContext
}

func (f *fakeSpace) CreateTreeObjectWithPayload(ctx context.Context, payload treestorage.TreeStorageCreatePayload, initFunc smartblock.InitFunc) (smartblock.SmartBlock, error) {
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

func (f *fakeFlags) SetIsArchived(sctx session.Context, ctx context.Context, objectId string, isArchived bool) error {
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
		t.TempDir(),
	)
	return fx
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
		result := fx.journal.Compensate(context.Background(), fx.objects, noLinks{})
		assert.Equal(t, 1, result.Compensated)
		assert.Equal(t, []string{"newId"}, fx.objects.deleted)
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
		result := fx.journal.Compensate(context.Background(), fx.objects, noLinks{})
		assert.Zero(t, result.Compensated)
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
		result := fx.journal.Compensate(context.Background(), fx.objects, noLinks{})
		assert.Equal(t, []string{"existingId"}, result.Uncovered)
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
	t.Run("deletes newest first, keeps linked files, reports leaks", func(t *testing.T) {
		// given
		objects := &fakeObjects{failIds: map[string]error{"bad": assert.AnError}}
		journal := NewJournal()
		journal.CreatedObject("obj1")
		journal.CreatedObject("obj2")
		journal.CreatedObject("bad")
		journal.CreatedFile("fileLinked")
		journal.CreatedFile("fileOrphan")

		// when
		result := journal.Compensate(context.Background(), objects, linkedIds{"fileLinked"})

		// then
		assert.Equal(t, []string{"obj2", "obj1", "fileOrphan"}, objects.deleted)
		assert.Equal(t, 3, result.Compensated)
		assert.Equal(t, 1, result.Leaked)
		require.Len(t, result.Issues, 1)
		assert.Equal(t, importv2.IssueStoreError, result.Issues[0].Code)
	})
}

type noLinks struct{}

func (noLinks) GetInboundLinksById(id string) ([]string, error) { return nil, nil }

// linkedIds marks ids that other (pre-existing) objects link to.
type linkedIds []string

func (l linkedIds) GetInboundLinksById(id string) ([]string, error) {
	for _, linked := range l {
		if linked == id {
			return []string{"other"}, nil
		}
	}
	return nil, nil
}
