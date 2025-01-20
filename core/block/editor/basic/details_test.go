package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/lastused/mock_lastused"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type basicFixture struct {
	sb       *smarttest.SmartTest
	store    *spaceindex.StoreFixture
	lastUsed *mock_lastused.MockObjectUsageUpdater
	basic    CommonOperations
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
	lastUsed := mock_lastused.NewMockObjectUsageUpdater(t)

	b := NewBasic(sb, store, converter.NewLayoutConverter(), nil, lastUsed)

	return &basicFixture{
		sb:       sb,
		store:    store,
		lastUsed: lastUsed,
		basic:    b,
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
}

func TestBasic_SetObjectTypesInState(t *testing.T) {
	t.Run("no error", func(t *testing.T) {
		// given
		f := newBasicFixture(t)

		f.lastUsed.EXPECT().UpdateLastUsedDate(mock.Anything, bundle.TypeKeyTask, mock.Anything).Return().Once()
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
		f.sb.TestRestrictions = restriction.Restrictions{Object: []model.RestrictionsObjectRestriction{model.Restrictions_TypeChange}}
		s := f.sb.NewState()

		// when
		err := f.basic.SetObjectTypesInState(s, []domain.TypeKey{bundle.TypeKeyTask}, false)

		// then
		assert.ErrorIs(t, err, restriction.ErrRestricted)
	})

	t.Run("changing to template type is restricted", func(t *testing.T) {
		// given
		f := newBasicFixture(t)
		s := f.sb.NewState()

		// when
		err := f.basic.SetObjectTypesInState(s, []domain.TypeKey{bundle.TypeKeyTemplate}, false)

		// then
		assert.Error(t, err)
	})
}
