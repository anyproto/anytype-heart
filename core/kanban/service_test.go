package kanban

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"

	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	smartblock2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database/filter"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore/clientds"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func Test_GrouperTags(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)

	app := testapp.New()
	defer app.Close(context.Background())
	tp := typeprovider.New(nil)
	tp.Init(nil)
	ds := objectstore.New(tp)
	kanbanSrv := New()
	err := app.With(&config.DefaultConfig).
		With(wallet.NewWithRepoPathAndKeys(tmpDir, nil, nil)).
		With(clientds.New()).
		With(ftsearch.New()).
		With(ds).
		With(kanbanSrv).
		Start(context.Background())
	require.NoError(t, err)

	require.NoError(t, ds.CreateObject("rel-tag", &types.Struct{
		Fields: map[string]*types.Value{
			"id":             pbtypes.String("rel-tag"),
			"relationKey":    pbtypes.String("tag"),
			"relationFormat": pbtypes.Int64(int64(model.RelationFormat_tag)),

			"type": pbtypes.String("ot-relation"),
		},
	}, nil, ""))

	id1 := bson.NewObjectId().String()
	id2 := bson.NewObjectId().String()
	id3 := bson.NewObjectId().String()
	tp.RegisterStaticType(id1, smartblock2.SmartBlockTypePage)
	tp.RegisterStaticType(id2, smartblock2.SmartBlockTypePage)
	tp.RegisterStaticType(id3, smartblock2.SmartBlockTypePage)

	require.NoError(t, ds.CreateObject(id1, &types.Struct{
		Fields: map[string]*types.Value{
			"name": pbtypes.String("one"),
			"type": pbtypes.StringList([]string{"ot-a1"}),
		},
	}, nil, "s1"))

	require.NoError(t, ds.CreateObject(id2, &types.Struct{Fields: map[string]*types.Value{
		"name": pbtypes.String("two"),
		"type": pbtypes.StringList([]string{"ot-a2"}),
		"tag":  pbtypes.StringList([]string{"tag1"}),
	}}, nil, "s2"))
	require.NoError(t, ds.CreateObject(id3, &types.Struct{Fields: map[string]*types.Value{
		"name": pbtypes.String("three"),
		"type": pbtypes.StringList([]string{"ot-a2"}),
		"tag":  pbtypes.StringList([]string{"tag1", "tag2", "tag3"}),
	}}, nil, "s3"))

	grouper, err := kanbanSrv.Grouper("tag")
	require.NoError(t, err)
	err = grouper.InitGroups(nil)
	require.NoError(t, err)
	groups, err := grouper.MakeDataViewGroups()
	require.NoError(t, err)
	require.Len(t, groups, 3)

	f := &database.Filters{FilterObj: filter.Eq{Key: "name", Cond: 1, Value: pbtypes.String("three")}}
	err = grouper.InitGroups(f)
	require.NoError(t, err)
	groups, err = grouper.MakeDataViewGroups()
	require.NoError(t, err)
	require.Len(t, groups, 2) // because results should always contain an option with empty tags set
}
