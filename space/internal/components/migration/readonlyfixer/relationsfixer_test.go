package readonlyfixer

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	mock_space "github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestFixReadonlyInRelations(t *testing.T) {
	store := objectstore.NewStoreFixture(t)
	store.AddObjects(t, "space1", []objectstore.TestObject{
		// space1
		{
			bundle.RelationKeySpaceId:               pbtypes.String("space1"),
			bundle.RelationKeyRelationFormat:        pbtypes.Int64(int64(model.RelationFormat_status)),
			bundle.RelationKeyId:                    pbtypes.String("rel-tag"),
			bundle.RelationKeyRelationReadonlyValue: pbtypes.Bool(true),
		},
		{
			bundle.RelationKeySpaceId:               pbtypes.String("space1"),
			bundle.RelationKeyRelationFormat:        pbtypes.Int64(int64(model.RelationFormat_tag)),
			bundle.RelationKeyId:                    pbtypes.String("rel-customTag"),
			bundle.RelationKeyRelationReadonlyValue: pbtypes.Bool(true),
		},
	})
	store.AddObjects(t, "space2", []objectstore.TestObject{
		// space2
		{
			bundle.RelationKeySpaceId:               pbtypes.String("space2"),
			bundle.RelationKeyRelationFormat:        pbtypes.Int64(0),
			bundle.RelationKeyId:                    pbtypes.String("rel-id"),
			bundle.RelationKeyRelationReadonlyValue: pbtypes.Bool(true),
		},
		{
			bundle.RelationKeySpaceId:               pbtypes.String("space2"),
			bundle.RelationKeyRelationFormat:        pbtypes.Int64(2),
			bundle.RelationKeyId:                    pbtypes.String("rel-relationFormat"),
			bundle.RelationKeyRelationReadonlyValue: pbtypes.Bool(true),
		},
	})
	store.AddObjects(t, "space3", []objectstore.TestObject{
		// space3
		{
			bundle.RelationKeySpaceId:               pbtypes.String("space3"),
			bundle.RelationKeyRelationFormat:        pbtypes.Int64(int64(model.RelationFormat_tag)),
			bundle.RelationKeyId:                    pbtypes.String("rel-category"),
			bundle.RelationKeyRelationReadonlyValue: pbtypes.Bool(false),
		},
		{
			bundle.RelationKeySpaceId:               pbtypes.String("space3"),
			bundle.RelationKeyRelationFormat:        pbtypes.Int64(int64(model.RelationFormat_status)),
			bundle.RelationKeyId:                    pbtypes.String("rel-genderCustom"),
			bundle.RelationKeyRelationReadonlyValue: pbtypes.Bool(false),
		},
	})
	fixer := &Migration{}
	ctx := context.Background()
	log := logger.NewNamed("test")

	t.Run("fix tag and status relations with readonly=true", func(t *testing.T) {
		// given
		spc := mock_space.NewMockSpace(t)
		spc.EXPECT().Id().Return("space1").Maybe()

		// both relations will be processed
		spc.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, objectId string, apply func(smartblock.SmartBlock) error) error {
				assert.True(t, slices.Contains([]string{"rel-customTag", "rel-tag"}, objectId))
				return nil
			},
		).Times(2)

		// when
		migrated, toMigrate, err := fixer.Run(ctx, log, store.SpaceStore("space1"), spc)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 2, migrated)
		assert.Equal(t, 2, toMigrate)
	})

	t.Run("do not process relations of other formats", func(t *testing.T) {
		// given
		spc := mock_space.NewMockSpace(t)
		spc.EXPECT().Id().Return("space2").Maybe()

		// none of relations will be processed
		// sp.EXPECT().Do(mock.Anything, mock.Anything).Times(1).Return(nil)

		// when
		migrated, toMigrate, err := fixer.Run(ctx, log, store.SpaceStore("space2"), spc)

		// then
		assert.NoError(t, err)
		assert.Zero(t, migrated)
		assert.Zero(t, toMigrate)
	})

	t.Run("do not process relations with readonly=false", func(t *testing.T) {
		// given
		spc := mock_space.NewMockSpace(t)
		spc.EXPECT().Id().Return("space3").Maybe()

		// none of relations will be processed
		// sp.EXPECT().Do(mock.Anything, mock.Anything).Times(1).Return(nil)

		// when
		migrated, toMigrate, err := fixer.Run(ctx, log, store.SpaceStore("space3"), spc)

		// then
		assert.NoError(t, err)
		assert.Zero(t, migrated)
		assert.Zero(t, toMigrate)
	})
}
