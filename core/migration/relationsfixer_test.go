package migration

import (
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	mock_space "github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestFixReadonlyInRelations(t *testing.T) {
	store := objectstore.NewStoreFixture(t)
	store.AddObjects(t, []objectstore.TestObject{
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
	fixer := &readonlyRelationsFixer{}

	t.Run("fix tag and status relations with readonly=true", func(t *testing.T) {
		spc := mock_space.NewMockSpace(t)
		spc.EXPECT().Id().Return("space1").Times(1)

		// both relations will be processed
		spc.EXPECT().Do(mock.Anything, mock.Anything).Times(2).Return(nil)

		fixer.Run(store, spc)
	})

	t.Run("do not process relations of other formats", func(t *testing.T) {
		spc := mock_space.NewMockSpace(t)
		spc.EXPECT().Id().Return("space2").Times(1)

		// none of relations will be processed
		// sp.EXPECT().Do(mock.Anything, mock.Anything).Times(1).Return(nil)

		fixer.Run(store, spc)
	})

	t.Run("do not process relations with readonly=false", func(t *testing.T) {
		spc := mock_space.NewMockSpace(t)
		spc.EXPECT().Id().Return("space3").Times(1)

		// none of relations will be processed
		// sp.EXPECT().Do(mock.Anything, mock.Anything).Times(1).Return(nil)

		fixer.Run(store, spc)
	})
}
