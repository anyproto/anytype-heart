package basic

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type duFixture struct {
	sb    *smarttest.SmartTest
	store *objectstore.StoreFixture
	basic DetailsUpdatable
}

var (
	objectId = "objectId"
	spaceId  = "space1"
)

func newDUFixture(t *testing.T) *duFixture {
	sb := smarttest.New(objectId)
	sb.SetDetails(nil, nil, false)
	sb.SetSpaceId(spaceId)

	store := objectstore.NewStoreFixture(t)

	b := NewBasic(sb, store, converter.NewLayoutConverter(), nil, nil)

	return &duFixture{
		sb:    sb,
		store: store,
		basic: b,
	}
}

func TestBasic_UpdateDetails(t *testing.T) {
	t.Run("add new details", func(t *testing.T) {
		// given
		f := newDUFixture(t)
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
		f := newDUFixture(t)
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
		f := newDUFixture(t)
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
