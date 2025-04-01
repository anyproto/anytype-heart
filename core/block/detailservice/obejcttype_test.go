package detailservice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestService_ObjectTypeAddRelations(t *testing.T) {
	t.Run("add recommended relations", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New(bundle.TypeKeyTask.URL())
		sb.SetSpace(fx.space)
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, objectId string) (smartblock.SmartBlock, error) {
			assert.Equal(t, bundle.TypeKeyTask.URL(), objectId)
			return sb, nil
		})
		fx.space.EXPECT().GetRelationIdByKey(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, key domain.RelationKey) (string, error) {
			return key.URL(), nil
		})

		// when
		err := fx.ObjectTypeAddRelations(nil, bundle.TypeKeyTask.URL(), []domain.RelationKey{
			bundle.RelationKeyAssignee, bundle.RelationKeyDone,
		})

		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{bundle.RelationKeyAssignee.URL(), bundle.RelationKeyDone.URL()},
			sb.Details().GetStringList(bundle.RelationKeyRecommendedRelations))
	})

	t.Run("editing of bundled types is prohibited", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		err := fx.ObjectTypeAddRelations(nil, bundle.TypeKeyTask.BundledURL(), []domain.RelationKey{
			bundle.RelationKeyAssignee, bundle.RelationKeyDone,
		})

		// then
		assert.Error(t, err)
		assert.ErrorIs(t, ErrBundledTypeIsReadonly, err)
	})
}

func TestService_ObjectTypeRemoveRelations(t *testing.T) {
	t.Run("remove recommended relations", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New(bundle.TypeKeyTask.URL())
		sb.SetSpace(fx.space)
		sb.Doc.(*state.State).SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{
				bundle.RelationKeyAssignee.URL(),
				bundle.RelationKeyIsFavorite.URL(),
				bundle.RelationKeyDone.URL(),
				bundle.RelationKeyLinkedProjects.URL(),
			}),
		}))
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, objectId string) (smartblock.SmartBlock, error) {
			assert.Equal(t, bundle.TypeKeyTask.URL(), objectId)
			return sb, nil
		})
		fx.space.EXPECT().GetRelationIdByKey(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, key domain.RelationKey) (string, error) {
			return key.URL(), nil
		})

		// when
		err := fx.ObjectTypeRemoveRelations(nil, bundle.TypeKeyTask.URL(), []domain.RelationKey{
			bundle.RelationKeyAssignee, bundle.RelationKeyDone,
		})

		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{bundle.RelationKeyIsFavorite.URL(), bundle.RelationKeyLinkedProjects.URL()},
			sb.Details().GetStringList(bundle.RelationKeyRecommendedRelations))
	})

	t.Run("editing of bundled types is prohibited", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		err := fx.ObjectTypeRemoveRelations(nil, bundle.TypeKeyTask.BundledURL(), []domain.RelationKey{
			bundle.RelationKeyAssignee, bundle.RelationKeyDone,
		})

		// then
		assert.Error(t, err)
		assert.ErrorIs(t, ErrBundledTypeIsReadonly, err)
	})
}

