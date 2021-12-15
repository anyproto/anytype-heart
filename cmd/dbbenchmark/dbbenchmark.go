package main

import (
	"flag"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore/clientds"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	dsbadgerv3 "github.com/anytypeio/go-ds-badger3"
	"github.com/gogo/protobuf/types"
	dsbadgerv1 "github.com/ipfs/go-ds-badger"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

const localstoreDir string = "localstore"
const objectType string = "_otobject_type"

type options struct {
	isV3 bool
	sync bool
	path string
}

func (o *options) withDatastoreVersion(isV3 bool) *options {
	o.isV3 = isV3
	return o
}

func (o *options) withDatastorePath(path string) *options {
	o.path = path
	return o
}

func initObjecStore(o *options) (os objectstore.ObjectStore, closer func(), err error) {
	var ds datastore.DSTxnBatching
	if o.isV3 {
		ds, err = initBadgerV3(o)
		closer = func() {
			ds.Close()
		}
	} else {
		ds, err = initBadgerV1(o)
		closer = func() {
			ds.Close()
		}
	}
	if err != nil {
		return
	}

	return objectstore.NewWithLocalstore(ds), closer, nil
}

func initBadgerV3(o *options) (*dsbadgerv3.Datastore, error) {
	cfg := clientds.DefaultConfig.Localstore
	cfg.SyncWrites = o.sync
	localstoreDS, err := dsbadgerv3.NewDatastore(filepath.Join(o.path, localstoreDir), &cfg)
	if err != nil {
		return nil, err
	}
	return localstoreDS, nil
}

func initBadgerV1(o *options) (*dsbadgerv1.Datastore, error) {
	cfg := clientds.DefaultConfig.Logstore
	cfg.SyncWrites = o.sync
	localstoreDS, err := dsbadgerv1.NewDatastore(filepath.Join(o.path, localstoreDir), &cfg)
	if err != nil {
		return nil, err
	}
	return localstoreDS, nil
}

var (
	detailsCount   = flag.Int("det_count", 10, "the number of details of each object")
	relationsCount = flag.Int("rel_count", 10, "the number of relations of each object")
	sync           = flag.Bool("s", false, "sync mode")
	path           = flag.String("p", "", "path to localstore")
	isV3           = flag.Bool("isv3", true, "are we using badger v3")
	keys           = flag.Int("keys", 100000, "the number of different keys to be used")
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6
	letterIdxMask = 1<<letterIdxBits - 1
	letterIdxMax  = 63 / letterIdxBits
)

func randString(n int) string {
	b := make([]byte, n)
	for i, cache, remain := n-1, rand.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = rand.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

func genRandomIds(count, size int) []string {
	buf := make([]string, 0, count)
	for i := 0; i < count; i++ {
		// we use _anytype_profile, so it will deem this as profile id and not check the string
		// in SmartBlockTypeFromID
		buf = append(buf, "_anytype_profile"+randString(size))
	}
	return buf
}

func genRandomDetails(strings []string, count int) *types.Struct {
	f := make(map[string]*types.Value)
	min := count
	if count > len(strings) {
		min = len(strings)
	}
	for i := 0; i < min; i++ {
		f[strings[i]] = pbtypes.String(randString(60))
	}
	f[bundle.RelationKeySetOf.String()] = pbtypes.String(objectType)
	return &types.Struct{
		Fields: f,
	}
}

func genRandomRelations(strings []string, count int) *model.Relations {
	var rels []*model.Relation
	min := count
	if count > len(strings) {
		min = len(strings)
	}
	for i := 0; i < min; i++ {
		rels = append(rels, []*model.Relation{
			{
				Key:          strings[i],
				Format:       model.RelationFormat_status,
				Name:         randString(60),
				DefaultValue: nil,
				SelectDict: []*model.RelationOption{
					{"id1", "option1", "red", model.RelationOption_local},
					{"id2", "option2", "red", model.RelationOption_local},
					{"id3", "option3", "red", model.RelationOption_local},
				},
			},
			{
				Key:          strings[i][:len(strings[i])-1],
				Format:       model.RelationFormat_shorttext,
				Name:         randString(60),
				DefaultValue: nil,
			},
		}...)
	}
	return &model.Relations{Relations: rels}
}

func createObjects(store objectstore.ObjectStore, ids []string, detailsCount int, relationsCount int) error {
	avg := float32(0)
	i := float32(0)
	for _, id := range ids {
		details := genRandomDetails(ids, detailsCount)
		relations := genRandomRelations(ids, relationsCount)
		start := time.Now()
		err := store.CreateObject(id, details, relations, nil, "snippet")
		if err != nil {
			fmt.Println("error occurred while updating object store:", err.Error())
			return err
		}
		taken := float32(time.Now().Sub(start).Nanoseconds())
		avg = (avg*i + taken) / (i + 1)
		i += 1.0
	}
	fmt.Println("avg create operation time ms", avg/1000000)
	return nil
}

func updateDetails(store objectstore.ObjectStore, ids []string, detailsCount int, relationsCount int) error {
	avg := float32(0)
	i := float32(0)
	creatorId := genRandomIds(1, 60)[0]
	for _, id := range ids {
		details := genRandomDetails(ids, detailsCount)
		relations := genRandomRelations(ids, relationsCount)
		start := time.Now()
		err := store.UpdateObjectDetails(id, details, relations, false)
		if err != nil {
			fmt.Println("error occurred while updating object store:", err.Error())
			return err
		}
		err = store.UpdateRelationsInSetByObjectType(id, objectType, creatorId, relations.Relations)
		if err != nil {
			fmt.Println("updating relationships failed", err.Error())
			return err
		}
		taken := float32(time.Now().Sub(start).Nanoseconds())
		avg = (avg*i + taken) / (i + 1)
		i += 1.0
	}
	fmt.Println("avg update operation time ms", avg/1000000)
	return nil
}

func main() {
	// go run dbbenchmark.go -p localstore -keys 3000 -det_count 200 -rel_count 10 -isv3 false -s false
	// this should be read as total keys 3000, entries in details struct - 200, entries in relations - 10
	// using badger v3 - false, sync writes - false
	flag.Parse()
	if *path == "" {
		flag.PrintDefaults()
		return
	}
	os.RemoveAll(*path)
	o := &options{
		isV3: *isV3,
		path: *path,
		sync: *sync,
	}
	store, closeDb, err := initObjecStore(o)
	if err != nil {
		fmt.Println("error occurred when opening object store", err.Error())
		return
	}
	defer closeDb()
	ids := genRandomIds(*keys, 64)
	err = createObjects(store, ids, *detailsCount, *relationsCount)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = updateDetails(store, ids, *detailsCount, *relationsCount)
	if err != nil {
		fmt.Println(err)
		return
	}
}
