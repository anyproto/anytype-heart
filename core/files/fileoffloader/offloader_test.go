package fileoffloader

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	Service

	commonFile  fileservice.FileService
	objectStore *objectstore.StoreFixture
}

func newFixture(t *testing.T) *fixture {
	blockStorage := filestorage.NewInMemory()
	commonFileService := fileservice.New()
	objectStore := objectstore.NewStoreFixture(t)
	dataStoreProvider, err := datastore.NewInMemory()
	spaceIdResolver := mock_idresolver.NewMockResolver(t)

	require.NoError(t, err)
	offloader := New()

	ctx := context.Background()
	a := new(app.App)
	a.Register(dataStoreProvider)
	a.Register(blockStorage)
	a.Register(commonFileService)
	a.Register(objectStore)
	a.Register(offloader)
	a.Register(testutil.PrepareMock(ctx, a, spaceIdResolver))

	err = a.Start(ctx)
	require.NoError(t, err)

	return &fixture{
		Service:     offloader,
		commonFile:  commonFileService,
		objectStore: objectStore,
	}
}

func TestOffloadAllFiles(t *testing.T) {
	fx := newFixture(t)

	ctx := context.Background()
	fileNode1, err := fx.commonFile.AddFile(ctx, generateTestFileData(t, 2*1024*1024))
	require.NoError(t, err)

	fileNode2, err := fx.commonFile.AddFile(ctx, generateTestFileData(t, 2*1024*1024))
	require.NoError(t, err)

	fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("fileObjectId1"),
			bundle.RelationKeyFileId:           domain.String(fileNode1.Cid().String()),
			bundle.RelationKeyFileBackupStatus: domain.Int64(int64(filesyncstatus.Synced)),
		},
		{
			bundle.RelationKeyId:               domain.String("fileObjectId2"),
			bundle.RelationKeyFileId:           domain.String(fileNode2.Cid().String()),
			bundle.RelationKeyFileBackupStatus: domain.Int64(int64(filesyncstatus.Limited)),
		},
	})

	err = fx.FilesOffload(ctx, nil, false)
	require.NoError(t, err)

	_, err = fx.commonFile.GetFile(ctx, fileNode1.Cid())
	require.Error(t, err)

	_, err = fx.commonFile.GetFile(ctx, fileNode2.Cid())
	require.NoError(t, err)
}

func TestSpaceOffload(t *testing.T) {
	fx := newFixture(t)

	ctx := context.Background()
	fileNode1, err := fx.commonFile.AddFile(ctx, generateTestFileData(t, 2*1024*1024))
	require.NoError(t, err)

	fileNode2, err := fx.commonFile.AddFile(ctx, generateTestFileData(t, 2*1024*1024))
	require.NoError(t, err)

	fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("fileObjectId1"),
			bundle.RelationKeySpaceId:          domain.String("space1"),
			bundle.RelationKeyFileId:           domain.String(fileNode1.Cid().String()),
			bundle.RelationKeyFileBackupStatus: domain.Int64(int64(filesyncstatus.Synced)),
		},
		{
			bundle.RelationKeyId:               domain.String("fileObjectId2"),
			bundle.RelationKeySpaceId:          domain.String("space1"),
			bundle.RelationKeyFileId:           domain.String(fileNode2.Cid().String()),
			bundle.RelationKeyFileBackupStatus: domain.Int64(int64(filesyncstatus.Synced)),
		},
	})
	fx.objectStore.AddObjects(t, "space2", []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("fileObjectId3"),
			bundle.RelationKeySpaceId:          domain.String("space2"),
			bundle.RelationKeyFileId:           domain.String(fileNode2.Cid().String()),
			bundle.RelationKeyFileBackupStatus: domain.Int64(int64(filesyncstatus.Synced)),
		},
	})

	offloaded, _, err := fx.FileSpaceOffload(ctx, "space1", false)
	require.NoError(t, err)
	assert.True(t, 1 == offloaded)

	_, err = fx.commonFile.GetFile(ctx, fileNode1.Cid())
	require.Error(t, err)

	_, err = fx.commonFile.GetFile(ctx, fileNode2.Cid())
	require.NoError(t, err)
}

func generateTestFileData(t *testing.T, size int) io.Reader {
	buf := make([]byte, size)
	_, err := rand.Read(buf)
	require.NoError(t, err)
	return bytes.NewReader(buf)
}
