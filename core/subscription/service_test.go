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
	t.Run("dependencies", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close()
		defer fx.ctrl.Finish()

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
		}, nil)
		fx.store.EXPECT().GetRelation(bundle.RelationKeyAuthor.String()).Return(&model.Relation{
			Key:    bundle.RelationKeyAuthor.String(),
			Format: model.RelationFormat_object,
		}, nil)

		fx.store.EXPECT().QueryById([]string{"author1"}).Return([]database.Record{
			{Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("author1"),
				"name": pbtypes.String("author1"),
			}}},
		}, nil)

		resp, err := fx.Search(pb.RpcObjectSearchSubscribeRequest{
			SubId: "test",
			Keys:  []string{bundle.RelationKeyName.String(), bundle.RelationKeyAuthor.String()},
		})
		require.NoError(t, err)

		assert.Len(t, resp.Records, 1)
		assert.Len(t, resp.Dependencies, 1)

		fx.store.EXPECT().QueryById([]string{"author2", "author3", "1"}).Return([]database.Record{
			{Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("author2"),
				"name": pbtypes.String("author2"),
			}}},
			{Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("author3"),
				"name": pbtypes.String("author3"),
			}}},
			{Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("1"),
				"name": pbtypes.String("one"),
				"author": pbtypes.StringList([]string{"author2", "author3", "1"}),
			}}},
		}, nil)

		fx.Service.(*service).onChange([]*entry{
			{id: "1", data: &types.Struct{Fields: map[string]*types.Value{
				"id":     pbtypes.String("1"),
				"name":   pbtypes.String("one"),
				"author": pbtypes.StringList([]string{"author2", "author3", "1"}),
			}}},
		})

		assert.Len(t, fx.Service.(*service).cache.entries, 3)
		assert.Equal(t, 2,  fx.Service.(*service).cache.entries["1"].refs)
		assert.Equal(t, 1,  fx.Service.(*service).cache.entries["author2"].refs)
		assert.Equal(t, 1,  fx.Service.(*service).cache.entries["author3"].refs)


		fx.events = fx.events[:0]

		fx.Service.(*service).onChange([]*entry{
			{id: "1", data: &types.Struct{Fields: map[string]*types.Value{
				"id":     pbtypes.String("1"),
				"name":   pbtypes.String("one"),
			}}},
		})

		/*
		for _, e := range fx.events {
			t.Log(pbtypes.Sprint(e))
		}
		for _, e := range fx.Service.(*service).cache.entries {
			t.Log(e.id, e.refs)
		}*/

		assert.Len(t, fx.Service.(*service).cache.entries, 1)
		assert.Equal(t, 1,  fx.Service.(*service).cache.entries["1"].refs)

		assert.NoError(t, fx.Unsubscribe("test"))
		assert.Len(t, fx.Service.(*service).cache.entries, 0)
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
