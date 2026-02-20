package fileuploader

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type dummyObjectCreator struct {
	fn func(ctx context.Context, spaceID string, req objectcreator.CreateObjectRequest) (string, *domain.Details, error)
}

func (d *dummyObjectCreator) CreateObject(ctx context.Context, spaceID string, req objectcreator.CreateObjectRequest) (string, *domain.Details, error) {
	return d.fn(ctx, spaceID, req)
}

type noopBacklinksWatcher struct{}

func (noopBacklinksWatcher) Init(*app.App) error         { return nil }
func (noopBacklinksWatcher) Name() string                { return "noopBacklinksWatcher" }
func (noopBacklinksWatcher) Run(context.Context) error   { return nil }
func (noopBacklinksWatcher) Close(context.Context) error { return nil }
func (noopBacklinksWatcher) FlushUpdates()               {}

type objectWithCollection struct {
	*smarttest.SmartTest
	collection.Collection
}

func newObjectWithCollection(id string) *objectWithCollection {
	sb := smarttest.New(id)
	st := sb.Doc.NewState()
	st.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
	sb.Doc = st
	return &objectWithCollection{
		SmartTest:  sb,
		Collection: collection.New(sb, noopBacklinksWatcher{}),
	}
}

type dropFixture struct {
	pickerFx      *mock_cache.MockObjectGetter
	mockSender    *mock_event.MockSender
	fileUploader  *MockService
	objectCreator *dummyObjectCreator
	processServ   process.Service
}

func newDropFixture(t *testing.T) *dropFixture {
	picker := mock_cache.NewMockObjectGetter(t)
	mockSender := mock_event.NewMockSender(t)
	mockSender.EXPECT().BroadcastExceptSessions(mock.Anything, mock.Anything).Maybe()

	fu := NewMockService(t)

	ctx := context.Background()
	a := &app.App{}
	a.Register(testutil.PrepareMock(ctx, a, mockSender))
	a.Register(testutil.PrepareMock(ctx, a, fu))
	service := process.New()
	err := service.Init(a)
	require.NoError(t, err)

	return &dropFixture{
		pickerFx:      picker,
		mockSender:    mockSender,
		fileUploader:  fu,
		objectCreator: &dummyObjectCreator{},
		processServ:   service,
	}
}

func (fx *dropFixture) expectUpload(t *testing.T, times int) {
	upl := NewMockUploader(t)
	upl.EXPECT().SetName(mock.Anything).RunAndReturn(func(name string) Uploader {
		// store name for later use in Upload
		upl.EXPECT().Upload(mock.Anything).Unset()
		upl.EXPECT().Upload(mock.Anything).Return(UploadResult{
			FileObjectId: "fileObj-" + name,
			Name:         name,
		}).Maybe()
		return upl
	}).Times(times)
	upl.EXPECT().SetFile(mock.Anything).Return(upl).Times(times)
	upl.EXPECT().SetCreatedInContext(mock.Anything).Return(upl).Times(times)
	upl.EXPECT().SetCreatedInContextRef(mock.Anything).Return(upl).Times(times)
	upl.EXPECT().Upload(mock.Anything).Return(UploadResult{
		FileObjectId: "fileObj",
		Name:         "uploaded",
	}).Maybe()
	fx.fileUploader.EXPECT().NewUploader(mock.Anything, mock.Anything).Return(upl).Times(times)
}

// --- Tests ---

