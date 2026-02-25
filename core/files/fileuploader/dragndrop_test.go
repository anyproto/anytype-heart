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

func TestDropFilesInSpace(t *testing.T) {
	t.Run("drop files with empty contextId - uploads to space", func(t *testing.T) {
		// given
		dir := t.TempDir()
		f1, err := os.Create(filepath.Join(dir, "file1.txt"))
		require.NoError(t, err)
		f2, err := os.Create(filepath.Join(dir, "file2.txt"))
		require.NoError(t, err)

		fx := newDropFixture(t)
		fx.mockSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()
		fx.expectUpload(t, 2)

		// when
		proc := &dropFilesProcess{
			spaceId:        "space1",
			processService: fx.processServ,
			picker:         fx.pickerFx,
			service:        fx.fileUploader,
			isDropInSpace:  true,
		}
		err = proc.Init([]string{f1.Name(), f2.Name()})
		require.NoError(t, err)

		ch := make(chan error)
		go proc.Start("", false, pb.RpcFileDropRequest{}, ch)
		err = <-ch

		// then
		assert.NoError(t, err)
		<-proc.Done()
	})

	t.Run("drop dir with empty contextId - uploads files flat", func(t *testing.T) {
		// given
		dir := t.TempDir()
		_, err := os.Create(filepath.Join(dir, "test.txt"))
		require.NoError(t, err)

		fx := newDropFixture(t)
		fx.mockSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()
		fx.expectUpload(t, 1)

		// when
		proc := &dropFilesProcess{
			spaceId:        "space1",
			processService: fx.processServ,
			picker:         fx.pickerFx,
			service:        fx.fileUploader,
			isDropInSpace:  true,
		}
		err = proc.Init([]string{dir})
		require.NoError(t, err)

		// then — directory is flattened, only files are collected
		assert.Equal(t, int64(1), proc.total)
		require.Len(t, proc.root.children, 1)
		assert.False(t, proc.root.children[0].isDir)

		ch := make(chan error)
		go proc.Start("", false, pb.RpcFileDropRequest{}, ch)
		err = <-ch
		assert.NoError(t, err)
		<-proc.Done()
	})

	t.Run("drop mix of files and dirs with empty contextId - all flat", func(t *testing.T) {
		// given
		base := t.TempDir()
		subdir := filepath.Join(base, "subdir")
		err := os.Mkdir(subdir, 0o755)
		require.NoError(t, err)
		_, err = os.Create(filepath.Join(subdir, "nested.txt"))
		require.NoError(t, err)

		file1, err := os.Create(filepath.Join(base, "standalone.txt"))
		require.NoError(t, err)

		fx := newDropFixture(t)
		fx.mockSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()
		// 1 standalone file + 1 nested file = 2 uploads, all flat
		fx.expectUpload(t, 2)

		// when
		proc := &dropFilesProcess{
			spaceId:        "space1",
			processService: fx.processServ,
			picker:         fx.pickerFx,
			service:        fx.fileUploader,
			isDropInSpace:  true,
		}
		err = proc.Init([]string{file1.Name(), subdir})
		require.NoError(t, err)

		// then — all entries are flat files
		assert.Equal(t, int64(2), proc.total)
		for _, child := range proc.root.children {
			assert.False(t, child.isDir)
		}

		ch := make(chan error)
		go proc.Start("", false, pb.RpcFileDropRequest{}, ch)
		err = <-ch
		assert.NoError(t, err)
		<-proc.Done()
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

func TestDetectFileType(t *testing.T) {
	t.Run("png file detected as image", func(t *testing.T) {
		// given — minimal PNG header (magic bytes)
		dir := t.TempDir()
		path := filepath.Join(dir, "photo.png")
		pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		err := os.WriteFile(path, pngHeader, 0o644)
		assert.NoError(t, err)

		// when
		got := detectFileType(path, "photo.png")

		// then
		assert.Equal(t, model.BlockContentFile_Image, got)
	})

	t.Run("mp3 file detected as audio", func(t *testing.T) {
		// given — ID3v2 header (common MP3 tag)
		dir := t.TempDir()
		path := filepath.Join(dir, "song.mp3")
		id3Header := []byte("ID3\x04\x00\x00\x00\x00\x00\x00")
		err := os.WriteFile(path, id3Header, 0o644)
		assert.NoError(t, err)

		// when
		got := detectFileType(path, "song.mp3")

		// then
		assert.Equal(t, model.BlockContentFile_Audio, got)
	})

	t.Run("non-existent file returns File type", func(t *testing.T) {
		got := detectFileType("/nonexistent/path", "file.txt")
		assert.Equal(t, model.BlockContentFile_File, got)
	})

	t.Run("empty file returns generic file type", func(t *testing.T) {
		// given
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.txt")
		err := os.WriteFile(path, []byte{}, 0o644)
		assert.NoError(t, err)

		// when
		got := detectFileType(path, "empty.txt")

		// then — empty files can't be identified by magic bytes
		assert.Contains(t, []model.BlockContentFileType{
			model.BlockContentFile_File,
			model.BlockContentFile_None,
		}, got)
	})
}

func TestIsTypeFilterActive(t *testing.T) {
	t.Run("Image is active", func(t *testing.T) {
		assert.True(t, isTypeFilterActive(model.BlockContentFile_Image))
	})
	t.Run("Audio is active", func(t *testing.T) {
		assert.True(t, isTypeFilterActive(model.BlockContentFile_Audio))
	})
	t.Run("Video is active", func(t *testing.T) {
		assert.True(t, isTypeFilterActive(model.BlockContentFile_Video))
	})
	t.Run("File is active", func(t *testing.T) {
		assert.True(t, isTypeFilterActive(model.BlockContentFile_File))
	})
	t.Run("PDF is active", func(t *testing.T) {
		assert.True(t, isTypeFilterActive(model.BlockContentFile_PDF))
	})
	t.Run("None is not active", func(t *testing.T) {
		assert.False(t, isTypeFilterActive(model.BlockContentFile_None))
	})
}

func TestFilterFileType(t *testing.T) {
	t.Run("Image maps to Image", func(t *testing.T) {
		assert.Equal(t, model.BlockContentFile_Image, filterFileType(model.BlockContentFile_Image))
	})
	t.Run("Audio maps to Audio", func(t *testing.T) {
		assert.Equal(t, model.BlockContentFile_Audio, filterFileType(model.BlockContentFile_Audio))
	})
	t.Run("Video maps to Video", func(t *testing.T) {
		assert.Equal(t, model.BlockContentFile_Video, filterFileType(model.BlockContentFile_Video))
	})
	t.Run("File maps to File", func(t *testing.T) {
		assert.Equal(t, model.BlockContentFile_File, filterFileType(model.BlockContentFile_File))
	})
	t.Run("PDF maps to File", func(t *testing.T) {
		assert.Equal(t, model.BlockContentFile_File, filterFileType(model.BlockContentFile_PDF))
	})
}

func TestDropFilesTypeFilter(t *testing.T) {
	t.Run("type=Image filters to only image files", func(t *testing.T) {
		// given — 2 PNGs + 1 text file
		dir := t.TempDir()
		pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

		require.NoError(t, os.WriteFile(filepath.Join(dir, "photo1.png"), pngHeader, 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "photo2.png"), pngHeader, 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o644))

		// when
		proc := &dropFilesProcess{
			fileType:      model.BlockContentFile_Image,
			isDropInSpace: true,
		}
		err := proc.Init([]string{
			filepath.Join(dir, "photo1.png"),
			filepath.Join(dir, "photo2.png"),
			filepath.Join(dir, "readme.txt"),
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, int64(2), proc.total)
		assert.Len(t, proc.root.children, 2)
	})

	t.Run("type=Image recursively collects images from directories", func(t *testing.T) {
		// given — a directory with nested image and text file
		dir := t.TempDir()
		subdir := filepath.Join(dir, "subdir")
		require.NoError(t, os.Mkdir(subdir, 0o755))

		pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		require.NoError(t, os.WriteFile(filepath.Join(subdir, "nested.png"), pngHeader, 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(subdir, "readme.txt"), []byte("hello"), 0o644))

		// when
		proc := &dropFilesProcess{
			fileType:      model.BlockContentFile_Image,
			isDropInSpace: true,
		}
		err := proc.Init([]string{subdir})

		// then — only the nested image is collected, no directory entries
		require.NoError(t, err)
		assert.Equal(t, int64(1), proc.total)
		require.Len(t, proc.root.children, 1)
		assert.Equal(t, "nested.png", proc.root.children[0].name)
		assert.False(t, proc.root.children[0].isDir)
	})

	t.Run("type=None collects all files flat without directories", func(t *testing.T) {
		// given — directory with an image and a text file
		dir := t.TempDir()
		pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		require.NoError(t, os.WriteFile(filepath.Join(dir, "photo.png"), pngHeader, 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644))

		// when
		proc := &dropFilesProcess{
			fileType:      model.BlockContentFile_None,
			isDropInSpace: true,
		}
		err := proc.Init([]string{dir})

		// then — both files collected flat, no directory entry
		require.NoError(t, err)
		assert.Equal(t, int64(2), proc.total)
		require.Len(t, proc.root.children, 2)
		for _, child := range proc.root.children {
			assert.False(t, child.isDir)
		}
	})

	t.Run("type=File filters out images and keeps generic files", func(t *testing.T) {
		// given — 1 generic file + 1 PNG image
		dir := t.TempDir()
		pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		require.NoError(t, os.WriteFile(filepath.Join(dir, "data.bin"), []byte{0x00, 0x01}, 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "photo.png"), pngHeader, 0o644))

		// when
		proc := &dropFilesProcess{
			fileType:      model.BlockContentFile_File,
			isDropInSpace: true,
		}
		err := proc.Init([]string{
			filepath.Join(dir, "data.bin"),
			filepath.Join(dir, "photo.png"),
		})

		// then — only the generic file is included, image is filtered out
		require.NoError(t, err)
		assert.Equal(t, int64(1), proc.total)
		require.Len(t, proc.root.children, 1)
		assert.Equal(t, "data.bin", proc.root.children[0].name)
	})

	t.Run("type filter ignored when dropping to editor", func(t *testing.T) {
		// given — 1 PNG + 1 text file dropped into editor (isDropInSpace=false)
		dir := t.TempDir()
		pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		require.NoError(t, os.WriteFile(filepath.Join(dir, "photo.png"), pngHeader, 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o644))

		// when — fileType is set but isDropInSpace is false
		proc := &dropFilesProcess{
			fileType:      model.BlockContentFile_Image,
			isDropInSpace: false,
		}
		err := proc.Init([]string{
			filepath.Join(dir, "photo.png"),
			filepath.Join(dir, "readme.txt"),
		})

		// then — both files are included, no filtering applied
		require.NoError(t, err)
		assert.Equal(t, int64(2), proc.total)
		assert.Len(t, proc.root.children, 2)
	})

	t.Run("type=Image with full upload flow including directory", func(t *testing.T) {
		// given — 1 PNG file + 1 dir containing 1 PNG and 1 text file
		dir := t.TempDir()
		pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

		require.NoError(t, os.WriteFile(filepath.Join(dir, "img1.png"), pngHeader, 0o644))
		subdir := filepath.Join(dir, "photos")
		require.NoError(t, os.Mkdir(subdir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(subdir, "img2.png"), pngHeader, 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(subdir, "doc.txt"), []byte("text"), 0o644))

		fx := newDropFixture(t)
		fx.mockSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()
		// 1 top-level PNG + 1 nested PNG = 2 uploads
		fx.expectUpload(t, 2)

		// when
		proc := &dropFilesProcess{
			spaceId:        "space1",
			processService: fx.processServ,
			picker:         fx.pickerFx,
			service:        fx.fileUploader,
			isDropInSpace:  true,
			fileType:       model.BlockContentFile_Image,
		}
		err := proc.Init([]string{
			filepath.Join(dir, "img1.png"),
			subdir,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(2), proc.total)

		ch := make(chan error)
		go proc.Start("", false, pb.RpcFileDropRequest{}, ch)
		err = <-ch

		// then
		assert.NoError(t, err)
		<-proc.Done()
	})
}
