package fileobject

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	bb "github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type fixture struct {
	objectStore   *objectstore.StoreFixture
	fileService   *mock_files.MockService
	objectCreator *objectCreatorStub

	*service
}

func newFixture(t *testing.T) *fixture {
	fileStore := filestore.New()
	objectStore := objectstore.NewStoreFixture(t)
	fileService := mock_files.NewMockService(t)
	objectCreator := &objectCreatorStub{}

	ctx := context.Background()
	a := new(app.App)
	a.Register(datastore.NewInMemory())
	a.Register(fileStore)
	a.Register(objectStore)
	a.Register(testutil.PrepareMock(ctx, a, fileService))

	err := a.Start(ctx)
	require.NoError(t, err)

	svc := &service{
		objectStore:   objectStore,
		fileStore:     fileStore,
		fileService:   fileService,
		objectCreator: objectCreator,
	}

	fx := &fixture{
		objectStore:   objectStore,
		fileService:   fileService,
		objectCreator: objectCreator,

		service: svc,
	}
	return fx
}

type objectCreatorStub struct {
	objectId      string
	creationState *state.State
	details       *types.Struct
}

func (c *objectCreatorStub) CreateSmartBlockFromStateInSpace(ctx context.Context, space clientspace.Space, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *types.Struct, err error) {
	c.creationState = createState
	return c.objectId, c.details, nil
}

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
		fileId := domain.FileId("fileId")
		st := testutil.BuildStateFromAST(
			bb.Root(
				bb.ID(objectId),
				bb.Children(bb.File("", bb.FileHash(fileId.String()))),
			),
		)

		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().Do(fileId.String(), mock.Anything).Return(nil)

		fx.MigrateBlocks(st, space, nil)
	})

	t.Run("do not migrate already migrated file: objectId is found by fileId in current space", func(t *testing.T) {
		fx := newFixture(t)

		spaceId := "spaceId"
		objectId := "objectId"
		fileId := domain.FileId("fileId")
		expectedFileObjectId := "fileObjectId"
		st := testutil.BuildStateFromAST(
			bb.Root(
				bb.ID(objectId),
			),
		)
		st.SetDetailAndBundledRelation(bundle.RelationKeyAttachments, pbtypes.StringList([]string{fileId.String()}))

		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().Do(fileId.String(), mock.Anything).Return(ocache.ErrNotExists)
		space.EXPECT().Id().Return(spaceId)

		fx.objectStore.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      pbtypes.String(expectedFileObjectId),
				bundle.RelationKeyFileId:  pbtypes.String(fileId.String()),
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

	t.Run("do not migrate already migrated file: objectId is found by fileId in another space", func(t *testing.T) {
		fx := newFixture(t)

		spaceId := "spaceId"
		objectId := "objectId"
		fileId := domain.FileId("fileObjectIdFromAnotherSpace")
		st := testutil.BuildStateFromAST(
			bb.Root(
				bb.ID(objectId),
			),
		)
		st.SetDetailAndBundledRelation(bundle.RelationKeyAttachments, pbtypes.StringList([]string{fileId.String()}))

		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().Do(fileId.String(), mock.Anything).Return(ocache.ErrNotExists)
		space.EXPECT().Id().Return(spaceId)

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
		objectId := "objectId"
		fileId := domain.FileId("fileId")
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

		file := mock_files.NewMockFile(t)
		file.EXPECT().Info().Return(&storage.FileInfo{
			Media: "text/html",
		})
		file.EXPECT().Details(mock.Anything).Return(&types.Struct{Fields: map[string]*types.Value{}}, bundle.TypeKeyFile, nil)

		fx.fileService.EXPECT().FileByHash(mock.Anything, domain.FullFileId{SpaceId: spaceId, FileId: fileId}).Return(file, nil)

		fx.objectCreator.objectId = expectedFileObjectId

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
	})
}
