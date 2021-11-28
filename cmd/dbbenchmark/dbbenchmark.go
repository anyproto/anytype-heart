package main

import (
	"flag"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore/clientds"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	dsbadgerv3 "github.com/anytypeio/go-ds-badger3"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"
	dsbadgerv1 "github.com/ipfs/go-ds-badger"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

const localstoreDir string = "localstore"

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
	var ds ds.TxnDatastore
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
	sync = flag.Bool("s", false, "sync mode")
	path = flag.String("p", "", "path to localstore")
	isV3 = flag.Bool("isv3", true, "are we using badger v3")
	keys = flag.Int("keys", 100000, "the number of different keys to be used")
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

func getRandomString(randomStrings []string) string {
	return randomStrings[rand.Int63()%int64(len(randomStrings))]
}

func genRandomDetails(randomStrings []string) *types.Struct {
	f := make(map[string]*types.Value)
	for _, letter := range letterBytes {
		f[string(letter)] = pbtypes.String(getRandomString(randomStrings))
	}
	f[bundle.RelationKeySetOf.String()] = pbtypes.String(objectType)
	return &types.Struct{
		Fields: f,
	}
}

func genRandomRelations(randomStrings []string) *model.Relations {
	var rels []*model.Relation
	for _, l := range letterBytes {
		rels = append(rels, []*model.Relation{
			{
				Key:          string(l) + string(l),
				Format:       model.RelationFormat_status,
				Name:         getRandomString(randomStrings),
				DefaultValue: nil,
				SelectDict: []*model.RelationOption{
					{"id1", "option1", "red", model.RelationOption_local},
					{"id2", "option2", "red", model.RelationOption_local},
					{"id3", "option3", "red", model.RelationOption_local},
				},
			},
			{
				Key:          string(l),
				Format:       model.RelationFormat_shorttext,
				Name:         getRandomString(randomStrings),
				DefaultValue: nil,
			},
		}...)
	}
	return &model.Relations{Relations: rels}
}

func createObjects(store objectstore.ObjectStore, ids []string) error {
	avg := float32(0)
	i := float32(0)
	for _, id := range ids {
		start := time.Now()
		details := genRandomDetails(ids)
		relations := genRandomRelations(ids)
		err := store.CreateObject(id, details, relations, nil, "snippet")
		if err != nil {
			fmt.Println("error occurred while updating object store:", err.Error())
			return err
		}
		taken := float32(time.Now().Sub(start).Nanoseconds())
		avg = (avg*i + taken) / (i + 1)
		i += 1.0
	}
	fmt.Println("avg create operation time ms", avg)
	return nil
}

func updateDetails(store objectstore.ObjectStore, ids []string) error {
	avg := float32(0)
	i := float32(0)
	creatorId := genRandomIds(1, 60)[0]
	for _, id := range ids {
		details := genRandomDetails(ids)
		relations := genRandomRelations(ids)
		start := time.Now()
		err := store.UpdateObjectDetails(id, details, relations, false)
		if err != nil {
			fmt.Println("error occurred while updating object store:", err.Error())
			return err
		}
		err = store.UpdateRelationsInSetByObjectType(id, objectType, creatorId, genRandomRelations(ids).Relations)
		if err != nil {
			fmt.Println("updating relationships failed", err.Error())
			return err
		}
		taken := float32(time.Now().Sub(start).Nanoseconds())
		avg = (avg*i + taken) / (i + 1)
		i += 1.0
	}
	fmt.Println("avg update operation time ms", avg)
	return nil
}

func main() {
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
	objectstore, closer, err := initObjecStore(o)
	if err != nil {
		fmt.Println("error occurred when opening object store", err.Error())
		return
	}
	defer closer()
	ids := genRandomIds(*keys, 64)
	err = createObjects(objectstore, ids)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = updateDetails(objectstore, ids)
	if err != nil {
		fmt.Println(err)
		return
	}
}
