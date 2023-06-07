package objectstore

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/app/testapp"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/clientds"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/space/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestDsObjectStore_UpdateLocalDetails(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)
	app := testapp.New()
	defer app.Close(context.Background())

	tp := typeprovider.New(nil)
	tp.Init(nil)
	ds := New(tp)
	id := bson.NewObjectId()
	tp.RegisterStaticType(id.String(), smartblock.SmartBlockTypePage)

	err := app.With(&config.DefaultConfig).With(wallet.NewWithRepoDirAndRandomKeys(tmpDir)).With(clientds.New()).With(ds).Start(context.Background())
	require.NoError(t, err)
	// bundle.RelationKeyLastOpenedDate is local relation (not stored in the changes tree)
	err = ds.UpdateObjectDetails(id.String(), &types.Struct{
		Fields: map[string]*types.Value{bundle.RelationKeyLastOpenedDate.String(): pbtypes.Int64(4), "type": pbtypes.String("_otp1")},
	})
	require.NoError(t, err)

	ot := &model.ObjectType{Url: "_otp1", Name: "otp1"}
	recs, _, err := ds.Query(schema.NewByType(ot, nil), database.Query{})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.Equal(t, pbtypes.Int64(4), pbtypes.Get(recs[0].Details, bundle.RelationKeyLastOpenedDate.String()))

	err = ds.UpdateObjectDetails(id.String(), &types.Struct{
		Fields: map[string]*types.Value{"k1": pbtypes.String("1"), "k2": pbtypes.String("2"), "type": pbtypes.String("_otp1")},
	})
	require.NoError(t, err)

	recs, _, err = ds.Query(schema.NewByType(ot, nil), database.Query{})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.Nil(t, pbtypes.Get(recs[0].Details, bundle.RelationKeyLastOpenedDate.String()))
	require.Equal(t, "2", pbtypes.GetString(recs[0].Details, "k2"))
}

func TestDsObjectStore_IndexQueue(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)

	app := testapp.New()
	defer app.Close(context.Background())

	ds := New(nil)
	err := app.With(&config.DefaultConfig).With(wallet.NewWithRepoDirAndRandomKeys(tmpDir)).With(clientds.New()).With(ds).Start(context.Background())
	require.NoError(t, err)

	t.Run("add to queue", func(t *testing.T) {
		require.NoError(t, ds.AddToIndexQueue("one"))
		require.NoError(t, ds.AddToIndexQueue("one"))
		require.NoError(t, ds.AddToIndexQueue("two"))

		ids, err := ds.ListIDsFromFullTextQueue()
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"one", "two"}, ids)
	})

	t.Run("remove from queue", func(t *testing.T) {
		ds.RemoveIDsFromFullTextQueue([]string{"one"})
		ids, err := ds.ListIDsFromFullTextQueue()
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"two"}, ids)
	})
}

func TestDsObjectStore_PrefixQuery(t *testing.T) {
	bds := sync.MutexWrap(ds.NewMapDatastore())
	err := bds.Put(context.Background(), ds.NewKey("/p1/abc/def/1"), []byte{})

	require.NoError(t, err)

	res, err := bds.Query(context.Background(), query.Query{Prefix: "/p1/abc", KeysOnly: true})
	require.NoError(t, err)

	entries, err := res.Rest()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "/p1/abc/def/1", entries[0].Key)

}

func Test_removeByPrefix(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)

	app := testapp.New()
	defer app.Close(context.Background())
	ds := New(nil)
	err := app.With(&config.DefaultConfig).With(wallet.NewWithRepoDirAndRandomKeys(tmpDir)).With(clientds.New()).With(ftsearch.New()).With(ds).Start(context.Background())
	require.NoError(t, err)

	ds2 := ds.(*dsObjectStore)
	var key = make([]byte, 32)
	for i := 0; i < 10; i++ {

		var links []string
		rand.Seed(time.Now().UnixNano())
		rand.Read(key)
		objId := fmt.Sprintf("%x", key)

		for j := 0; j < 8000; j++ {
			rand.Seed(time.Now().UnixNano())
			rand.Read(key)
			links = append(links, fmt.Sprintf("%x", key))
		}
		require.NoError(t, ds.UpdateObjectDetails(objId, nil))
		require.NoError(t, ds.UpdateObjectLinks(objId, links))
	}
	tx, err := ds2.ds.NewTransaction(false)
	_, err = removeByPrefixInTx(tx, pagesInboundLinksBase.String())
	require.NotNil(t, err)
	tx.Discard()

	got, err := removeByPrefix(ds2.ds, pagesInboundLinksBase.String())
	require.NoError(t, err)
	require.Equal(t, 10*8000, got)

	got, err = removeByPrefix(ds2.ds, pagesOutboundLinksBase.String())
	require.NoError(t, err)
	require.Equal(t, 10*8000, got)
}

