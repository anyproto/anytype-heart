package indexer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

func TestIndexer(t *testing.T) {
	space := mock_clientspace.NewMockSpace(t)
	space.EXPECT().Id().Return("spaceId1").Maybe()

	for _, testCase := range []struct {
		name    string
		options smartblock.IndexOption
	}{
		{
			name:    "SkipFullTextIfHeadsNotChanged",
			options: smartblock.SkipFullTextIfHeadsNotChanged,
		},
		{
			name:    "SkipIfHeadsNotChanged",
			options: smartblock.SkipIfHeadsNotChanged,
		},
	} {
		t.Run("index has not started - when hashes match and "+testCase.name, func(t *testing.T) {
			// given
			indexerFx := NewIndexerFixture(t)
			smartTest := smarttest.New("objectId1")
			smartTest.SetSpaceId("spaceId1")
			smartTest.SetSpace(space)
			smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.Text(
						"to index",
						blockbuilder.ID("blockId1"),
					),
				)))

			smartTest.SetType(coresb.SmartBlockTypePage)
			indexerFx.store.SpaceIndex("spaceId1").SaveLastIndexedHeadsHash(ctx, "objectId1", "7f40bc2814f5297818461f889780a870ea033fe64c5a261117f2b662515a3dba")

			// when
			err := indexerFx.Index(smartTest.GetDocInfo(), testCase.options)

			// then
			assert.NoError(t, err)
			count, _ := indexerFx.store.ListIdsFromFullTextQueue([]string{"spaceId1"}, 0)
			assert.Equal(t, 0, len(count))
		})

		t.Run("index has started - when hashes don't match and key "+testCase.name, func(t *testing.T) {
			// given
			indexerFx := NewIndexerFixture(t)
			smartTest := smarttest.New("objectId1")
			smartTest.SetSpaceId("spaceId1")
			smartTest.SetSpace(space)
			smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.Text(
						"to index",
						blockbuilder.ID("blockId1"),
					),
				)))

			smartTest.SetType(coresb.SmartBlockTypePage)
			indexerFx.store.SpaceIndex("spaceId1").SaveLastIndexedHeadsHash(ctx, "objectId1", "randomHash")

			// when
			err := indexerFx.Index(smartTest.GetDocInfo(), testCase.options)

			// then
			assert.NoError(t, err)
			count, _ := indexerFx.store.ListIdsFromFullTextQueue([]string{"spaceId1"}, 0)
			assert.Equal(t, 1, len(count))
		})
	}

	t.Run("index has started - when hashes match and options are not provided", func(t *testing.T) {
		// given
		indexerFx := NewIndexerFixture(t)
		smartTest := smarttest.New("objectId1")
		smartTest.SetSpaceId("spaceId1")
		smartTest.SetSpace(space)
		smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"to index",
					blockbuilder.ID("blockId1"),
				),
			)))

		smartTest.SetType(coresb.SmartBlockTypePage)
		indexerFx.store.SpaceIndex("spaceId1").SaveLastIndexedHeadsHash(ctx, "objectId1", "7f40bc2814f5297818461f889780a870ea033fe64c5a261117f2b662515a3dba")

		// when
		err := indexerFx.Index(smartTest.GetDocInfo())

		// then
		assert.NoError(t, err)
		count, _ := indexerFx.store.ListIdsFromFullTextQueue([]string{"spaceId1"}, 0)
		assert.Equal(t, 1, len(count))
	})
}
