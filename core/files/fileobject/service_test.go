package fileobject

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	bb "github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type fixture struct {
	fileService   files.Service
	objectStore   *objectstore.StoreFixture
	objectCreator *objectCreatorStub
	spaceService  *mock_space.MockService

	*service
}

type dummyObjectGetter struct{}

func (g *dummyObjectGetter) Init(_ *app.App) error {
	return nil
}

func (g *dummyObjectGetter) Name() string {
	return "dummyObjectGetter"
}

func (g *dummyObjectGetter) GetObject(ctx context.Context, id string) (smartblock.SmartBlock, error) {
	return nil, nil
}

func newFixture(t *testing.T) *fixture {
	fileStore := filestore.New()
	objectStore := objectstore.NewStoreFixture(t)
	objectCreator := &objectCreatorStub{}
	dataStoreProvider := datastore.NewInMemory()
	blockStorage := filestorage.NewInMemory()
	rpcStore := rpcstore.NewInMemoryStore(1024)
	rpcStoreService := rpcstore.NewInMemoryService(rpcStore)
	commonFileService := fileservice.New()
	fileSyncService := filesync.New()
	eventSender := mock_event.NewMockSender(t)
	fileService := files.New()
	spaceService := mock_space.NewMockService(t)
	objectGetter := &dummyObjectGetter{}

	svc := New()

	ctx := context.Background()
	a := new(app.App)
	a.Register(dataStoreProvider)
	a.Register(fileStore)
	a.Register(objectStore)
	a.Register(commonFileService)
	a.Register(fileSyncService)
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(testutil.PrepareMock(ctx, a, spaceService))
	a.Register(blockStorage)
	a.Register(rpcStoreService)
	a.Register(fileService)
	a.Register(objectCreator)
	a.Register(svc)
	a.Register(objectGetter)

	err := a.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := a.Close(ctx)
		require.NoError(t, err)
	})

	fx := &fixture{
		fileService:   fileService,
		objectStore:   objectStore,
		objectCreator: objectCreator,
		spaceService:  spaceService,

		service: svc.(*service),
	}
	return fx
}

type objectCreatorStub struct {
	objectId      string
	creationState *state.State
	details       *types.Struct
}

func (c *objectCreatorStub) Init(_ *app.App) error {
	return nil
}

func (c *objectCreatorStub) Name() string {
	return "objectCreatorStub"
}

func (c *objectCreatorStub) CreateSmartBlockFromStateInSpaceWithOptions(ctx context.Context, space clientspace.Space, objectTypeKeys []domain.TypeKey, createState *state.State, opts ...objectcreator.CreateOption) (id string, newDetails *types.Struct, err error) {
	c.creationState = createState
	return c.objectId, c.details, nil
}

const testFileId = domain.FileId("bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku")

