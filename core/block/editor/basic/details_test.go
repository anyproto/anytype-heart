package basic

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type basicFixture struct {
	sb    *smarttest.SmartTest
	store *spaceindex.StoreFixture
	basic CommonOperations
}

var (
	objectId = "objectId"
	spaceId  = "space1"
)

func newBasicFixture(t *testing.T) *basicFixture {
	sb := smarttest.New(objectId)
	sb.SetDetails(nil, nil, false)
	sb.SetSpaceId(spaceId)

	store := spaceindex.NewStoreFixture(t)

	b := NewBasic(sb, store, converter.NewLayoutConverter(), nil)

	return &basicFixture{
		sb:    sb,
		store: store,
		basic: b,
	}
}

func TestBasic_UpdateDetails(t *testing.T) {
	t.Run("add new details", func(t *testing.T) {
		// given
		f := newBasicFixture(t)
		f.store.AddObjects(t, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("rel-aperture"),
			bundle.RelationKeySpaceId:        domain.String(spaceId),
			bundle.RelationKeyRelationKey:    domain.String("aperture"),
			bundle.RelationKeyUniqueKey:      domain.String("rel-aperture"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_longtext)),
		}, {
			bundle.RelationKeyId:             domain.String("rel-maxCount"),
			bundle.RelationKeySpaceId:        domain.String(spaceId),
			bundle.RelationKeyRelationKey:    domain.String("relationMaxCount"),
			bundle.RelationKeyUniqueKey:      domain.String("rel-relationMaxCount"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_number)),
		}})

		// when
		err := f.basic.UpdateDetails(nil, func(current *domain.Details) (*domain.Details, error) {
			current.Set(bundle.RelationKeyAperture, domain.String("aperture"))
			current.Set(bundle.RelationKeyRelationMaxCount, domain.Int64(5))
			return current, nil
		})

		// then
		assert.NoError(t, err)

		value, found := f.sb.Details().TryString(bundle.RelationKeyAperture)
		assert.True(t, found)
		assert.Equal(t, "aperture", value)
		assert.True(t, f.sb.HasRelation(f.sb.NewState(), bundle.RelationKeyAperture.String()))

		{
			value, found := f.sb.Details().TryInt64(bundle.RelationKeyRelationMaxCount)
			assert.True(t, found)
			assert.Equal(t, int64(5), value)
			assert.True(t, f.sb.HasRelation(f.sb.NewState(), bundle.RelationKeyRelationMaxCount.String()))
		}
	})

	t.Run("modify details", func(t *testing.T) {
		// given
		f := newBasicFixture(t)
		err := f.sb.SetDetails(nil, []domain.Detail{{
			Key:   bundle.RelationKeySpaceDashboardId,
			Value: domain.String("123"),
		}}, false)
		assert.NoError(t, err)
		f.store.AddObjects(t, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("rel-spaceDashboardId"),
			bundle.RelationKeySpaceId:        domain.String(spaceId),
			bundle.RelationKeyRelationKey:    domain.String("spaceDashboardId"),
			bundle.RelationKeyUniqueKey:      domain.String("rel-spaceDashboardId"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
		}})

		// when
		err = f.basic.UpdateDetails(nil, func(current *domain.Details) (*domain.Details, error) {
			current.Set(bundle.RelationKeySpaceDashboardId, domain.String("new123"))
			return current, nil
		})

		// then
		assert.NoError(t, err)

		value, found := f.sb.Details().TryString(bundle.RelationKeySpaceDashboardId)
		assert.True(t, found)
		assert.Equal(t, "new123", value)
		assert.True(t, f.sb.HasRelation(f.sb.NewState(), bundle.RelationKeySpaceDashboardId.String()))
	})

	t.Run("delete details", func(t *testing.T) {
		// given
		f := newBasicFixture(t)
		err := f.sb.SetDetails(nil, []domain.Detail{{
			Key:   bundle.RelationKeyTargetObjectType,
			Value: domain.String("ot-note"),
		}}, false)
		assert.NoError(t, err)

		// when
		err = f.basic.UpdateDetails(nil, func(current *domain.Details) (*domain.Details, error) {
			current.Delete(bundle.RelationKeyTargetObjectType)
			return current, nil
		})

		// then
		assert.NoError(t, err)

		value, found := f.sb.Details().TryString(bundle.RelationKeyTargetObjectType)
		assert.False(t, found)
		assert.Empty(t, value)
		assert.False(t, f.sb.HasRelation(f.sb.NewState(), bundle.RelationKeyTargetObjectType.String()))
	})

	t.Run("removal of internal relation should fail", func(t *testing.T) {
		// given
		f := newBasicFixture(t)

		err := f.sb.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyName, Value: domain.String("test object")},
			{Key: bundle.RelationKeyDescription, Value: domain.String("Description")},
			{Key: bundle.RelationKeyCreatedDate, Value: domain.Int64(1234567890)},
		}, false)
		require.NoError(t, err)

		// when
		err = f.basic.UpdateDetails(nil, func(current *domain.Details) (*domain.Details, error) {
			current.Delete(bundle.RelationKeyName)
			current.Delete(bundle.RelationKeyDescription)
			current.Delete(bundle.RelationKeyCreatedDate)
			return current, nil
		})

		// then
		assert.Error(t, err)
		assert.Equal(t, "test object", f.sb.Details().GetString(bundle.RelationKeyName))
		assert.Equal(t, "test description", f.sb.Details().GetString(bundle.RelationKeyDescription))
		assert.Equal(t, int64(1234567890), f.sb.Details().GetInt64(bundle.RelationKeyCreatedDate))
	})
}

