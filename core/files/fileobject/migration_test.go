package fileobject

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	ipld "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	bb "github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

func TestMigrateFiles(t *testing.T) {
	t.Run("file not migrated yet", func(t *testing.T) {
		fx := newFixture(t)
		const objectId = "objectId"
		fx.objectCreator.objectId = objectId

		// Just use dummy for state
		st := testutil.BuildStateFromAST(
			bb.Root(bb.ID("root")),
		)
		space := mock_clientspace.NewMockSpace(t)
		spaceId := "spaceId"
		space.EXPECT().IsPersonal().Return(true)
		space.EXPECT().Id().Return(spaceId)
		space.EXPECT().DeriveObjectIdWithAccountSignature(mock.Anything, mock.Anything).Return(objectId, nil)
		space.EXPECT().GetObject(mock.Anything, objectId).Return(nil, fmt.Errorf("not found"))

		// Called in metadata indexer
		space.EXPECT().Do(objectId, mock.Anything).Return(nil)

		fx.spaceService.EXPECT().Get(mock.Anything, "spaceId").Return(space, nil)

		testFileId, _ := fx.givenFileAddedToDAG(t)

		wantKeys := map[string]string{
			"/0": "encryptionKey2",
		}
		filesKeys := []*pb.ChangeFileKeys{
			{
				// This should be ignored. We migrate only old files (not objects)
				Hash: testFileObjectId,
				Keys: map[string]string{
					"/0": "encryptionKey1",
				},
			},
			{
				Hash: testFileId.String(),
				Keys: wantKeys,
			},
		}
		fx.MigrateFiles(st, space, filesKeys)

		fx.waitFileMigrationHandled(t)

		wantFileInfo := state.FileInfo{
			FileId:         testFileId,
			EncryptionKeys: wantKeys,
		}
		assert.Equal(t, wantFileInfo, fx.objectCreator.creationState.GetFileInfo())
	})

	t.Run("file object is already created", func(t *testing.T) {
		fx := newFixture(t)
		const objectId = "objectId"
		fx.objectCreator.objectId = objectId

		// Just use dummy for state
		st := testutil.BuildStateFromAST(
			bb.Root(bb.ID("root")),
		)
		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().IsPersonal().Return(true)
		space.EXPECT().Id().Return("spaceId")
		space.EXPECT().DeriveObjectIdWithAccountSignature(mock.Anything, mock.Anything).Return(objectId, nil)

		// Object already exists in space
		space.EXPECT().GetObject(mock.Anything, objectId).Return(nil, nil)

		fx.spaceService.EXPECT().Get(mock.Anything, "spaceId").Return(space, nil)

		filesKeys := []*pb.ChangeFileKeys{
			{
				// This should be ignored. We migrate only old files (not objects)
				Hash: testFileObjectId,
				Keys: map[string]string{
					"/0": "encryptionKey1",
				},
			},
			{
				Hash: testFileId.String(),
				Keys: map[string]string{
					"/0": "encryptionKey2",
				},
			},
		}
		fx.MigrateFiles(st, space, filesKeys)

		fx.waitFileMigrationHandled(t)

		assert.Nil(t, fx.objectCreator.creationState)
	})
}

func (fx *fixture) waitFileMigrationHandled(t *testing.T) {
	timeout := time.NewTimer(100 * time.Millisecond)
	for {
		select {
		case <-timeout.C:
			t.Fatal("timeout")
		case <-time.After(10 * time.Millisecond):
			if fx.migrationQueue.NumProcessedItems() > 0 {
				return
			}
		}
	}
}

func (fx *fixture) givenFileAddedToDAG(t *testing.T) (domain.FileId, ipld.Node) {
	buf := make([]byte, 1024*1024)
	_, err := rand.Read(buf)
	require.NoError(t, err)
	fileNode, err := fx.commonFileService.AddFile(context.Background(), bytes.NewReader(buf))
	require.NoError(t, err)
	return domain.FileId(fileNode.Cid().String()), fileNode
}

func TestMigrateIds(t *testing.T) {
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
		space.EXPECT().IsPersonal().Return(true)

		fx.MigrateFileIdsInBlocks(st, space)
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
		space.EXPECT().IsPersonal().Return(true)

		fx.MigrateFileIdsInBlocks(st, space)
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
		st.SetDetailAndBundledRelation(bundle.RelationKeyIconImage, domain.StringList([]string{fileId.String()}))

		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().IsPersonal().Return(true)

		fx.objectStore.AddObjects(t, "spaceId2", []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String(fileId.String()),
				bundle.RelationKeyFileId:  domain.String("fileId"),
				bundle.RelationKeySpaceId: domain.String("spaceId2"),
			},
		})

		fx.MigrateFileIdsInBlocks(st, space)
		fx.MigrateFileIdsInDetails(st, space)

		wantState := testutil.BuildStateFromAST(
			bb.Root(
				bb.ID(objectId),
			),
		)
		wantState.SetDetailAndBundledRelation(bundle.RelationKeyIconImage, domain.StringList([]string{fileId.String()}))

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

		// Relation format: file
		st.SetDetailAndBundledRelation(bundle.RelationKeyIconImage, domain.StringList([]string{fileId.String()}))
		// Relation format: object
		st.SetDetailAndBundledRelation(bundle.RelationKeyAssignee, domain.StringList([]string{fileId.String()}))

		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().IsPersonal().Return(true)
		space.EXPECT().DeriveObjectIdWithAccountSignature(mock.Anything, mock.Anything).Return(expectedFileObjectId, nil)

		fx.MigrateFileIdsInBlocks(st, space)
		fx.MigrateFileIdsInDetails(st, space)

		wantState := testutil.BuildStateFromAST(
			bb.Root(
				bb.ID(objectId),
				bb.Children(
					bb.File(expectedFileObjectId, bb.FileHash(fileId.String())),
					bb.Text("sample text", bb.TextIconImage(expectedFileObjectId)),
				),
			),
		)
		wantState.SetDetailAndBundledRelation(bundle.RelationKeyIconImage, domain.StringList([]string{expectedFileObjectId}))
		wantState.SetDetailAndBundledRelation(bundle.RelationKeyAssignee, domain.StringList([]string{expectedFileObjectId}))

		bb.AssertTreesEqual(t, wantState.Blocks(), st.Blocks())
		assert.Equal(t, wantState.Details(), st.Details())
	})
}
