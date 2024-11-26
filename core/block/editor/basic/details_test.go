package basic

import (
	"testing"

	"github.com/gogo/protobuf/types"
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
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
			bundle.RelationKeyId:             pbtypes.String("rel-aperture"),
			bundle.RelationKeySpaceId:        pbtypes.String(spaceId),
			bundle.RelationKeyRelationKey:    pbtypes.String("aperture"),
			bundle.RelationKeyUniqueKey:      pbtypes.String("rel-aperture"),
			bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_longtext)),
		}, {
			bundle.RelationKeyId:             pbtypes.String("rel-maxCount"),
			bundle.RelationKeySpaceId:        pbtypes.String(spaceId),
			bundle.RelationKeyRelationKey:    pbtypes.String("relationMaxCount"),
			bundle.RelationKeyUniqueKey:      pbtypes.String("rel-relationMaxCount"),
			bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_number)),
		}})

		// when
		err := f.basic.UpdateDetails(func(current *types.Struct) (*types.Struct, error) {
			current.Fields[bundle.RelationKeyAperture.String()] = pbtypes.String("aperture")
			current.Fields[bundle.RelationKeyRelationMaxCount.String()] = pbtypes.Int64(5)
			return current, nil
		})

		// then
		assert.NoError(t, err)

		value, found := f.sb.Details().Fields[bundle.RelationKeyAperture.String()]
		assert.True(t, found)
		assert.Equal(t, pbtypes.String("aperture"), value)
		assert.True(t, f.sb.HasRelation(f.sb.NewState(), bundle.RelationKeyAperture.String()))

		value, found = f.sb.Details().Fields[bundle.RelationKeyRelationMaxCount.String()]
		assert.True(t, found)
		assert.Equal(t, pbtypes.Int64(5), value)
		assert.True(t, f.sb.HasRelation(f.sb.NewState(), bundle.RelationKeyRelationMaxCount.String()))
	})

	t.Run("modify details", func(t *testing.T) {
		// given
		f := newBasicFixture(t)
		err := f.sb.SetDetails(nil, []*model.Detail{{
			Key:   bundle.RelationKeySpaceDashboardId.String(),
			Value: pbtypes.String("123"),
		}}, false)
		assert.NoError(t, err)
		f.store.AddObjects(t, []objectstore.TestObject{{
			bundle.RelationKeyId:             pbtypes.String("rel-spaceDashboardId"),
			bundle.RelationKeySpaceId:        pbtypes.String(spaceId),
			bundle.RelationKeyRelationKey:    pbtypes.String("spaceDashboardId"),
			bundle.RelationKeyUniqueKey:      pbtypes.String("rel-spaceDashboardId"),
			bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_object)),
		}})

		// when
		err = f.basic.UpdateDetails(func(current *types.Struct) (*types.Struct, error) {
			current.Fields[bundle.RelationKeySpaceDashboardId.String()] = pbtypes.String("new123")
			return current, nil
		})

		// then
		assert.NoError(t, err)

		value, found := f.sb.Details().Fields[bundle.RelationKeySpaceDashboardId.String()]
		assert.True(t, found)
		assert.Equal(t, pbtypes.String("new123"), value)
		assert.True(t, f.sb.HasRelation(f.sb.NewState(), bundle.RelationKeySpaceDashboardId.String()))
	})

	t.Run("delete details", func(t *testing.T) {
		// given
		f := newBasicFixture(t)
		err := f.sb.SetDetails(nil, []*model.Detail{{
			Key:   bundle.RelationKeyTargetObjectType.String(),
			Value: pbtypes.String("ot-note"),
		}}, false)
		assert.NoError(t, err)

		// when
		err = f.basic.UpdateDetails(func(current *types.Struct) (*types.Struct, error) {
			delete(current.Fields, bundle.RelationKeyTargetObjectType.String())
			return current, nil
		})

		// then
		assert.NoError(t, err)

		value, found := f.sb.Details().Fields[bundle.RelationKeyTargetObjectType.String()]
		assert.False(t, found)
		assert.Nil(t, value)
		assert.False(t, f.sb.HasRelation(f.sb.NewState(), bundle.RelationKeyTargetObjectType.String()))
	})
}

func TestBasic_SetObjectTypesInState(t *testing.T) {
	t.Run("no error", func(t *testing.T) {
		// given
		f := newBasicFixture(t)

		f.lastUsed.EXPECT().UpdateLastUsedDate(mock.Anything, bundle.TypeKeyTask, mock.Anything).Return().Once()
		f.store.AddObjects(t, []objectstore.TestObject{{
			bundle.RelationKeySpaceId:   pbtypes.String(spaceId),
			bundle.RelationKeyId:        pbtypes.String("ot-task"),
			bundle.RelationKeyUniqueKey: pbtypes.String("ot-task"),
			bundle.RelationKeyLayout:    pbtypes.Int64(int64(model.ObjectType_todo)),
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
