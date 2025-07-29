package files

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/globalsign/mgo/bson"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	rpcstore2 "github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	wallet2 "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/mill/schema"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	Service

	eventSender       *mock_event.MockSender
	commonFileService fileservice.FileService
	fileSyncService   filesync.FileSync
	rpcStore          rpcstore2.RpcStore
	objectStore       objectstore.ObjectStore
}

const (
	spaceId         = "space1"
	testFileName    = "myFile"
	testFileContent = "it's my favorite file"
)

func newFixture(t *testing.T) *fixture {
	blockStorage := filestorage.NewInMemory()

	rpcStore := rpcstore2.NewInMemoryStore(1024)
	rpcStoreService := rpcstore2.NewInMemoryService(rpcStore)
	commonFileService := fileservice.New()
	fileSyncService := filesync.New()
	objectStore := objectstore.NewStoreFixture(t)
	eventSender := mock_event.NewMockSender(t)
	eventSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

	ctrl := gomock.NewController(t)
	wallet := mock_wallet.NewMockWallet(t)
	wallet.EXPECT().Name().Return(wallet2.CName)
	wallet.EXPECT().RepoPath().Return(t.TempDir())
	ctx := context.Background()
	a := new(app.App)
	a.Register(anystoreprovider.New())
	a.Register(commonFileService)
	a.Register(fileSyncService)
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(blockStorage)
	a.Register(objectStore)
	a.Register(rpcStoreService)
	a.Register(testutil.PrepareMock(ctx, a, mock_accountservice.NewMockService(ctrl)))
	a.Register(testutil.PrepareMock(ctx, a, wallet))
	a.Register(&config.Config{DisableFileConfig: true, NetworkMode: pb.RpcAccount_DefaultConfig, PeferYamuxTransport: true})
	err := a.Start(ctx)
	require.NoError(t, err)

	s := New()
	err = s.Init(a)
	require.NoError(t, err)
	return &fixture{
		Service:           s,
		eventSender:       eventSender,
		commonFileService: commonFileService,
		fileSyncService:   fileSyncService,
		rpcStore:          rpcStore,
		objectStore:       objectStore,
	}
}

func TestFileAdd(t *testing.T) {
	fx, got, uploaded := getFixtureAndFileInfo(t)
	ctx := context.Background()

	require.Len(t, got.Variants, 1)

	var variantCid string

	t.Run("expect decrypting content", func(t *testing.T) {
		assertFileMeta(t, got, got.Variants)

		variant := got.Variants[0]

		variantCid = variant.Hash
		reader, err := fx.GetContentReader(ctx, spaceId, variantCid, got.EncryptionKeys.EncryptionKeys[variant.Path])
		require.NoError(t, err)

		gotContent, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, testFileContent, string(gotContent))

	})

	t.Run("expect that encrypted content stored in DAG", func(t *testing.T) {
		contentCid := cid.MustParse(variantCid)
		encryptedContent, err := fx.commonFileService.GetFile(ctx, contentCid)
		require.NoError(t, err)
		gotEncryptedContent, err := io.ReadAll(encryptedContent)
		require.NoError(t, err)
		assert.NotEqual(t, testFileContent, string(gotEncryptedContent))
	})

	t.Run("check that file is uploaded to backup node", func(t *testing.T) {
		req := filesync.AddFileRequest{
			FileObjectId:        "objectId1",
			FileId:              domain.FullFileId{SpaceId: spaceId, FileId: got.FileId},
			UploadedByUser:      true,
			Imported:            false,
			PrioritizeVariantId: "",
			Score:               0,
		}
		err := fx.fileSyncService.AddFile(req)
		require.NoError(t, err)
		<-uploaded
		infos, err := fx.rpcStore.FilesInfo(ctx, spaceId, got.FileId)
		require.NoError(t, err)

		require.Len(t, infos, 1)

		assert.Equal(t, got.FileId.String(), infos[0].FileId)
	})
}

func getFixtureAndFileInfo(t *testing.T) (*fixture, *AddResult, chan struct{}) {
	fx := newFixture(t)
	ctx := context.Background()

	uploaded := make(chan struct{})
	fx.fileSyncService.OnUploaded(func(objectId string, fileId domain.FullFileId) error {
		close(uploaded)
		return nil
	})

	lastModifiedDate := time.Now()
	buf := strings.NewReader(testFileContent)
	fx.eventSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

	opts := []AddOption{
		WithName(testFileName),
		WithLastModifiedDate(lastModifiedDate.Unix()),
		WithReader(buf),
	}
	got, err := fx.FileAdd(ctx, spaceId, opts...)
	require.NoError(t, err)
	assert.NotEmpty(t, got.FileId)
	got.Commit()
	return fx, got, uploaded
}

func TestIndexFile(t *testing.T) {
	t.Run("with encryption keys available", func(t *testing.T) {
		fx := newFixture(t)

		fileResult := testAddFile(t, fx)

		// Index
		file, err := fx.GetFileVariants(context.Background(), domain.FullFileId{FileId: fileResult.FileId, SpaceId: spaceId}, fileResult.EncryptionKeys.EncryptionKeys)
		require.NoError(t, err)

		assertFileMeta(t, fileResult, file)
	})

	t.Run("with encryption keys not available", func(t *testing.T) {
		fx := newFixture(t)

		fileResult := testAddFile(t, fx)

		_, err := fx.GetFileVariants(context.Background(), domain.FullFileId{FileId: fileResult.FileId, SpaceId: spaceId}, nil)
		require.Error(t, err)
	})
}