func TestBasic_SetObjectTypesInState(t *testing.T) {
	t.Run("no error", func(t *testing.T) {
		// given
		f := newBasicFixture(t)

		f.store.AddObjects(t, []objectstore.TestObject{{
			bundle.RelationKeySpaceId:        domain.String(spaceId),
			bundle.RelationKeyId:             domain.String("ot-task"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-task"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_todo)),
		}})

		s := f.sb.NewState()

		// when
		err := f.basic.SetObjectTypesInState(s, []domain.TypeKey{bundle.TypeKeyTask}, false)

		// then
		assert.NoError(t, err)
		assert.Equal(t, bundle.TypeKeyTask, s.ObjectTypeKey())
	})

	t.Run("type change is restricted", func(t *testing.T) {
		// given
		f := newBasicFixture(t)
		f.sb.TestRestrictions = restriction.Restrictions{Object: restriction.ObjectRestrictions{model.Restrictions_TypeChange: {}}}
		s := f.sb.NewState()

		// when
		err := f.basic.SetObjectTypesInState(s, []domain.TypeKey{bundle.TypeKeyTask}, false)

		// then
		assert.ErrorIs(t, err, restriction.ErrRestricted)
	})

	typeKey := "type"
	for _, tc := range []struct {
		from, to    model.ObjectTypeLayout
		shouldError bool
	}{
		{model.ObjectType_basic, model.ObjectType_todo, false},
		{model.ObjectType_profile, model.ObjectType_note, false},
		{model.ObjectType_basic, model.ObjectType_set, true},
		{model.ObjectType_collection, model.ObjectType_todo, true},
		{model.ObjectType_tag, model.ObjectType_note, true},
		{model.ObjectType_dashboard, model.ObjectType_collection, true},
		{model.ObjectType_todo, model.ObjectType_relation, true},
	} {
		t.Run(fmt.Sprintf("change to type with other layout group is restricted. From '%s' to '%s'",
			model.ObjectTypeLayout_name[int32(tc.from)], model.ObjectTypeLayout_name[int32(tc.to)]), func(t *testing.T) {
			// given
			f := newBasicFixture(t)
			f.store.AddObjects(t, []objectstore.TestObject{{
				bundle.RelationKeyId:                domain.String(typeKey),
				bundle.RelationKeySpaceId:           domain.String(spaceId),
				bundle.RelationKeyUniqueKey:         domain.String("ot-" + typeKey),
				bundle.RelationKeyRecommendedLayout: domain.Int64(int64(tc.to)),
			}})
			s := f.sb.NewState()
			s.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(tc.from)))

			// when
			err := f.basic.SetObjectTypesInState(s, []domain.TypeKey{domain.TypeKey(typeKey)}, false)

			// then
			if tc.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}

	t.Run("changing to template type is restricted", func(t *testing.T) {
		// given
		f := newBasicFixture(t)
		s := f.sb.NewState()

		// when
		err := f.basic.SetObjectTypesInState(s, []domain.TypeKey{bundle.TypeKeyTemplate}, false)

		// then
		assert.Error(t, err)
	})

	t.Run("layout settings should be removed", func(t *testing.T) {
		// given
		f := newBasicFixture(t)

		f.store.AddObjects(t, []objectstore.TestObject{{
			bundle.RelationKeySpaceId:   domain.String(spaceId),
			bundle.RelationKeyId:        domain.String(bundle.TypeKeyPage.URL()),
			bundle.RelationKeyUniqueKey: domain.String(bundle.TypeKeyPage.URL()),
		}})

		s := f.sb.NewState()
		s.SetDetail(bundle.RelationKeyLayout, domain.Int64(model.ObjectType_todo))
		s.SetDetail(bundle.RelationKeyLayoutAlign, domain.Int64(model.Block_AlignRight))
		s.SetDetail(bundle.RelationKeyFeaturedRelations, domain.StringList([]string{
			bundle.RelationKeyDescription.String(), bundle.RelationKeyTag.String(),
		}))

		// when
		err := f.basic.SetObjectTypesInState(s, []domain.TypeKey{bundle.TypeKeyPage}, false)

		// then
		assert.NoError(t, err)
		assert.False(t, s.Details().Has(bundle.RelationKeyLayout))
		assert.False(t, s.Details().Has(bundle.RelationKeyLayoutAlign))
		assert.Len(t, s.Details().GetStringList(bundle.RelationKeyFeaturedRelations), 1)
	})
}