func TestDropFiles(t *testing.T) {
	t.Run("drop files in collection - no restriction error", func(t *testing.T) {
		// given
		dir := t.TempDir()
		f, err := os.Create(filepath.Join(dir, "test"))
		require.NoError(t, err)

		fx := newDropFixture(t)
		sb := smarttest.New("root")
		st := sb.Doc.NewState()
		st.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
		sb.Doc = st
		sb.TestRestrictions = restriction.Restrictions{Object: restriction.ObjectRestrictions{model.Restrictions_Blocks: {}}}

		cst := &objectWithCollection{SmartTest: sb, Collection: collection.New(sb, noopBacklinksWatcher{})}
		fx.pickerFx.EXPECT().GetObject(mock.Anything, "root").Return(cst, nil).Maybe()
		fx.mockSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()
		fx.expectUpload(t, 1)

		// when
		proc := &dropFilesProcess{
			spaceId:        sb.SpaceID(),
			processService: fx.processServ,
			picker:         fx.pickerFx,
			service:        fx.fileUploader,
			contextId:      "root",
		}
		err = proc.Init([]string{f.Name()})
		require.NoError(t, err)

		ch := make(chan error)
		go proc.Start("root", true, pb.RpcFileDropRequest{
			ContextId:      "root",
			LocalFilePaths: []string{f.Name()},
		}, ch)
		err = <-ch

		// then
		assert.Nil(t, err)
		<-proc.Done()
	})

	t.Run("drop dir in collection - creates child collection", func(t *testing.T) {
		// given
		objectStore := spaceindex.NewStoreFixture(t)
		dir := t.TempDir()
		_, err := os.Create(filepath.Join(dir, "test"))
		require.NoError(t, err)

		fx := newDropFixture(t)
		sb := smarttest.New("root")
		st := sb.Doc.NewState()
		st.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
		sb.Doc = st

		cst := &objectWithCollection{SmartTest: sb, Collection: collection.New(sb, noopBacklinksWatcher{})}
		childColl := newObjectWithCollection("childColl")

		fx.objectCreator.fn = func(_ context.Context, _ string, _ objectcreator.CreateObjectRequest) (string, *domain.Details, error) {
			return "childColl", domain.NewDetails(), nil
		}
		fx.pickerFx.EXPECT().GetObject(mock.Anything, "root").Return(cst, nil).Maybe()
		fx.pickerFx.EXPECT().GetObject(mock.Anything, "childColl").Return(childColl, nil).Maybe()
		fx.mockSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()
		fx.expectUpload(t, 1)

		// when
		proc := &dropFilesProcess{
			spaceId:        sb.SpaceID(),
			processService: fx.processServ,
			picker:         fx.pickerFx,
			service:        fx.fileUploader,
			objectCreator:  fx.objectCreator,
			objectStore:    objectStore,
			contextId:      "root",
		}
		err = proc.Init([]string{dir})
		require.NoError(t, err)

		ch := make(chan error)
		proc.Start("root", true, pb.RpcFileDropRequest{Position: model.Block_Bottom}, ch)
		err = <-ch

		// then
		assert.Nil(t, err)
		storeSlice := cst.NewState().GetStoreSlice(template.CollectionStoreKey)
		assert.Contains(t, storeSlice, "childColl")
	})

	t.Run("drop files in collection - success", func(t *testing.T) {
		// given
		dir := t.TempDir()
		f, err := os.Create(filepath.Join(dir, "test"))
		require.NoError(t, err)

		fx := newDropFixture(t)
		sb := smarttest.New("root")
		st := sb.Doc.NewState()
		st.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
		sb.Doc = st

		cst := &objectWithCollection{SmartTest: sb, Collection: collection.New(sb, noopBacklinksWatcher{})}
		fx.pickerFx.EXPECT().GetObject(mock.Anything, "root").Return(cst, nil)
		fx.mockSender.EXPECT().Broadcast(mock.Anything).Return()
		fx.expectUpload(t, 1)

		// when
		proc := &dropFilesProcess{
			spaceId:        sb.SpaceID(),
			processService: fx.processServ,
			picker:         fx.pickerFx,
			service:        fx.fileUploader,
		}
		err = proc.Init([]string{f.Name()})
		require.NoError(t, err)
		ch := make(chan error)
		proc.Start("root", true, pb.RpcFileDropRequest{Position: model.Block_Bottom}, ch)
		err = <-ch

		// then
		assert.Nil(t, err)
		storeSlice := cst.NewState().GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, storeSlice, 1)
	})

	t.Run("drop dir with file in collection - creates child collection with file", func(t *testing.T) {
		// given
		objectStore := spaceindex.NewStoreFixture(t)
		dir := t.TempDir()
		_, err := os.Create(filepath.Join(dir, "test"))
		require.NoError(t, err)

		fx := newDropFixture(t)
		sb := smarttest.New("root")
		st := sb.Doc.NewState()
		st.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
		sb.Doc = st

		cst := &objectWithCollection{SmartTest: sb, Collection: collection.New(sb, noopBacklinksWatcher{})}
		childColl := newObjectWithCollection("childColl")

		fx.objectCreator.fn = func(_ context.Context, _ string, _ objectcreator.CreateObjectRequest) (string, *domain.Details, error) {
			return "childColl", domain.NewDetails(), nil
		}
		fx.pickerFx.EXPECT().GetObject(mock.Anything, "root").Return(cst, nil)
		fx.pickerFx.EXPECT().GetObject(mock.Anything, "childColl").Return(childColl, nil).Maybe()
		fx.mockSender.EXPECT().Broadcast(mock.Anything).Return()
		fx.expectUpload(t, 1)

		// when
		proc := &dropFilesProcess{
			spaceId:        sb.SpaceID(),
			processService: fx.processServ,
			picker:         fx.pickerFx,
			service:        fx.fileUploader,
			objectCreator:  fx.objectCreator,
			objectStore:    objectStore,
		}
		err = proc.Init([]string{dir})
		require.NoError(t, err)
		ch := make(chan error)
		proc.Start("root", true, pb.RpcFileDropRequest{Position: model.Block_Bottom}, ch)
		err = <-ch

		// then
		assert.Nil(t, err)
		storeSlice := cst.NewState().GetStoreSlice(template.CollectionStoreKey)
		assert.Contains(t, storeSlice, "childColl")
	})

	t.Run("drop dir in document - creates linked collection", func(t *testing.T) {
		// given
		objectStore := spaceindex.NewStoreFixture(t)
		dir := t.TempDir()
		_, err := os.Create(filepath.Join(dir, "test.txt"))
		require.NoError(t, err)

		fx := newDropFixture(t)
		sb := smarttest.New("root")
		sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text("", blockbuilder.ID("targetBlock")),
			)))

		childColl := newObjectWithCollection("childColl")

		fx.objectCreator.fn = func(_ context.Context, _ string, _ objectcreator.CreateObjectRequest) (string, *domain.Details, error) {
			return "childColl", domain.NewDetails(), nil
		}
		fx.pickerFx.EXPECT().GetObject(mock.Anything, "root").Return(sb, nil).Maybe()
		fx.pickerFx.EXPECT().GetObject(mock.Anything, "childColl").Return(childColl, nil).Maybe()
		fx.mockSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()
		fx.expectUpload(t, 1)

		// when
		proc := &dropFilesProcess{
			spaceId:        sb.SpaceID(),
			processService: fx.processServ,
			picker:         fx.pickerFx,
			service:        fx.fileUploader,
			objectCreator:  fx.objectCreator,
			objectStore:    objectStore,
		}
		err = proc.Init([]string{dir})
		require.NoError(t, err)
		ch := make(chan error)
		go proc.Start("root", false, pb.RpcFileDropRequest{
			DropTargetId: "targetBlock",
			Position:     model.Block_Bottom,
		}, ch)
		err = <-ch

		// then
		assert.Nil(t, err)
		<-proc.Done()
	})

	t.Run("drop nested dirs in collection - preserves structure", func(t *testing.T) {
		// given
		objectStore := spaceindex.NewStoreFixture(t)
		dir := t.TempDir()
		subdir := filepath.Join(dir, "subdir")
		err := os.Mkdir(subdir, 0o755)
		require.NoError(t, err)
		_, err = os.Create(filepath.Join(subdir, "test.txt"))
		require.NoError(t, err)

		fx := newDropFixture(t)
		sb := smarttest.New("root")
		st := sb.Doc.NewState()
		st.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_collection)))
		sb.Doc = st

		cst := &objectWithCollection{SmartTest: sb, Collection: collection.New(sb, noopBacklinksWatcher{})}
		parentColl := newObjectWithCollection("parentColl")
		childColl := newObjectWithCollection("childColl")

		fx.objectCreator.fn = func(_ context.Context, _ string, req objectcreator.CreateObjectRequest) (string, *domain.Details, error) {
			name := req.Details.GetString(bundle.RelationKeyName)
			if name == filepath.Base(dir) {
				return "parentColl", domain.NewDetails(), nil
			}
			return "childColl", domain.NewDetails(), nil
		}

		fx.pickerFx.EXPECT().GetObject(mock.Anything, "root").Return(cst, nil).Maybe()
		fx.pickerFx.EXPECT().GetObject(mock.Anything, "parentColl").Return(parentColl, nil).Maybe()
		fx.pickerFx.EXPECT().GetObject(mock.Anything, "childColl").Return(childColl, nil).Maybe()
		fx.mockSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()
		fx.expectUpload(t, 1)

		// when
		proc := &dropFilesProcess{
			spaceId:        sb.SpaceID(),
			processService: fx.processServ,
			picker:         fx.pickerFx,
			service:        fx.fileUploader,
			objectCreator:  fx.objectCreator,
			objectStore:    objectStore,
		}
		err = proc.Init([]string{dir})
		require.NoError(t, err)
		ch := make(chan error)
		proc.Start("root", true, pb.RpcFileDropRequest{Position: model.Block_Bottom}, ch)
		err = <-ch

		// then
		assert.Nil(t, err)
		rootStore := cst.NewState().GetStoreSlice(template.CollectionStoreKey)
		assert.Contains(t, rootStore, "parentColl")
		parentStore := parentColl.NewState().GetStoreSlice(template.CollectionStoreKey)
		assert.Contains(t, parentStore, "childColl")
	})
}

