package subscription

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_Search(t *testing.T) {

	var newSub = func(fx *fixture, subId string) {
		fx.store.EXPECT().QueryRaw(gomock.Any()).Return(
			[]database.Record{
				{Details: &types.Struct{Fields: map[string]*types.Value{
					"id":     pbtypes.String("1"),
					"name":   pbtypes.String("one"),
					"author": pbtypes.StringList([]string{"author1"}),
				}}},
			},
			nil,
		)
		fx.store.EXPECT().GetRelation(bundle.RelationKeyName.String()).Return(&model.Relation{
			Key:    bundle.RelationKeyName.String(),
			Format: model.RelationFormat_shorttext,
		}, nil).AnyTimes()
		fx.store.EXPECT().GetRelation(bundle.RelationKeyAuthor.String()).Return(&model.Relation{
			Key:    bundle.RelationKeyAuthor.String(),
			Format: model.RelationFormat_object,
		}, nil).AnyTimes()

		fx.store.EXPECT().QueryById([]string{"author1"}).Return([]database.Record{
			{Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("author1"),
				"name": pbtypes.String("author1"),
			}}},
		}, nil).AnyTimes()

		resp, err := fx.Search(pb.RpcObjectSearchSubscribeRequest{
			SubId: subId,
			Keys:  []string{bundle.RelationKeyName.String(), bundle.RelationKeyAuthor.String()},
		})
		require.NoError(t, err)

		assert.Len(t, resp.Records, 1)
		assert.Len(t, resp.Dependencies, 1)
	}

	t.Run("dependencies", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close()
		defer fx.ctrl.Finish()

		newSub(fx, "test")

		fx.store.EXPECT().QueryById([]string{"author2", "author3"}).Return([]database.Record{
			{Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("author2"),
				"name": pbtypes.String("author2"),
			}}},
			{Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("author3"),
				"name": pbtypes.String("author3"),
			}}},
		}, nil)

		fx.Service.(*service).onChange([]*entry{
			{id: "1", data: &types.Struct{Fields: map[string]*types.Value{
				"id":     pbtypes.String("1"),
				"name":   pbtypes.String("one"),
				"author": pbtypes.StringList([]string{"author2", "author3", "1"}),
			}}},
		})

		require.Len(t, fx.Service.(*service).cache.entries, 3)
		assert.Len(t, fx.Service.(*service).cache.entries["1"].SubIds(), 2)
		assert.Len(t, fx.Service.(*service).cache.entries["author2"].SubIds(), 1)
		assert.Len(t, fx.Service.(*service).cache.entries["author3"].SubIds(), 1)

		fx.events = fx.events[:0]

		fx.Service.(*service).onChange([]*entry{
			{id: "1", data: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("1"),
				"name": pbtypes.String("one"),
			}}},
		})

		assert.Len(t, fx.Service.(*service).cache.entries, 1)

		assert.NoError(t, fx.Unsubscribe("test"))
		assert.Len(t, fx.Service.(*service).cache.entries, 0)
	})
	t.Run("cache ref counter", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close()
		defer fx.ctrl.Finish()

		newSub(fx, "test")

		require.Len(t, fx.Service.(*service).cache.entries, 2)
		assert.Equal(t, []string{"test"}, fx.Service.(*service).cache.entries["1"].SubIds())
		assert.Equal(t, []string{"test/dep"}, fx.Service.(*service).cache.entries["author1"].SubIds())

		newSub(fx, "test1")

		require.Len(t, fx.Service.(*service).cache.entries, 2)
		assert.Len(t, fx.Service.(*service).cache.entries["1"].SubIds(), 2)
		assert.Len(t, fx.Service.(*service).cache.entries["author1"].SubIds(), 2)
	})

	t.Run("filter deps", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close()
		defer fx.ctrl.Finish()

		fx.store.EXPECT().QueryRaw(gomock.Any()).Return(
			[]database.Record{
				{Details: &types.Struct{Fields: map[string]*types.Value{
					"id":   pbtypes.String("1"),
					"name": pbtypes.String("one"),
				}}},
			},
			nil,
		)
		fx.store.EXPECT().GetRelation(bundle.RelationKeyName.String()).Return(&model.Relation{
			Key:    bundle.RelationKeyName.String(),
			Format: model.RelationFormat_shorttext,
		}, nil).AnyTimes()
		fx.store.EXPECT().GetRelation(bundle.RelationKeyAuthor.String()).Return(&model.Relation{
			Key:    bundle.RelationKeyAuthor.String(),
			Format: model.RelationFormat_object,
		}, nil).AnyTimes()

		fx.store.EXPECT().QueryById([]string{"force1", "force2"}).Return([]database.Record{
			{Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("force1"),
				"name": pbtypes.String("force1"),
			}}},
			{Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("force2"),
				"name": pbtypes.String("force2"),
			}}},
		}, nil)

		var resp, err = fx.Search(pb.RpcObjectSearchSubscribeRequest{
			SubId: "subId",
			Keys:  []string{bundle.RelationKeyName.String(), bundle.RelationKeyAuthor.String()},
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyAuthor.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.StringList([]string{"force1", "force2"}),
				},
			},
		})
		require.NoError(t, err)

		assert.Len(t, resp.Records, 1)
		assert.Len(t, resp.Dependencies, 2)

	})
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	a := testapp.New()
	testMock.RegisterMockObjectStore(ctrl, a)
	fx := &fixture{
		Service: New(),
		a:       a,
		ctrl:    ctrl,
		store:   a.MustComponent(objectstore.CName).(*testMock.MockObjectStore),
	}
	fx.sender = &testapp.EventSender{F: func(e *pb.Event) {
		fx.events = append(fx.events, e)
	}}
	a.Register(fx.Service)
	a.Register(fx.sender)
	fx.store.EXPECT().SubscribeForAll(gomock.Any())
	require.NoError(t, a.Start())
	return fx
}

type fixture struct {
	Service
	a      *testapp.TestApp
	ctrl   *gomock.Controller
	store  *testMock.MockObjectStore
	sender *testapp.EventSender
	events []*pb.Event
}
