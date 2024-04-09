package fileobject

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	bb "github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

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

		fx.MigrateBlocks(st, space)
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

		fx.MigrateBlocks(st, space)
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

		fx.MigrateBlocks(st, space)
		fx.MigrateDetails(st, space)

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
		space.EXPECT().DeriveObjectIdWithAccountSignature(mock.Anything, mock.Anything).Return(expectedFileObjectId, nil)

		fx.MigrateBlocks(st, space)
		fx.MigrateDetails(st, space)

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
