package filesync

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	mock_rpcstore2 "github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore/mock_rpcstore"
	wallet2 "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

func TestCancelDeletion(t *testing.T) {
	sender := mock_event.NewMockSender(t)
	sender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

	rpcStore := mock_rpcstore2.NewMockRpcStore(t)
	rpcStoreService := mock_rpcstore2.NewMockService(t)
	rpcStoreService.EXPECT().NewStore().Return(rpcStore)
	rpcStore.EXPECT().AccountInfo(mock.Anything).Return(&fileproto.AccountInfoResponse{}, nil).Maybe()

	localFileStorage := filestorage.NewInMemory()

	dataStoreProvider, err := datastore.NewInMemory()
	require.NoError(t, err)

	fileService := fileservice.New()
	ctrl := gomock.NewController(t)
	wallet := mock_wallet.NewMockWallet(t)
	wallet.EXPECT().Name().Return(wallet2.CName)

	a := new(app.App)
	a.Register(dataStoreProvider)
	a.Register(localFileStorage)
	a.Register(fileService)
	a.Register(testutil.PrepareMock(ctx, a, rpcStoreService))
	a.Register(testutil.PrepareMock(ctx, a, sender))
	a.Register(testutil.PrepareMock(ctx, a, mock_accountservice.NewMockService(ctrl)))
	a.Register(testutil.PrepareMock(ctx, a, wallet))
	a.Register(&config.Config{DisableFileConfig: true, NetworkMode: pb.RpcAccount_DefaultConfig, PeferYamuxTransport: true})

	s := New().(*fileSync)
	err = s.Init(a)

	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	require.NoError(t, err)
	s.store, err = newFileSyncStore(db)
	require.NoError(t, err)

	require.NoError(t, err)

	testObjectId1 := "objectId1"
	testFileId1 := domain.FullFileId{SpaceId: "spaceId", FileId: "bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku"}

	testObjectId2 := "objectId2"
	testFileId2 := domain.FullFileId{SpaceId: "spaceId", FileId: "bafybeiasl27gslws4hpvzufm467zjhxb3klodj53rt6dpola67bmvep3x4"}

	s.loopCtx = context.Background()

	err = s.deletionQueue.Add(&deletionQueueItem{
		ObjectId: testObjectId1,
		SpaceId:  testFileId1.SpaceId,
		FileId:   testFileId1.FileId,
	})
	require.NoError(t, err)

	err = s.retryDeletionQueue.Add(&deletionQueueItem{
		ObjectId: testObjectId2,
		SpaceId:  testFileId2.SpaceId,
		FileId:   testFileId2.FileId,
	})
	require.NoError(t, err)

	rpcStore.EXPECT().DeleteFiles(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	s.deletionQueue.Run()
	s.retryDeletionQueue.Run()

	err = s.CancelDeletion(testObjectId1, testFileId1)
	require.NoError(t, err)

	err = s.CancelDeletion(testObjectId2, testFileId2)
	require.NoError(t, err)

	assert.Zero(t, s.deletionQueue.Len())
	assert.Zero(t, s.retryDeletionQueue.Len())
}
