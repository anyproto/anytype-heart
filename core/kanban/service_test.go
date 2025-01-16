package kanban

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider/mock_typeprovider"
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
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyUniqueKey:      domain.String("rel-tag"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
			bundle.RelationKeySpaceId:        domain.String(spaceId),
		},
	})

	kanbanSrv := New()
	err := a.Register(objectStore).
		Register(kanbanSrv).
		Register(tp).
		Start(context.Background())
	require.NoError(t, err)

	store := objectStore.SpaceIndex(spaceId)

	require.NoError(t, store.UpdateObjectDetails(context.Background(), "rel-tag", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		"id":             domain.String("rel-tag"),
		"relationKey":    domain.String("tag"),
		"relationFormat": domain.Int64(int64(model.RelationFormat_tag)),
		"type":           domain.String(bundle.TypeKeyRelation.URL()),
		"layout":         domain.Int64(int64(model.ObjectType_relation)),
	})))

	idTag1 := bson.NewObjectId().Hex()
	idTag2 := bson.NewObjectId().Hex()
	idTag3 := bson.NewObjectId().Hex()

	require.NoError(t, store.UpdateObjectDetails(context.Background(), idTag1, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		"id":          domain.String(idTag1),
		"relationKey": domain.String("tag"),
		"type":        domain.String(bundle.TypeKeyRelationOption.URL()),
		"layout":      domain.Int64(int64(model.ObjectType_relationOption)),
	})))

	require.NoError(t, store.UpdateObjectDetails(context.Background(), idTag2, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		"id":          domain.String(idTag2),
		"relationKey": domain.String("tag"),
		"type":        domain.String(bundle.TypeKeyRelationOption.URL()),
		"layout":      domain.Int64(int64(model.ObjectType_relationOption)),
	})))
	require.NoError(t, store.UpdateObjectDetails(context.Background(), idTag3, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		"id":          domain.String(idTag3),
		"relationKey": domain.String("tag"),
		"type":        domain.String(bundle.TypeKeyRelationOption.URL()),
		"layout":      domain.Int64(int64(model.ObjectType_relationOption)),
	})))

	id1 := bson.NewObjectId().Hex()
	id2 := bson.NewObjectId().Hex()
	id3 := bson.NewObjectId().Hex()
	id4 := bson.NewObjectId().Hex()

	require.NoError(t, store.UpdateObjectDetails(context.Background(), id1, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"name": domain.String("one")})))

	require.NoError(t, store.UpdateObjectDetails(context.Background(), id2, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		"name": domain.String("two"),
		"tag":  domain.StringList([]string{idTag1}),
	})))

	require.NoError(t, store.UpdateObjectDetails(context.Background(), id3, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		"name": domain.String("three"),
		"tag":  domain.StringList([]string{idTag1, idTag2, idTag3}),
	})))

	require.NoError(t, store.UpdateObjectDetails(context.Background(), id4, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		"name": domain.String("four"),
		"tag":  domain.StringList([]string{idTag1, idTag3}),
	})))

	grouper, err := kanbanSrv.Grouper(spaceId, "tag")
	require.NoError(t, err)
	err = grouper.InitGroups(spaceId, nil)
	require.NoError(t, err)
	groups, err := grouper.MakeDataViewGroups()
	require.NoError(t, err)
	require.Len(t, groups, 6)

	f := &database.Filters{FilterObj: database.FilterEq{Key: "name", Cond: 1, Value: domain.String("three")}}
	err = grouper.InitGroups(spaceId, f)
	require.NoError(t, err)
	groups, err = grouper.MakeDataViewGroups()
	require.NoError(t, err)
	require.Len(t, groups, 5)
}
