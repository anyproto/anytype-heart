package block

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/mock_objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type fixture struct {
	blockService *Service
	store        *mock_objectstore.MockObjectStore

	sb  *smarttest.SmartTest
	spc *mock_clientspace.MockSpace
}

var (
	objectId = "id"
	spaceId  = "spaceId"
)

func newFixture(t *testing.T) *fixture {
	blockService := New()

	sb := smarttest.New(objectId)
	sb.SetDetails(nil, nil, false)
	sb.SetSpaceId(spaceId)

	spc := mock_clientspace.NewMockSpace(t)
	spc.EXPECT().GetObject(mock.Anything, objectId).Return(sb, nil).Times(1)

	spaceService := mock_space.NewMockService(t)
	spaceService.EXPECT().Get(mock.Anything, spaceId).Return(spc, nil).Times(1)

	resolver := mock_idresolver.NewMockResolver(t)
	resolver.EXPECT().ResolveSpaceID(objectId).Return(spaceId, nil).Times(1)

	store := mock_objectstore.NewMockObjectStore(t)

	blockService.spaceService = spaceService
	blockService.resolver = resolver
	blockService.objectStore = store

	return &fixture{
		blockService: blockService,
		sb:           sb,
		store:        store,
		spc:          spc,
	}
}

func TestService_ModifyDetails(t *testing.T) {

	t.Run("add new details", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.store.EXPECT().FetchRelationByKeys(spaceId, bundle.RelationKeyAperture.String(), bundle.RelationKeyRelationMaxCount.String()).
			Return(relationutils.Relations{
				{&model.Relation{
					Key:    bundle.RelationKeyAperture.String(),
					Format: model.RelationFormat_longtext,
				}}, {&model.Relation{
					Key:    bundle.RelationKeyRelationMaxCount.String(),
					Format: model.RelationFormat_number,
				}}}, nil)

		// when
		err := f.blockService.ModifyDetails(objectId, func(current *types.Struct) (*types.Struct, error) {
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
		f := newFixture(t)
		err := f.sb.SetDetails(nil, []*pb.RpcObjectSetDetailsDetail{{
			Key:   bundle.RelationKeySpaceDashboardId.String(),
			Value: pbtypes.String("123"),
		}}, false)
		assert.NoError(t, err)
		f.store.EXPECT().FetchRelationByKeys(spaceId, bundle.RelationKeySpaceDashboardId.String()).
			Return(relationutils.Relations{
				{&model.Relation{
					Key:    bundle.RelationKeySpaceDashboardId.String(),
					Format: model.RelationFormat_object,
				}}}, nil)

		// when
		err = f.blockService.ModifyDetails(objectId, func(current *types.Struct) (*types.Struct, error) {
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
		f := newFixture(t)
		err := f.sb.SetDetails(nil, []*pb.RpcObjectSetDetailsDetail{{
			Key:   bundle.RelationKeyTargetObjectType.String(),
			Value: pbtypes.String("note"),
		}}, false)
		assert.NoError(t, err)
		f.store.EXPECT().FetchRelationByKeys(spaceId).Return(nil, nil)

		// when
		err = f.blockService.ModifyDetails(objectId, func(current *types.Struct) (*types.Struct, error) {
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
