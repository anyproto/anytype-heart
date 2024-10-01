package kanban

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func Test_GrouperTags(t *testing.T) {
	const spaceId = "space1"
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)

	a := new(app.App)
	tp := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
	tp.EXPECT().Name().Return("typeprovider")
	tp.EXPECT().Init(a).Return(nil)

	objectStore := objectstore.NewStoreFixture(t)

	objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             pbtypes.String("tag1"),
			bundle.RelationKeyUniqueKey:      pbtypes.String("rel-tag"),
			bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_tag)),
			bundle.RelationKeySpaceId:        pbtypes.String(spaceId),
		},
	})

	kanbanSrv := New()
	err := a.Register(objectStore).
		Register(kanbanSrv).
		Register(tp).
		Start(context.Background())
	require.NoError(t, err)

	store := objectStore.SpaceIndex(spaceId)

	require.NoError(t, store.UpdateObjectDetails(context.Background(), "rel-tag", &types.Struct{
		Fields: map[string]*types.Value{
			"id":             pbtypes.String("rel-tag"),
			"relationKey":    pbtypes.String("tag"),
			"relationFormat": pbtypes.Int64(int64(model.RelationFormat_tag)),
			"type":           pbtypes.String(bundle.TypeKeyRelation.URL()),
			"layout":         pbtypes.Int64(int64(model.ObjectType_relation)),
		},
	}))

	idTag1 := bson.NewObjectId().Hex()
	idTag2 := bson.NewObjectId().Hex()
	idTag3 := bson.NewObjectId().Hex()

	require.NoError(t, store.UpdateObjectDetails(context.Background(), idTag1, &types.Struct{
		Fields: map[string]*types.Value{
			"id":          pbtypes.String(idTag1),
			"relationKey": pbtypes.String("tag"),
			"type":        pbtypes.String(bundle.TypeKeyRelationOption.URL()),
			"layout":      pbtypes.Int64(int64(model.ObjectType_relationOption)),
		},
	}))

	require.NoError(t, store.UpdateObjectDetails(context.Background(), idTag2, &types.Struct{
		Fields: map[string]*types.Value{
			"id":          pbtypes.String(idTag2),
			"relationKey": pbtypes.String("tag"),
			"type":        pbtypes.String(bundle.TypeKeyRelationOption.URL()),
			"layout":      pbtypes.Int64(int64(model.ObjectType_relationOption)),
		},
	}))
	require.NoError(t, store.UpdateObjectDetails(context.Background(), idTag3, &types.Struct{
		Fields: map[string]*types.Value{
			"id":          pbtypes.String(idTag3),
			"relationKey": pbtypes.String("tag"),
			"type":        pbtypes.String(bundle.TypeKeyRelationOption.URL()),
			"layout":      pbtypes.Int64(int64(model.ObjectType_relationOption)),
		},
	}))

	id1 := bson.NewObjectId().Hex()
	id2 := bson.NewObjectId().Hex()
	id3 := bson.NewObjectId().Hex()
	id4 := bson.NewObjectId().Hex()

	require.NoError(t, store.UpdateObjectDetails(context.Background(), id1, &types.Struct{
		Fields: map[string]*types.Value{"name": pbtypes.String("one")},
	}))

	require.NoError(t, store.UpdateObjectDetails(context.Background(), id2, &types.Struct{Fields: map[string]*types.Value{
		"name": pbtypes.String("two"),
		"tag":  pbtypes.StringList([]string{idTag1}),
	}}))

	require.NoError(t, store.UpdateObjectDetails(context.Background(), id3, &types.Struct{Fields: map[string]*types.Value{
		"name": pbtypes.String("three"),
		"tag":  pbtypes.StringList([]string{idTag1, idTag2, idTag3}),
	}}))

	require.NoError(t, store.UpdateObjectDetails(context.Background(), id4, &types.Struct{Fields: map[string]*types.Value{
		"name": pbtypes.String("four"),
		"tag":  pbtypes.StringList([]string{idTag1, idTag3}),
	}}))

	grouper, err := kanbanSrv.Grouper(spaceId, "tag")
	require.NoError(t, err)
	err = grouper.InitGroups(spaceId, nil)
	require.NoError(t, err)
	groups, err := grouper.MakeDataViewGroups()
	require.NoError(t, err)
	require.Len(t, groups, 6)

	f := &database.Filters{FilterObj: database.FilterEq{Key: "name", Cond: 1, Value: pbtypes.String("three")}}
	err = grouper.InitGroups(spaceId, f)
	require.NoError(t, err)
	groups, err = grouper.MakeDataViewGroups()
	require.NoError(t, err)
	require.Len(t, groups, 5)
}
