package systemobjectreviser

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	mock_space "github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestMigration_Run(t *testing.T) {
	t.Run("migrate relations with different revisions", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, "space1", []objectstore.TestObject{
			{
				bundle.RelationKeySpaceId:        pbtypes.String("space1"),
				bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_object)),
				bundle.RelationKeyLayout:         pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyId:             pbtypes.String("id1"),
				bundle.RelationKeyIsHidden:       pbtypes.Bool(true), // bundle = false
				bundle.RelationKeyRevision:       pbtypes.Int64(1),   // bundle = 3
				bundle.RelationKeyUniqueKey:      pbtypes.String(bundle.RelationKeyBacklinks.URL()),
				bundle.RelationKeySourceObject:   pbtypes.String(bundle.RelationKeyBacklinks.BundledURL()),
			},
		})
		fixer := &Migration{}
		ctx := context.Background()
		log := logger.NewNamed("test")

		spc := mock_space.NewMockSpace(t)
		spc.EXPECT().Id().Return("space1").Maybe()

		spc.EXPECT().DoCtx(ctx, "id1", mock.Anything).Return(nil).Times(1)

		// when
		migrated, toMigrate, err := fixer.Run(ctx, log, store.SpaceIndex("space1"), spc)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 1, migrated)
		assert.Equal(t, 1, toMigrate)
	})
}

func TestReviseSystemObject(t *testing.T) {
	ctx := context.Background()
	log := logger.NewNamed("test")
	marketObjects := map[string]*types.Struct{
		"_otnote":        {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(3)}},
		"_otpage":        {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(2)}},
		"_otcontact":     {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(1)}},
		"_brid":          {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(1)}},
		"_brdescription": {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(2)}},
		"_brlyrics":      {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(1)}},
		"_brisReadonly":  {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(3)}},
	}

	t.Run("system object type is updated if revision is higher", func(t *testing.T) {
		// given
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():     pbtypes.Int64(bundle.MustGetType(bundle.TypeKeyFile).Revision - 1),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_otfile"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-file"),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")
		space.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return addr.ObjectTypeKeyToIdPrefix + key.InternalKey(), nil
		}).Maybe()

		// when
		toRevise, err := reviseObject(ctx, log, space, objectType)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("system object type is updated if no revision is set", func(t *testing.T) {
		// given
		objectType := &types.Struct{Fields: map[string]*types.Value{ // bundle Audio type revision = 1
			bundle.RelationKeySourceObject.String(): pbtypes.String("_otaudio"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-audio"),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")
		space.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return addr.ObjectTypeKeyToIdPrefix + key.InternalKey(), nil
		}).Maybe()

		// when
		toRevise, err := reviseObject(ctx, log, space, objectType)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("custom object type is not updated", func(t *testing.T) {
		// given
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyUniqueKey.String(): pbtypes.String("ot-kitty"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseObject(ctx, log, space, objectType)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("non system object type is not updated", func(t *testing.T) {
		// given
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeySourceObject.String(): pbtypes.String("_otcontact"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-contact"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseObject(ctx, log, space, objectType)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("system object type with same revision is not updated", func(t *testing.T) {
		// given
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():     pbtypes.Int64(bundle.MustGetType(bundle.TypeKeyImage).Revision),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_otimage"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-image"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseObject(ctx, log, space, objectType)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("system relation is updated if revision is higher", func(t *testing.T) {
		// given
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():     pbtypes.Int64(bundle.MustGetRelation(bundle.RelationKeyGlobalName).Revision - 1),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_brglobalName"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("rel-globalName"),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("system relation is updated if no revision is set", func(t *testing.T) {
		// given
		rel := &types.Struct{Fields: map[string]*types.Value{ // relationMaxCount revision = 1
			bundle.RelationKeySourceObject.String(): pbtypes.String("_brrelationMaxCount"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("rel-relationMaxCount"),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("custom relation is not updated", func(t *testing.T) {
		// given
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyUniqueKey.String(): pbtypes.String("rel-custom"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("non system relation is not updated", func(t *testing.T) {
		// given
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():     pbtypes.Int64(1),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_brlyrics"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("rel-lyrics"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("system relation with same revision is not updated", func(t *testing.T) {
		// given
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():     pbtypes.Int64(bundle.MustGetRelation(bundle.RelationKeyBacklinks).Revision),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_brbacklinks"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("rel-backlinks"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("relation with absent maxCount is updated", func(t *testing.T) {
		// given
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():         pbtypes.Int64(bundle.MustGetRelation(bundle.RelationKeyBacklinks).Revision - 1),
			bundle.RelationKeySourceObject.String():     pbtypes.String("_brbacklinks"),
			bundle.RelationKeyUniqueKey.String():        pbtypes.String("rel-backlinks"),
			bundle.RelationKeyRelationMaxCount.String(): pbtypes.Int64(1), // maxCount of bundle backlinks = 0
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("recommendedRelations list is updated", func(t *testing.T) {
		// given
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():             pbtypes.Int64(bundle.MustGetType(bundle.TypeKeyImage).Revision - 1),
			bundle.RelationKeySourceObject.String():         pbtypes.String("_otimage"),
			bundle.RelationKeyUniqueKey.String():            pbtypes.String("ot-image"),
			bundle.RelationKeyRecommendedRelations.String(): pbtypes.StringList([]string{"rel-name"}),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")
		space.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return addr.ObjectTypeKeyToIdPrefix + key.InternalKey(), nil
		}).Maybe()

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("recommendedRelations list is updated by not system relations", func(t *testing.T) {
		// given
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():             pbtypes.Int64(2),
			bundle.RelationKeySourceObject.String():         pbtypes.String("_otpage"),
			bundle.RelationKeyUniqueKey.String():            pbtypes.String("ot-page"),
			bundle.RelationKeyRecommendedRelations.String(): pbtypes.StringList([]string{"rel-name"}),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return addr.ObjectTypeKeyToIdPrefix + key.InternalKey(), nil
		}).Maybe()

		// when
		marketObjects["_otpage"].Fields["recommendedRelations"] = pbtypes.StringList([]string{"_brname", "_brtag"})
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})
}