func assertFileMeta(t *testing.T, fileResult *AddResult, variants []*storage.FileInfo) {
	for _, v := range variants {
		assert.Equal(t, fileResult.MIME, v.Media)
		assert.Equal(t, testFileName, v.Name)
		assert.Equal(t, int64(len(testFileContent)), v.Size_)
		now := time.Now()
		if !assert.True(t, now.Sub(time.Unix(v.LastModifiedDate, 0)) < time.Second) {
			fmt.Println(now)
			fmt.Println(time.Unix(v.LastModifiedDate, 0))
		}
		assert.True(t, now.Sub(time.Unix(v.Added, 0)) < time.Second)
	}
}

func TestFileAddWithCustomKeys(t *testing.T) {
	t.Run("with valid keys expect use them", func(t *testing.T) {
		fx := newFixture(t)
		ctx := context.Background()

		uploaded := make(chan struct{})
		fx.fileSyncService.OnUploaded(func(objectId string, fileId domain.FullFileId) error {
			close(uploaded)
			return nil
		})

		lastModifiedDate := time.Now()
		buf := strings.NewReader(testFileContent)
		fx.eventSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

		customKeys := map[string]string{
			encryptionKeyPath(schema.LinkFile): "bweokjjonr756czpdoymdfwzromqtqb27z44tmcb2vv322y2v62ja",
		}

		opts := []AddOption{
			WithName(testFileName),
			WithLastModifiedDate(lastModifiedDate.Unix()),
			WithReader(buf),
			WithCustomEncryptionKeys(customKeys),
		}
		got, err := fx.FileAdd(ctx, spaceId, opts...)
		require.NoError(t, err)
		assert.NotEmpty(t, got.FileId)
		got.Commit()

		assertCustomEncryptionKeys(t, fx, got, customKeys)
	})

	t.Run("with invalid keys expect generate new ones", func(t *testing.T) {
		for i, customKeys := range []map[string]string{
			nil,
			{"invalid": "key"},
			{encryptionKeyPath(schema.LinkFile): "not-an-aes-key"},
		} {
			t.Run(fmt.Sprintf("case %d", i+1), func(t *testing.T) {
				fx := newFixture(t)
				ctx := context.Background()

				uploaded := make(chan struct{})
				fx.fileSyncService.OnUploaded(func(objectId string, fileId domain.FullFileId) error {
					close(uploaded)
					return nil
				})

				lastModifiedDate := time.Now()
				buf := strings.NewReader(testFileContent)
				fx.eventSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

				opts := []AddOption{
					WithName(testFileName),
					WithLastModifiedDate(lastModifiedDate.Unix()),
					WithReader(buf),
					WithCustomEncryptionKeys(customKeys),
				}
				got, err := fx.FileAdd(ctx, spaceId, opts...)
				require.NoError(t, err)
				assert.NotEmpty(t, got.FileId)
				got.Commit()

				encKeys, err := fx.objectStore.GetFileKeys(got.FileId)
				require.NoError(t, err)
				assert.NotEmpty(t, encKeys)
				assert.NotEqual(t, customKeys, encKeys)
			})
		}
	})
}

func TestAddFilesConcurrently(t *testing.T) {
	testAddConcurrently(t, func(t *testing.T, fx *fixture) *AddResult {
		return testAddFile(t, fx)
	})
}

func testAddConcurrently(t *testing.T, addFunc func(t *testing.T, fx *fixture) *AddResult) {
	fx := newFixture(t)

	const numTimes = 5
	gotCh := make(chan *AddResult, numTimes)

	for i := 0; i < numTimes; i++ {
		go func() {
			got := addFunc(t, fx)
			gotCh <- got
		}()
	}

	var prev *AddResult
	for i := 0; i < numTimes; i++ {
		got := <-gotCh

		if prev == nil {
			// The first file should be new
			assert.False(t, got.IsExisting)
			prev = got
		} else {
			assert.Equal(t, prev.FileId, got.FileId)
			assert.Equal(t, prev.EncryptionKeys, got.EncryptionKeys)
			assert.Equal(t, prev.MIME, got.MIME)
			assert.Equal(t, prev.Size, got.Size)
			assert.True(t, got.IsExisting)
		}
	}
}

func testAddFile(t *testing.T, fx *fixture) *AddResult {
	lastModifiedDate := time.Now()
	buf := strings.NewReader(testFileContent)
	opts := []AddOption{
		WithName(testFileName),
		WithLastModifiedDate(lastModifiedDate.Unix()),
		WithReader(buf),
	}
	got, err := fx.FileAdd(context.Background(), spaceId, opts...)
	require.NoError(t, err)

	fx.addFileObjectToStore(t, got)

	got.Commit()

	return got
}

func (fx *fixture) addFileObjectToStore(t *testing.T, got *AddResult) {
	fullFileId := domain.FullFileId{
		SpaceId: spaceId,
		FileId:  got.FileId,
	}

	file, err := NewFile(fx.Service, fullFileId, got.Variants)
	require.NoError(t, err)

	objectId := bson.NewObjectId().Hex()
	st := state.NewDoc(objectId, nil).(*state.State)
	st.SetFileInfo(state.FileInfo{
		FileId:         got.FileId,
		EncryptionKeys: got.EncryptionKeys.EncryptionKeys,
	})
	details, _, err := file.Details(context.Background())
	require.NoError(t, err)

	st.SetDetails(details)
	st.SetDetail(bundle.RelationKeyFileId, domain.String(got.FileId))
	err = filemodels.InjectVariantsToDetails(got.Variants, st)
	require.NoError(t, err)

	err = fx.objectStore.SpaceIndex(spaceId).UpdateObjectDetails(context.Background(), objectId, st.CombinedDetails())
	require.NoError(t, err)
}
