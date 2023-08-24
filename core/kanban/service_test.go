package kanban

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/relation/mock_relation"
	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/clientds"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type quicSetter struct{}

func (q quicSetter) Init(a *app.App) (err error) {
	return
}

func (q quicSetter) Name() (name string) {
	return peerservice.CName
}

func (q quicSetter) PreferQuic(_ bool) {}

func Test_GrouperTags(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)

	a := new(app.App)
	tp := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
	tp.EXPECT().Name().Return("typeprovider")
	tp.EXPECT().Init(a).Return(nil)

	relationService := mock_relation.NewMockService(t)
	relationService.EXPECT().Name().Return("relation")
	relationService.EXPECT().Init(a).Return(nil)

	relationWithFormatTag := &relationutils.Relation{&model.Relation{Format: model.RelationFormat_tag}}
	relationService.EXPECT().FetchRelationByKey("", "tag").Return(relationWithFormatTag, nil)

	ds := objectstore.New()
	kanbanSrv := New()
	err := a.Register(quicSetter{}).
		Register(&config.DefaultConfig).
		Register(wallet.NewWithRepoDirAndRandomKeys(tmpDir)).
		Register(clientds.New()).
		Register(ftsearch.New()).
		Register(ds).
		Register(kanbanSrv).
		Register(tp).
		Register(relationService).
		Start(context.Background())
	require.NoError(t, err)

	require.NoError(t, ds.UpdateObjectDetails("rel-tag", &types.Struct{
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

	require.NoError(t, ds.UpdateObjectDetails(idTag1, &types.Struct{
		Fields: map[string]*types.Value{
			"id":          pbtypes.String(idTag1),
			"relationKey": pbtypes.String("tag"),
			"type":        pbtypes.String(bundle.TypeKeyRelationOption.URL()),
			"layout":      pbtypes.Int64(int64(model.ObjectType_relationOption)),
		},
	}))

	require.NoError(t, ds.UpdateObjectDetails(idTag2, &types.Struct{
		Fields: map[string]*types.Value{
			"id":          pbtypes.String(idTag2),
			"relationKey": pbtypes.String("tag"),
			"type":        pbtypes.String(bundle.TypeKeyRelationOption.URL()),
			"layout":      pbtypes.Int64(int64(model.ObjectType_relationOption)),
		},
	}))
	require.NoError(t, ds.UpdateObjectDetails(idTag3, &types.Struct{
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

	require.NoError(t, ds.UpdateObjectDetails(id1, &types.Struct{
		Fields: map[string]*types.Value{"name": pbtypes.String("one")},
	}))
	require.NoError(t, ds.UpdateObjectSnippet(id1, "s1"))

	require.NoError(t, ds.UpdateObjectDetails(id2, &types.Struct{Fields: map[string]*types.Value{
		"name": pbtypes.String("two"),
		"tag":  pbtypes.StringList([]string{idTag1}),
	}}))
	require.NoError(t, ds.UpdateObjectSnippet(id1, "s2"))

	require.NoError(t, ds.UpdateObjectDetails(id3, &types.Struct{Fields: map[string]*types.Value{
		"name": pbtypes.String("three"),
		"tag":  pbtypes.StringList([]string{idTag1, idTag2, idTag3}),
	}}))
	require.NoError(t, ds.UpdateObjectSnippet(id1, "s3"))

	require.NoError(t, ds.UpdateObjectDetails(id4, &types.Struct{Fields: map[string]*types.Value{
		"name": pbtypes.String("four"),
		"tag":  pbtypes.StringList([]string{idTag1, idTag3}),
	}}))
	require.NoError(t, ds.UpdateObjectSnippet(id1, "s4"))

	grouper, err := kanbanSrv.Grouper("", "tag")
	require.NoError(t, err)
	err = grouper.InitGroups(nil)
	require.NoError(t, err)
	groups, err := grouper.MakeDataViewGroups()
	require.NoError(t, err)
	require.Len(t, groups, 6)

	f := &database.Filters{FilterObj: database.FilterEq{Key: "name", Cond: 1, Value: pbtypes.String("three")}}
	err = grouper.InitGroups(f)
	require.NoError(t, err)
	groups, err = grouper.MakeDataViewGroups()
	require.NoError(t, err)
	require.Len(t, groups, 5)
}