func TestMigration(t *testing.T) {
	t.Run("do not migrate empty file ids", func(t *testing.T) {
		fx := newFixture(t)

		fileId := ""
		st := testutil.BuildStateFromAST(
			bb.Root(
				bb.ID("root"),
				bb.Children(bb.File("", bb.FileHash(fileId))),
			),
		)

		space := mock_clientspace.NewMockSpace(t)

		fx.MigrateBlocks(st, space, nil)
	})

	t.Run("do not migrate object itself", func(t *testing.T) {
		fx := newFixture(t)

		objectId := "objectId"
		fileId := objectId
		st := testutil.BuildStateFromAST(
			bb.Root(
				bb.ID(objectId),
				bb.Children(bb.File("", bb.FileHash(fileId))),
			),
		)

		space := mock_clientspace.NewMockSpace(t)

		fx.MigrateBlocks(st, space, nil)
	})

	t.Run("do not migrate already migrated file: fileId equals to objectId", func(t *testing.T) {
		fx := newFixture(t)

		objectId := "objectId"
		st := testutil.BuildStateFromAST(
			bb.Root(
				bb.ID(objectId),
				bb.Children(bb.File("", bb.FileHash(testFileId.String()))),
			),
		)

		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().Do(testFileId.String(), mock.Anything).Return(nil)

		fx.MigrateBlocks(st, space, nil)
	})

	t.Run("do not create new object for already migrated file: objectId is found by fileId in current space", func(t *testing.T) {
		fx := newFixture(t)

		spaceId := "spaceId"
		objectId := "objectId"
		expectedFileObjectId := "fileObjectId"
		st := testutil.BuildStateFromAST(
			bb.Root(
				bb.ID(objectId),
			),
		)
		st.SetDetailAndBundledRelation(bundle.RelationKeyAttachments, pbtypes.StringList([]string{testFileId.String()}))

		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().Do(testFileId.String(), mock.Anything).Return(ocache.ErrNotExists)
		space.EXPECT().Id().Return(spaceId)

		fx.objectStore.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      pbtypes.String(expectedFileObjectId),
				bundle.RelationKeyFileId:  pbtypes.String(testFileId.String()),
				bundle.RelationKeySpaceId: pbtypes.String(spaceId),
			},
		})

		fx.MigrateBlocks(st, space, nil)
		fx.MigrateDetails(st, space, nil)

		wantState := testutil.BuildStateFromAST(
			bb.Root(
				bb.ID(objectId),
			),
		)
		wantState.SetDetailAndBundledRelation(bundle.RelationKeyAttachments, pbtypes.StringList([]string{expectedFileObjectId}))

		bb.AssertTreesEqual(t, wantState.Blocks(), st.Blocks())
		assert.Equal(t, wantState.Details(), st.Details())
	})

	t.Run("do not migrate already migrated file: migrated objectId has different CID format", func(t *testing.T) {
		fx := newFixture(t)

		objectId := "objectId"
		fileId := domain.FileId("fileObjectIdFromAnotherSpace")
		st := testutil.BuildStateFromAST(
			bb.Root(
				bb.ID(objectId),
			),
		)
		st.SetDetailAndBundledRelation(bundle.RelationKeyAttachments, pbtypes.StringList([]string{fileId.String()}))

		space := mock_clientspace.NewMockSpace(t)

		fx.objectStore.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      pbtypes.String(fileId.String()),
				bundle.RelationKeyFileId:  pbtypes.String("fileId"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId2"),
			},
		})

		fx.MigrateBlocks(st, space, nil)
		fx.MigrateDetails(st, space, nil)

		wantState := testutil.BuildStateFromAST(
			bb.Root(
				bb.ID(objectId),
			),
		)
		wantState.SetDetailAndBundledRelation(bundle.RelationKeyAttachments, pbtypes.StringList([]string{fileId.String()}))

		bb.AssertTreesEqual(t, wantState.Blocks(), st.Blocks())
		assert.Equal(t, wantState.Details(), st.Details())
	})

	t.Run("when file is not migrated yet: derive new object", func(t *testing.T) {
		fx := newFixture(t)

		spaceId := "spaceId"
		addedFile := testAddFile(t, fx, spaceId)

		objectId := "objectId"
		fileId := addedFile.FileId
		expectedFileObjectId := "fileObjectId"
		st := testutil.BuildStateFromAST(
			bb.Root(
				bb.ID(objectId),
				bb.Children(
					bb.File("", bb.FileHash(fileId.String())),
					bb.Text("sample text", bb.TextIconImage(fileId.String())),
				),
			),
		)
		st.SetDetailAndBundledRelation(bundle.RelationKeyIconImage, pbtypes.StringList([]string{fileId.String()}))

		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().Do(fileId.String(), mock.Anything).Return(ocache.ErrNotExists)
		space.EXPECT().Id().Return(spaceId)
		space.EXPECT().DeriveObjectIdWithAccountSignature(mock.Anything, mock.Anything).Return(expectedFileObjectId, nil)

		origin := objectorigin.Import(model.Import_Html)
		err := fx.fileStore.SetFileOrigin(fileId, origin)
		require.NoError(t, err)

		fx.objectCreator.objectId = expectedFileObjectId

		waitIndexerCh := make(chan struct{}, 1)
		expectIndexerCalled(t, fx, waitIndexerCh, expectedFileObjectId)

		keys := map[string]string{
			"filepath": "key",
		}
		keysChanges := []*pb.ChangeFileKeys{
			{
				Hash: fileId.String(),
				Keys: keys,
			},
		}
		fx.MigrateBlocks(st, space, keysChanges)
		fx.MigrateDetails(st, space, keysChanges)

		assert.Equal(t, pbtypes.GetInt64(fx.objectCreator.creationState.Details(), bundle.RelationKeyOrigin.String()), int64(origin.Origin))
		assert.Equal(t, pbtypes.GetInt64(fx.objectCreator.creationState.Details(), bundle.RelationKeyImportType.String()), int64(origin.ImportType))
		assert.Equal(t, state.FileInfo{
			FileId:         fileId,
			EncryptionKeys: keys,
		}, fx.objectCreator.creationState.GetFileInfo())

		wantState := testutil.BuildStateFromAST(
			bb.Root(
				bb.ID(objectId),
				bb.Children(
					bb.File(expectedFileObjectId, bb.FileHash(fileId.String())),
					bb.Text("sample text", bb.TextIconImage(expectedFileObjectId)),
				),
			),
		)
		wantState.SetDetailAndBundledRelation(bundle.RelationKeyIconImage, pbtypes.StringList([]string{expectedFileObjectId}))

		bb.AssertTreesEqual(t, wantState.Blocks(), st.Blocks())
		assert.Equal(t, wantState.Details(), st.Details())

		select {
		case <-waitIndexerCh:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("indexer was not called")
		}
	})
}

func testAddFile(t *testing.T, fx *fixture, spaceId string) *files.AddResult {
	fileName := "myFile"
	lastModifiedDate := time.Now()
	fileContent := "it's my favorite file"
	buf := strings.NewReader(fileContent)
	opts := []files.AddOption{
		files.WithName(fileName),
		files.WithLastModifiedDate(lastModifiedDate.Unix()),
		files.WithReader(buf),
	}
	got, err := fx.fileService.FileAdd(context.Background(), spaceId, opts...)
	require.NoError(t, err)
	got.Commit()
	return got
}

func expectIndexerCalled(t *testing.T, fx *fixture, waitIndexerCh chan struct{}, fileObjectId string) {
	space := mock_clientspace.NewMockSpace(t)
	space.EXPECT().Do(fileObjectId, mock.Anything).RunAndReturn(func(_ string, _ func(smartblock.SmartBlock) error) error {
		waitIndexerCh <- struct{}{}
		return nil
	})

	fx.spaceService.EXPECT().Get(mock.Anything, mock.Anything).Return(space, nil)
}
