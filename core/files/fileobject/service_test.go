package fileobject

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/mutex"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type fixture struct {
	fileService     files.Service
	objectStore     *objectstore.StoreFixture
	objectCreator   *objectCreatorStub
	spaceService    *mock_space.MockService
	spaceIdResolver *mock_idresolver.MockResolver
	*service
}

const testResolveRetryDelay = 5 * time.Millisecond

func newFixture(t *testing.T) *fixture {
	fileStore := filestore.New()
	objectStore := objectstore.NewStoreFixture(t)
	objectCreator := &objectCreatorStub{}
	dataStoreProvider, err := datastore.NewInMemory()
	require.NoError(t, err)
	blockStorage := filestorage.NewInMemory()
	rpcStore := rpcstore.NewInMemoryStore(1024)
	rpcStoreService := rpcstore.NewInMemoryService(rpcStore)
	commonFileService := fileservice.New()
	fileSyncService := filesync.New()
	eventSender := mock_event.NewMockSender(t)
	fileService := files.New()
	spaceService := mock_space.NewMockService(t)
	spaceIdResolver := mock_idresolver.NewMockResolver(t)

	svc := New(testResolveRetryDelay, testResolveRetryDelay)

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
	a.Register(testutil.PrepareMock(ctx, a, spaceIdResolver))

	err = a.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := a.Close(ctx)
		require.NoError(t, err)
	})

	fx := &fixture{
		fileService:     fileService,
		objectStore:     objectStore,
		objectCreator:   objectCreator,
		spaceService:    spaceService,
		spaceIdResolver: spaceIdResolver,

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

const testFileObjectId = "bafyreiebxsn65332wl7qavcxxkfwnsroba5x5h2sshcn7f7cr66ztixb54"

func TestGetFileIdFromObjectWaitLoad(t *testing.T) {
	t.Run("with invalid id expect error", func(t *testing.T) {
		fx := newFixture(t)
		_, err := fx.GetFileIdFromObjectWaitLoad(context.Background(), "invalid")
		require.Error(t, err)
	})

	t.Run("with file id expect error", func(t *testing.T) {
		fx := newFixture(t)
		_, err := fx.GetFileIdFromObjectWaitLoad(context.Background(), testFileId.String())
		require.Error(t, err)
	})

	t.Run("with not yet loaded object load object and when timed out expect return error", func(t *testing.T) {
		fx := newFixture(t)

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		fx.spaceIdResolver.EXPECT().ResolveSpaceID(testFileObjectId).Return("", fmt.Errorf("not yet resolved"))

		_, err := fx.GetFileIdFromObjectWaitLoad(ctx, testFileObjectId)
		require.Error(t, err)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("with not yet loaded object load object and return it's file id", func(t *testing.T) {
		fx := newFixture(t)

		ctx := context.Background()
		spaceId := "spaceId"
		resolvedSpace := mutex.NewValue("")
		resolvedSpaceErr := mutex.NewValue(fmt.Errorf("not yet resolved"))
		fx.spaceIdResolver.EXPECT().ResolveSpaceID(testFileObjectId).RunAndReturn(func(_ string) (string, error) {
			return resolvedSpace.Get(), resolvedSpaceErr.Get()
		})

		go func() {
			time.Sleep(3 * testResolveRetryDelay)
			resolvedSpace.Set(spaceId)
			resolvedSpaceErr.Set(nil)
		}()

		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().Do(testFileObjectId, mock.Anything).RunAndReturn(func(_ string, apply func(smartblock.SmartBlock) error) error {
			sb := smarttest.New(testFileObjectId)

			st := sb.Doc.(*state.State)
			st.SetDetailAndBundledRelation(bundle.RelationKeyFileId, pbtypes.String(testFileId.String()))

			return apply(sb)
		})

		fx.spaceService.EXPECT().Get(ctx, spaceId).Return(space, nil)

		id, err := fx.GetFileIdFromObjectWaitLoad(ctx, testFileObjectId)
		require.NoError(t, err)
		assert.Equal(t, domain.FullFileId{
			SpaceId: spaceId,
			FileId:  testFileId,
		}, id)
	})

	t.Run("with loaded object without file id expect error", func(t *testing.T) {
		fx := newFixture(t)

		ctx := context.Background()
		spaceId := "spaceId"
		fx.spaceIdResolver.EXPECT().ResolveSpaceID(testFileObjectId).Return(spaceId, nil)

		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().Do(testFileObjectId, mock.Anything).RunAndReturn(func(_ string, apply func(smartblock.SmartBlock) error) error {
			sb := smarttest.New(testFileObjectId)

			st := sb.Doc.(*state.State)
			st.SetDetailAndBundledRelation(bundle.RelationKeyFileId, pbtypes.String(""))

			return apply(sb)
		})

		fx.spaceService.EXPECT().Get(ctx, spaceId).Return(space, nil)

		_, err := fx.GetFileIdFromObjectWaitLoad(ctx, testFileObjectId)
		require.ErrorIs(t, err, ErrEmptyFileId)
	})
}