func TestService_objectTypeSetRelations(t *testing.T) {
	t.Run("set recommended relations to type", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New(bundle.TypeKeyTask.URL())
		sb.Doc.(*state.State).SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{
				bundle.RelationKeyAssignee.URL(),
				bundle.RelationKeyIsFavorite.URL(),
				bundle.RelationKeyLinkedProjects.URL(),
			}),
		}))
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, objectId string) (smartblock.SmartBlock, error) {
			assert.Equal(t, bundle.TypeKeyTask.URL(), objectId)
			return sb, nil
		})

		// when
		err := fx.ObjectTypeSetRelations(bundle.TypeKeyTask.URL(), []string{
			bundle.RelationKeyAssignee.URL(), bundle.RelationKeyDone.URL(),
		})

		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{bundle.RelationKeyAssignee.URL(), bundle.RelationKeyDone.URL()},
			sb.Details().GetStringList(bundle.RelationKeyRecommendedRelations))
	})

	t.Run("setting recommended relations to bundled type is prohibited", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		err := fx.ObjectTypeSetRelations(bundle.TypeKeyTask.BundledURL(), []string{
			bundle.RelationKeyAssignee.URL(), bundle.RelationKeyDone.URL(),
		})

		// then
		assert.Error(t, err)
		assert.ErrorIs(t, ErrBundledTypeIsReadonly, err)
	})

	t.Run("set recommended featured relations to type", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New(bundle.TypeKeyTask.URL())
		sb.Doc.(*state.State).SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{
				bundle.RelationKeyAssignee.URL(),
				bundle.RelationKeyIsFavorite.URL(),
				bundle.RelationKeyLinkedProjects.URL(),
			}),
		}))
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, objectId string) (smartblock.SmartBlock, error) {
			assert.Equal(t, bundle.TypeKeyTask.URL(), objectId)
			return sb, nil
		})

		// when
		err := fx.ObjectTypeSetFeaturedRelations(bundle.TypeKeyTask.URL(), []string{
			bundle.RelationKeyAssignee.URL(), bundle.RelationKeyDone.URL(),
		})

		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{bundle.RelationKeyAssignee.URL(), bundle.RelationKeyDone.URL()},
			sb.Details().GetStringList(bundle.RelationKeyRecommendedFeaturedRelations))
	})

	t.Run("setting recommended featured relations to bundled type is prohibited", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		err := fx.ObjectTypeSetFeaturedRelations(bundle.TypeKeyTask.BundledURL(), []string{
			bundle.RelationKeyAssignee.URL(), bundle.RelationKeyDone.URL(),
		})

		// then
		assert.Error(t, err)
		assert.ErrorIs(t, ErrBundledTypeIsReadonly, err)
	})
}

func TestService_ObjectTypeListConflictingRelations(t *testing.T) {
	t.Run("list conflicting relations", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			// object type
			{
				bundle.RelationKeyId: domain.String(bundle.TypeKeyTask.URL()),
				bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{
					bundle.RelationKeyType.URL(),
					bundle.RelationKeyName.URL(),
				}),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{
					bundle.RelationKeyAssignee.URL(),
					bundle.RelationKeyDone.URL(),
				}),
				bundle.RelationKeyRecommendedHiddenRelations: domain.StringList([]string{
					bundle.RelationKeyCreatedDate.URL(),
				}),
			},
			// objects
			{
				bundle.RelationKeyId:       domain.String("task1"), // 1
				bundle.RelationKeyType:     domain.String(bundle.TypeKeyTask.URL()),
				bundle.RelationKeyName:     domain.String("Invent alphabet"),
				bundle.RelationKeyAssignee: domain.StringList([]string{"Kirill", "Methodius"}),
				bundle.RelationKeyDueDate:  domain.Int64(863), // 2
			},
			{
				bundle.RelationKeyId:       domain.String("task2"),
				bundle.RelationKeyType:     domain.String(bundle.TypeKeyTask.URL()),
				bundle.RelationKeyName:     domain.String("Fight CO2 pollution"),
				bundle.RelationKeyAssignee: domain.StringList([]string{"Humanity"}),
				bundle.RelationKeyDone:     domain.Bool(false),
				bundle.RelationKeyStatus:   domain.String("In Progress"), // 3
			},
			// relations
			generateRelationTestObject(bundle.RelationKeyId),
			generateRelationTestObject(bundle.RelationKeyType),
			generateRelationTestObject(bundle.RelationKeyName),
			generateRelationTestObject(bundle.RelationKeyAssignee),
			generateRelationTestObject(bundle.RelationKeyDueDate),
			generateRelationTestObject(bundle.RelationKeyDone),
			generateRelationTestObject(bundle.RelationKeyStatus),
		})

		// when
		relations, err := fx.ObjectTypeListConflictingRelations(spaceId, bundle.TypeKeyTask.URL())

		// then
		assert.NoError(t, err)
		assert.Len(t, relations, 3)
	})
}

func generateRelationTestObject(key domain.RelationKey) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:          domain.String(key.URL()),
		bundle.RelationKeyRelationKey: domain.String(key.String()),
		bundle.RelationKeyLayout:      domain.Int64(model.ObjectType_relation),
	}
}
