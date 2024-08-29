package systemobjectreviser

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	mock_space "github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

func TestReviseSystemObject(t *testing.T) {
	ctx := context.Background()
	log := logger.NewNamed("tesr")
	marketObjects := map[string]*domain.Details{
		"_otnote":        domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{revisionKey: domain.Int64(3)}),
		"_otpage":        domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{revisionKey: domain.Int64(2)}),
		"_otcontact":     domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{revisionKey: domain.Int64(1)}),
		"_brid":          domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{revisionKey: domain.Int64(1)}),
		"_brdescription": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{revisionKey: domain.Int64(2)}),
		"_brlyrics":      domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{revisionKey: domain.Int64(1)}),
		"_brisReadonly":  domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{revisionKey: domain.Int64(3)}),
	}

	t.Run("system object type is updated if revision is higher", func(t *testing.T) {
		// given
		objectType := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRevision:     domain.Int64(1),
			bundle.RelationKeySourceObject: domain.String("_otnote"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-note"),
		})
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
		objectType := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeySourceObject: domain.String("_otpage"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-page"),
		})
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
		objectType := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyUniqueKey: domain.String("ot-kitty"),
		})
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseSystemObject(ctx, log, space, objectType, marketObjects)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("non system object type is not updated", func(t *testing.T) {
		// given
		objectType := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeySourceObject: domain.String("_otcontact"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-contact"),
		})
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseSystemObject(ctx, log, space, objectType, marketObjects)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("system object type with same revision is not updated", func(t *testing.T) {
		// given
		objectType := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRevision:     domain.Int64(3),
			bundle.RelationKeySourceObject: domain.String("_otnote"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-note"),
		})
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseSystemObject(ctx, log, space, objectType, marketObjects)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("system relation is updated if revision is higher", func(t *testing.T) {
		// given
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRevision:     domain.Int64(1),
			bundle.RelationKeySourceObject: domain.String("_brdescription"),
			bundle.RelationKeyUniqueKey:    domain.String("rel-description"),
		})
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
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeySourceObject: domain.String("_brid"),
			bundle.RelationKeyUniqueKey:    domain.String("rel-id"),
		})
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
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyUniqueKey: domain.String("rel-custom"),
		})
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseSystemObject(ctx, log, space, rel, marketObjects)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("non system relation is not updated", func(t *testing.T) {
		// given
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRevision:     domain.Int64(1),
			bundle.RelationKeySourceObject: domain.String("_brlyrics"),
			bundle.RelationKeyUniqueKey:    domain.String("rel-lyrics"),
		})
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseSystemObject(ctx, log, space, rel, marketObjects)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("system relation with same revision is not updated", func(t *testing.T) {
		// given
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRevision:     domain.Int64(3),
			bundle.RelationKeySourceObject: domain.String("_brisReadonly"),
			bundle.RelationKeyUniqueKey:    domain.String("rel-isReadonly"),
		})
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseSystemObject(ctx, log, space, rel, marketObjects)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("relation with absent maxCount is updated", func(t *testing.T) {
		// given
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRevision:         domain.Int64(2),
			bundle.RelationKeySourceObject:     domain.String("_brisReadonly"),
			bundle.RelationKeyUniqueKey:        domain.String("rel-isReadonly"),
			bundle.RelationKeyRelationMaxCount: domain.Int64(1),
		})
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")

		// when
		toRevise, err := reviseSystemObject(ctx, log, space, rel, marketObjects)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})
}