func TestList(t *testing.T) {
	s := newStoreFixture(t)
	typeProvider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
	s.sbtProvider = typeProvider

	obj1 := makeObjectWithName("id1", "name1")
	err := s.UpdateObjectSnippet("id1", "snippet1")
	require.NoError(t, err)
	typeProvider.EXPECT().Type("id1").Return(smartblock.SmartBlockTypePage, nil)

	obj2 := makeObjectWithName("id2", "name2")
	typeProvider.EXPECT().Type("id2").Return(smartblock.SmartBlockTypeFile, nil)

	obj3 := makeObjectWithName("id3", "date")
	obj3[bundle.RelationKeyIsDeleted] = pbtypes.Bool(true)
	typeProvider.EXPECT().Type("id3").Return(smartblock.SmartBlockTypePage, nil)

	s.addObjects(t, []testObject{obj1, obj2, obj3})

	got, err := s.List()
	require.NoError(t, err)

	want := []*model.ObjectInfo{
		{
			Id:         "id1",
			Details:    makeDetails(obj1),
			Snippet:    "snippet1",
			ObjectType: model.SmartBlockType_Page,
		},
		{
			Id:         "id2",
			Details:    makeDetails(obj2),
			ObjectType: model.SmartBlockType_File,
		},
		// Skip deleted id3
	}

	assert.Equal(t, want, got)
}

func TestGetObjectType(t *testing.T) {
	t.Run("get bundled type", func(t *testing.T) {
		s := newStoreFixture(t)

		got, err := s.GetObjectType(bundle.TypeKeyTask.BundledURL())
		require.NoError(t, err)

		want := bundle.MustGetType(bundle.TypeKeyTask)
		assert.Equal(t, want, got)
	})

	t.Run("with object is not type expect error", func(t *testing.T) {
		s := newStoreFixture(t)

		obj := testObject{
			bundle.RelationKeyId:   pbtypes.String("id1"),
			bundle.RelationKeyType: pbtypes.String(bundle.TypeKeyNote.URL()),
		}
		s.addObjects(t, []testObject{obj})

		_, err := s.GetObjectType("id1")
		require.Error(t, err)
	})

	t.Run("with object is type", func(t *testing.T) {
		s := newStoreFixture(t)

		obj := testObject{
			bundle.RelationKeyId:                   pbtypes.String("id1"),
			bundle.RelationKeyType:                 pbtypes.String(bundle.TypeKeyObjectType.URL()),
			bundle.RelationKeyName:                 pbtypes.String("my note"),
			bundle.RelationKeyRecommendedRelations: pbtypes.StringList([]string{bundle.RelationKeyAssignee.URL()}),
			bundle.RelationKeyRecommendedLayout:    pbtypes.Int64(int64(model.ObjectType_note)),
			bundle.RelationKeyIconEmoji:            pbtypes.String("📝"),
			bundle.RelationKeyIsArchived:           pbtypes.Bool(true),
		}
		relObj := testObject{
			bundle.RelationKeyId:          pbtypes.String("id2"),
			bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyAssignee.String()),
			bundle.RelationKeyType:        pbtypes.String(bundle.TypeKeyRelation.URL()),
		}
		s.addObjects(t, []testObject{obj, relObj})

		got, err := s.GetObjectType("id1")
		require.NoError(t, err)

		want := &model.ObjectType{
			Url:        "id1",
			Name:       "my note",
			Layout:     model.ObjectType_note,
			IconEmoji:  "📝",
			IsArchived: true,
			Types:      []model.SmartBlockType{model.SmartBlockType_Page},
			RelationLinks: []*model.RelationLink{
				{
					Key:    bundle.RelationKeyAssignee.String(),
					Format: model.RelationFormat_longtext,
				},
			},
		}

		assert.Equal(t, want, got)
	})
}
