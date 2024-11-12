package detailservice

import (
	"context"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/dateutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func relationObject(key domain.RelationKey, format model.RelationFormat) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             pbtypes.String(key.URL()),
		bundle.RelationKeySpaceId:        pbtypes.String(spaceId),
		bundle.RelationKeyLayout:         pbtypes.Float64(float64(model.ObjectType_relation)),
		bundle.RelationKeyRelationKey:    pbtypes.String(key.String()),
		bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(format)),
	}
}

func TestService_ListRelationsWithValue(t *testing.T) {
	now := time.Now()
	store := objectstore.NewStoreFixture(t)
	store.AddObjects(t, spaceId, []objectstore.TestObject{
		// relations
		relationObject(bundle.RelationKeyLastModifiedDate, model.RelationFormat_date),
		relationObject(bundle.RelationKeyAddedDate, model.RelationFormat_date),
		relationObject(bundle.RelationKeyCreatedDate, model.RelationFormat_date),
		relationObject(bundle.RelationKeyLinks, model.RelationFormat_object),
		relationObject(bundle.RelationKeyMentions, model.RelationFormat_object),
		relationObject(bundle.RelationKeyName, model.RelationFormat_longtext),
		relationObject(bundle.RelationKeyIsHidden, model.RelationFormat_checkbox),
		relationObject(bundle.RelationKeyIsFavorite, model.RelationFormat_checkbox),
		relationObject("daysTillSummer", model.RelationFormat_number),
		relationObject(bundle.RelationKeyCoverX, model.RelationFormat_number),
		{
			bundle.RelationKeyId:               pbtypes.String("obj1"),
			bundle.RelationKeySpaceId:          pbtypes.String(spaceId),
			bundle.RelationKeyCreatedDate:      pbtypes.Int64(now.Add(-5 * time.Minute).Unix()),
			bundle.RelationKeyAddedDate:        pbtypes.Int64(now.Add(-3 * time.Minute).Unix()),
			bundle.RelationKeyLastModifiedDate: pbtypes.Int64(now.Add(-1 * time.Minute).Unix()),
			bundle.RelationKeyIsFavorite:       pbtypes.Bool(true),
			"daysTillSummer":                   pbtypes.Int64(300),
			bundle.RelationKeyLinks:            pbtypes.StringList([]string{"obj2", "obj3", dateutil.TimeToDateId(now.Add(-30 * time.Minute))}),
		},
		{
			bundle.RelationKeyId:               pbtypes.String("obj2"),
			bundle.RelationKeySpaceId:          pbtypes.String(spaceId),
			bundle.RelationKeyName:             pbtypes.String(dateutil.TimeToDateId(now)),
			bundle.RelationKeyCreatedDate:      pbtypes.Int64(now.Add(-24*time.Hour - 5*time.Minute).Unix()),
			bundle.RelationKeyAddedDate:        pbtypes.Int64(now.Add(-24*time.Hour - 3*time.Minute).Unix()),
			bundle.RelationKeyLastModifiedDate: pbtypes.Int64(now.Add(-1 * time.Minute).Unix()),
			bundle.RelationKeyCoverX:           pbtypes.Int64(300),
		},
		{
			bundle.RelationKeyId:               pbtypes.String("obj3"),
			bundle.RelationKeySpaceId:          pbtypes.String(spaceId),
			bundle.RelationKeyIsHidden:         pbtypes.Bool(true),
			bundle.RelationKeyCreatedDate:      pbtypes.Int64(now.Add(-3 * time.Minute).Unix()),
			bundle.RelationKeyLastModifiedDate: pbtypes.Int64(now.Unix()),
			bundle.RelationKeyIsFavorite:       pbtypes.Bool(true),
			bundle.RelationKeyCoverX:           pbtypes.Int64(300),
			bundle.RelationKeyMentions:         pbtypes.StringList([]string{dateutil.TimeToDateId(now), dateutil.TimeToDateId(now.Add(-24 * time.Hour))}),
		},
	})

	bs := service{store: store}

	for _, tc := range []struct {
		name         string
		value        *types.Value
		expectedList []*pb.RpcRelationListWithValueResponseResponseItem
	}{
		{
			"date object - today",
			pbtypes.String(dateutil.TimeToDateId(now)),
			[]*pb.RpcRelationListWithValueResponseResponseItem{
				{bundle.RelationKeyAddedDate.String(), 1},
				{bundle.RelationKeyCreatedDate.String(), 2},
				{bundle.RelationKeyLastModifiedDate.String(), 3},
				{bundle.RelationKeyLinks.String(), 1},
				{bundle.RelationKeyMentions.String(), 1},
				{bundle.RelationKeyName.String(), 1},
			},
		},
		{
			"date object - yesterday",
			pbtypes.String(dateutil.TimeToDateId(now.Add(-24 * time.Hour))),
			[]*pb.RpcRelationListWithValueResponseResponseItem{
				{bundle.RelationKeyAddedDate.String(), 1},
				{bundle.RelationKeyCreatedDate.String(), 1},
				{bundle.RelationKeyMentions.String(), 1},
			},
		},
		{
			"number",
			pbtypes.Int64(300),
			[]*pb.RpcRelationListWithValueResponseResponseItem{
				{bundle.RelationKeyCoverX.String(), 2},
				{"daysTillSummer", 1},
			},
		},
		{
			"bool",
			pbtypes.Bool(true),
			[]*pb.RpcRelationListWithValueResponseResponseItem{
				{bundle.RelationKeyIsFavorite.String(), 2},
				{bundle.RelationKeyIsHidden.String(), 1},
			},
		},
		{
			"string list",
			pbtypes.StringList([]string{"obj2", "obj3", dateutil.TimeToDateId(now.Add(-30 * time.Minute))}),
			[]*pb.RpcRelationListWithValueResponseResponseItem{
				{bundle.RelationKeyLinks.String(), 1},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			list, err := bs.ListRelationsWithValue(spaceId, tc.value)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedList, list)
		})
	}
}

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
			pbtypes.GetStringList(sb.Details(), bundle.RelationKeyRecommendedRelations.String()))
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
		sb.Doc.(*state.State).SetDetails(&types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRecommendedRelations.String(): pbtypes.StringList([]string{
				bundle.RelationKeyAssignee.URL(),
				bundle.RelationKeyIsFavorite.URL(),
				bundle.RelationKeyDone.URL(),
				bundle.RelationKeyLinkedProjects.URL(),
			}),
		}})
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
			pbtypes.GetStringList(sb.Details(), bundle.RelationKeyRecommendedRelations.String()))
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
