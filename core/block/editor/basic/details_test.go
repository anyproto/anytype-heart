package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type duFixture struct {
	sb    *smarttest.SmartTest
	store *spaceindex.StoreFixture
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

	store := spaceindex.NewStoreFixture(t)

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
		err := f.basic.UpdateDetails(func(current *domain.Details) (*domain.Details, error) {
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
		f := newDUFixture(t)
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
		err = f.basic.UpdateDetails(func(current *domain.Details) (*domain.Details, error) {
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
		f := newDUFixture(t)
		err := f.sb.SetDetails(nil, []domain.Detail{{
			Key:   bundle.RelationKeyTargetObjectType,
			Value: domain.String("ot-note"),
		}}, false)
		assert.NoError(t, err)

		// when
		err = f.basic.UpdateDetails(func(current *domain.Details) (*domain.Details, error) {
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