func TestDropFilesDedup(t *testing.T) {
	t.Run("second call with same checksum reuses existing collection", func(t *testing.T) {
		// given
		objectStore := spaceindex.NewStoreFixture(t)
		createCalls := 0
		objCreator := &dummyObjectCreator{fn: func(_ context.Context, _ string, _ objectcreator.CreateObjectRequest) (string, *domain.Details, error) {
			createCalls++
			return "childColl", domain.NewDetails(), nil
		}}

		dp := &dropFilesProcess{
			spaceId:       "space1",
			objectCreator: objCreator,
			objectStore:   objectStore,
			ctx:           context.Background(),
		}

		checksum := "abc123"

		// when - first call creates
		id1, err := dp.createCollectionForFolder(context.Background(), "myFolder", checksum)
		require.NoError(t, err)
		assert.Equal(t, "childColl", id1)
		assert.Equal(t, 1, createCalls)

		// Simulate that the collection is now in the store
		objectStore.AddObjects(t, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:                 domain.String("childColl"),
				bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_collection)),
				bundle.RelationKeyFileSourceChecksum: domain.String(checksum),
			},
		})

		// then - second call reuses (no additional CreateObject)
		id2, err := dp.createCollectionForFolder(context.Background(), "myFolder", checksum)
		require.NoError(t, err)
		assert.Equal(t, "childColl", id2)
		assert.Equal(t, 1, createCalls)
	})

	t.Run("different checksum creates new collection", func(t *testing.T) {
		// given
		objectStore := spaceindex.NewStoreFixture(t)
		objCreator := &dummyObjectCreator{fn: func(_ context.Context, _ string, req objectcreator.CreateObjectRequest) (string, *domain.Details, error) {
			name := req.Details.GetString(bundle.RelationKeyName)
			if name == "folder1" {
				return "coll1", domain.NewDetails(), nil
			}
			return "coll2", domain.NewDetails(), nil
		}}

		dp := &dropFilesProcess{
			spaceId:       "space1",
			objectCreator: objCreator,
			objectStore:   objectStore,
			ctx:           context.Background(),
		}

		// when
		id1, err := dp.createCollectionForFolder(context.Background(), "folder1", "checksum_aaa")
		require.NoError(t, err)
		assert.Equal(t, "coll1", id1)

		objectStore.AddObjects(t, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:                 domain.String("coll1"),
				bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_collection)),
				bundle.RelationKeyFileSourceChecksum: domain.String("checksum_aaa"),
			},
		})

		id2, err := dp.createCollectionForFolder(context.Background(), "folder2", "checksum_bbb")
		require.NoError(t, err)
		assert.Equal(t, "coll2", id2)
	})

	t.Run("create collection includes fileSourceChecksum in details", func(t *testing.T) {
		// given
		objectStore := spaceindex.NewStoreFixture(t)
		var capturedDetails *domain.Details
		objCreator := &dummyObjectCreator{fn: func(_ context.Context, _ string, req objectcreator.CreateObjectRequest) (string, *domain.Details, error) {
			capturedDetails = req.Details
			return "childColl", domain.NewDetails(), nil
		}}

		dp := &dropFilesProcess{
			spaceId:       "space1",
			objectCreator: objCreator,
			objectStore:   objectStore,
			ctx:           context.Background(),
		}

		// when
		_, err := dp.createCollectionForFolder(context.Background(), "myFolder", "test_checksum_123")
		require.NoError(t, err)

		// then
		require.NotNil(t, capturedDetails)
		assert.Equal(t, "test_checksum_123", capturedDetails.GetString(bundle.RelationKeyFileSourceChecksum))
	})

	t.Run("checksum is computed during Init for directory entries", func(t *testing.T) {
		// given
		dir := t.TempDir()
		_, err := os.Create(filepath.Join(dir, "file_b.txt"))
		require.NoError(t, err)
		_, err = os.Create(filepath.Join(dir, "file_a.txt"))
		require.NoError(t, err)

		dp := &dropFilesProcess{}
		err = dp.Init([]string{dir})
		require.NoError(t, err)

		// then
		require.Len(t, dp.root.children, 1)
		dirEntry := dp.root.children[0]
		assert.True(t, dirEntry.isDir)
		assert.NotEmpty(t, dirEntry.checksum)

		// Verify determinism
		dp2 := &dropFilesProcess{}
		err = dp2.Init([]string{dir})
		require.NoError(t, err)
		assert.Equal(t, dirEntry.checksum, dp2.root.children[0].checksum)
	})
}

func TestComputeDirectoryChecksum(t *testing.T) {
	t.Run("same children produce same checksum", func(t *testing.T) {
		children1 := []*dropFileEntry{{name: "b.txt"}, {name: "a.txt"}}
		children2 := []*dropFileEntry{{name: "a.txt"}, {name: "b.txt"}}
		assert.Equal(t, computeDirectoryChecksum(children1), computeDirectoryChecksum(children2))
	})

	t.Run("different children produce different checksum", func(t *testing.T) {
		children1 := []*dropFileEntry{{name: "a.txt"}}
		children2 := []*dropFileEntry{{name: "b.txt"}}
		assert.NotEqual(t, computeDirectoryChecksum(children1), computeDirectoryChecksum(children2))
	})

	t.Run("empty children produce consistent checksum", func(t *testing.T) {
		children := []*dropFileEntry{}
		c1 := computeDirectoryChecksum(children)
		c2 := computeDirectoryChecksum(children)
		assert.Equal(t, c1, c2)
		assert.NotEmpty(t, c1)
	})
}
