package fileobject

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/fileobject"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/core/files/fileoffloader"
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	"github.com/anyproto/anytype-heart/core/files/filesync/mock_filesync"
	"github.com/anyproto/anytype-heart/core/relationutils/mock_relationutils"
	wallet2 "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	fileService       files.Service
	objectStore       *objectstore.StoreFixture
	objectCreator     *objectCreatorStub
	spaceService      *mock_space.MockService
	spaceIdResolver   *mock_idresolver.MockResolver
	commonFileService fileservice.FileService
	*service
}

type dummyAccountService struct{}

func (s *dummyAccountService) MyParticipantId(spaceId string) string {
	return ""
}

func (s *dummyAccountService) Init(_ *app.App) error { return nil }

func (s *dummyAccountService) Name() string { return "dummyAccountService" }

type dummyConfig struct{}

func (c *dummyConfig) IsLocalOnlyMode() bool {
	return false
}

func (c *dummyConfig) Init(_ *app.App) error {
	return nil
}

func (c *dummyConfig) Name() string {
	return "dummyConfig"
}

type dummyObjectArchiver struct{}

func (a *dummyObjectArchiver) SetListIsArchived(_ context.Context, _ []string, _ bool) error {
	return nil
}

func (a *dummyObjectArchiver) Name() string { return "dummyObjectArchiver" }

func (a *dummyObjectArchiver) Init(_ *app.App) error { return nil }

const testResolveRetryDelay = 5 * time.Millisecond

func newFixture(t *testing.T) *fixture {
	objectStore := objectstore.NewStoreFixture(t)
	objectCreator := &objectCreatorStub{}

	blockStorage := filestorage.NewInMemory()
	commonFileService := fileservice.New()
	fileSyncService := mock_filesync.NewMockFileSync(t)
	fileSyncService.EXPECT().AddFile(mock.Anything).Return(nil).Maybe()

	eventSender := mock_event.NewMockSender(t)
	eventSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()
	fileService := files.New()
	spaceService := mock_space.NewMockService(t)
	spaceService.EXPECT().GetPersonalSpace(mock.Anything).Return(nil, fmt.Errorf("not needed")).Maybe()
	spaceService.EXPECT().PersonalSpaceId().Return("personalSpaceId").Maybe()
	spaceIdResolver := mock_idresolver.NewMockResolver(t)

	svc := New()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	wallet := mock_wallet.NewMockWallet(t)
	wallet.EXPECT().Name().Return(wallet2.CName)
	wallet.EXPECT().RepoPath().Return(t.TempDir())

	fetcher := mock_relationutils.NewMockRelationFormatFetcher(t)
	fetcher.EXPECT().GetRelationFormatByKey(mock.Anything, mock.Anything).RunAndReturn(func(_ string, key domain.RelationKey) (model.RelationFormat, error) {
		rel, err := bundle.GetRelation(key)
		if err != nil {
			return 0, err
		}
		return rel.Format, nil
	}).Maybe()

	a := new(app.App)
	a.Register(&dummyConfig{})
	a.Register(&dummyAccountService{})
	a.Register(anystoreprovider.New())
	a.Register(objectStore)
	a.Register(commonFileService)
	a.Register(testutil.PrepareMock(ctx, a, fileSyncService))
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(testutil.PrepareMock(ctx, a, spaceService))
	a.Register(blockStorage)
	a.Register(fileService)
	a.Register(objectCreator)
	a.Register(svc)
	a.Register(testutil.PrepareMock(ctx, a, spaceIdResolver))
	a.Register(fileoffloader.New())
	a.Register(testutil.PrepareMock(ctx, a, mock_accountservice.NewMockService(ctrl)))
	a.Register(testutil.PrepareMock(ctx, a, wallet))
	a.Register(&config.Config{DisableFileConfig: true, NetworkMode: pb.RpcAccount_DefaultConfig, PeferYamuxTransport: true})
	a.Register(&dummyObjectArchiver{})
	a.Register(testutil.PrepareMock(ctx, a, fetcher))

	err := a.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := a.Close(ctx)
		require.NoError(t, err)
	})

	fx := &fixture{
		fileService:       fileService,
		objectStore:       objectStore,
		objectCreator:     objectCreator,
		spaceService:      spaceService,
		spaceIdResolver:   spaceIdResolver,
		commonFileService: commonFileService,

		service: svc.(*service),
	}
	return fx
}

type objectCreatorStub struct {
	objectId      string
	creationState *state.State
	details       *domain.Details
}

func (c *objectCreatorStub) Init(_ *app.App) error {
	return nil
}

func (c *objectCreatorStub) Name() string {
	return "objectCreatorStub"
}

func (c *objectCreatorStub) CreateSmartBlockFromStateInSpaceWithOptions(ctx context.Context, space clientspace.Space, objectTypeKeys []domain.TypeKey, createState *state.State, opts ...objectcreator.CreateOption) (id string, newDetails *domain.Details, err error) {
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

type fileObjectWrapper struct {
	smartblock.SmartBlock
	fileobject.FileObject
}

func (fx *fixture) newTestFileObject(sb smartblock.SmartBlock) *fileObjectWrapper {
	return &fileObjectWrapper{SmartBlock: sb, FileObject: fileobject.NewFileObject(sb, fx.fileService)}
}

func TestGetFileIdFromObjectWaitLoad(t *testing.T) {
	t.Run("with invalid id expect error", func(t *testing.T) {
		fx := newFixture(t)
		err := fx.DoFileWaitLoad(context.Background(), "invalid", func(object fileobject.FileObject) error {
			return nil
		})
		require.Error(t, err)
	})

	t.Run("with file id expect error", func(t *testing.T) {
		fx := newFixture(t)
		err := fx.DoFileWaitLoad(context.Background(), testFileId.String(), func(object fileobject.FileObject) error {
			return nil
		})
		require.Error(t, err)
	})

	t.Run("with loaded object without file id expect error", func(t *testing.T) {
		fx := newFixture(t)

		ctx := context.Background()
		spaceId := "spaceId"
		fx.spaceIdResolver.EXPECT().ResolveSpaceIdWithRetry(mock.Anything, testFileObjectId).Return(spaceId, nil)

		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().Do(testFileObjectId, mock.Anything).RunAndReturn(func(_ string, apply func(smartblock.SmartBlock) error) error {
			sb := smarttest.New(testFileObjectId)
			sb.SetSpaceId(spaceId)

			st := sb.Doc.(*state.State)
			st.SetDetailAndBundledRelation(bundle.RelationKeyFileId, domain.String(""))

			return apply(fx.newTestFileObject(sb))
		})

		fx.spaceService.EXPECT().Get(ctx, spaceId).Return(space, nil)

		err := fx.DoFileWaitLoad(ctx, testFileObjectId, func(object fileobject.FileObject) error {
			return nil
		})
		require.ErrorIs(t, err, filemodels.ErrEmptyFileId)
	})
}
