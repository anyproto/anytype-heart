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
	mock_space "github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestReviseSystemObject(t *testing.T) {
	ctx := context.Background()
	log := logger.NewNamed("tesr")
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
			bundle.RelationKeyRevision.String():     pbtypes.Int64(1),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_otnote"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-note"),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")

		// when
		toRevise, err := reviseSystemObject(ctx, log, space, objectType, marketObjects)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("system object type is updated if no revision is set", func(t *testing.T) {
		// given
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeySourceObject.String(): pbtypes.String("_otpage"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-page"),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")

		// when
		toRevise, err := reviseSystemObject(ctx, log, space, objectType, marketObjects)

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
		toRevise, err := reviseSystemObject(ctx, log, space, objectType, marketObjects)

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
		toRevise, err := reviseSystemObject(ctx, log, space, objectType, marketObjects)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("system object type with same revision is not updated", func(t *testing.T) {
		// given
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():     pbtypes.Int64(3),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_otnote"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-note"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseSystemObject(ctx, log, space, objectType, marketObjects)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("system relation is updated if revision is higher", func(t *testing.T) {
		// given
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():     pbtypes.Int64(1),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_brdescription"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("rel-description"),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")

		// when
		toRevise, err := reviseSystemObject(ctx, log, space, rel, marketObjects)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("system relation is updated if no revision is set", func(t *testing.T) {
		// given
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeySourceObject.String(): pbtypes.String("_brid"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("rel-id"),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")

		// when
		toRevise, err := reviseSystemObject(ctx, log, space, rel, marketObjects)

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
		toRevise, err := reviseSystemObject(ctx, log, space, rel, marketObjects)

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
		toRevise, err := reviseSystemObject(ctx, log, space, rel, marketObjects)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("system relation with same revision is not updated", func(t *testing.T) {
		// given
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():     pbtypes.Int64(3),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_brisReadonly"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("rel-isReadonly"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseSystemObject(ctx, log, space, rel, marketObjects)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("relation with absent maxCount is updated", func(t *testing.T) {
		// given
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():         pbtypes.Int64(2),
			bundle.RelationKeySourceObject.String():     pbtypes.String("_brisReadonly"),
			bundle.RelationKeyUniqueKey.String():        pbtypes.String("rel-isReadonly"),
			bundle.RelationKeyRelationMaxCount.String(): pbtypes.Int64(1),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")

		// when
		toRevise, err := reviseSystemObject(ctx, log, space, rel, marketObjects)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("recommendedRelations list is updated", func(t *testing.T) {
		// given
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():             pbtypes.Int64(1),
			bundle.RelationKeySourceObject.String():         pbtypes.String("_otpage"),
			bundle.RelationKeyUniqueKey.String():            pbtypes.String("ot-page"),
			bundle.RelationKeyRecommendedRelations.String(): pbtypes.StringList([]string{"rel-name"}),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")
		space.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return addr.ObjectTypeKeyToIdPrefix + key.InternalKey(), nil
		}).Maybe()

		// when
		marketObjects["_otpage"].Fields["recommendedRelations"] = pbtypes.StringList([]string{"_brname", "_brorigin"})
		toRevise, err := reviseSystemObject(ctx, log, space, rel, marketObjects)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("recommendedRelations list is not updated", func(t *testing.T) {
		// given
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():             pbtypes.Int64(2),
			bundle.RelationKeySourceObject.String():         pbtypes.String("_otpage"),
			bundle.RelationKeyUniqueKey.String():            pbtypes.String("ot-page"),
			bundle.RelationKeyRecommendedRelations.String(): pbtypes.StringList([]string{"rel-name", "rel-tag"}),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return addr.ObjectTypeKeyToIdPrefix + key.InternalKey(), nil
		}).Maybe()

		// when
		marketObjects["_otpage"].Fields["recommendedRelations"] = pbtypes.StringList([]string{"_brname", "_brtag"})
		toRevise, err := reviseSystemObject(ctx, log, space, rel, marketObjects)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
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
		toRevise, err := reviseSystemObject(ctx, log, space, rel, marketObjects)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})
}
