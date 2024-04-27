package cache

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
)

const CName = "cache"

var log = logger.NewNamed(CName)

var (
	ErrCacheDbNotInitialized   = errors.New("cache db is not initialized")
	ErrCacheDbError            = errors.New("cache db error")
	ErrUnsupportedCacheVersion = errors.New("unsupported cache version")
	ErrCacheDisabled           = errors.New("cache is disabled")
	ErrCacheExpired            = errors.New("cache is empty")
)

// once you change the cache format, you need to update this variable
// it will cause cache to be dropped and recreated
const LAST_CACHE_VERSION = 5

var dbKey = "payments/subscription/v" + strconv.Itoa(LAST_CACHE_VERSION)

type StorageStruct struct {
	// not to migrate old storage to new format, but just to check the validity of the cache
	// if format changes - usually we just want to drop the cache and create new one
	// see dbKey above
	CurrentVersion uint16

	// this variable is just for info
	LastUpdated time.Time

	// depending on the type of the subscription the cache will have different lifetime
	// if current time is >= ExpireTime -> cache is expired
	ExpireTime time.Time

	// if this is 0 - then cache is enabled
	DisableUntilTime time.Time

	// actual data
	SubscriptionStatus pb.RpcMembershipGetStatusResponse
	TiersData          pb.RpcMembershipGetTiersResponse
}

func newStorageStruct() *StorageStruct {
	return &StorageStruct{
		CurrentVersion:   LAST_CACHE_VERSION,
		LastUpdated:      time.Now().UTC(),
		ExpireTime:       time.Time{},
		DisableUntilTime: time.Time{},
		SubscriptionStatus: pb.RpcMembershipGetStatusResponse{
			Data: nil,
		},
		TiersData: pb.RpcMembershipGetTiersResponse{
			Tiers: nil,
		},
	}
}

type CacheService interface {
	// if cache is disabled -> will return objects and ErrCacheDisabled
	// if cache is expired -> will return objects and ErrCacheExpired
	CacheGet() (status *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, err error)

	// if cache is disabled -> will return no error
	// if cache is expired -> will return no error
	// status or tiers can be nil depending on what you want to update
	CacheSet(status *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, ExpireTime time.Time) (err error)

	IsCacheEnabled() (enabled bool)

	// if already enabled -> will not return error
	CacheEnable() (err error)

	// if already disabled -> will not return error
	// if currently disabled -> will disable GETs for next N minutes
	CacheDisableForNextMinutes(minutes int) (err error)

	// does not take into account if cache is enabled or not, erases always
	CacheClear() (err error)

	app.Component
}

func New() CacheService {
	return &cacheservice{}
}

type cacheservice struct {
	dbProvider datastore.Datastore
	db         *badger.DB

	m sync.Mutex
}

func (s *cacheservice) Name() (name string) {
	return CName
}

func (s *cacheservice) Init(a *app.App) (err error) {
	s.dbProvider = app.MustComponent[datastore.Datastore](a)

	db, err := s.dbProvider.LocalStorage()
	if err != nil {
		return err
	}
	s.db = db
	return nil
}

func (s *cacheservice) Run(_ context.Context) (err error) {
	return nil
}

func (s *cacheservice) Close(_ context.Context) (err error) {
	return s.db.Close()
}

func (s *cacheservice) CacheGet() (status *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, err error) {
	// 1 - check in storage
	ss, err := s.get()
	if err != nil {
		log.Error("can not get subscription status from cache", zap.Error(err))
		return nil, nil, ErrCacheDbError
	}

	if ss.CurrentVersion != LAST_CACHE_VERSION {
		// currently we have only one version, but in future we can have more
		// this error can happen if you "downgrade" the app
		log.Error("unsupported cache version", zap.Uint16("version", ss.CurrentVersion))
		return nil, nil, ErrUnsupportedCacheVersion
	}

	// 2 - check if cache is disabled
	if !s.IsCacheEnabled() {
		// return object too
		return &ss.SubscriptionStatus, &ss.TiersData, ErrCacheDisabled
	}

	// 3 - check if cache is outdated
	if time.Now().UTC().After(ss.ExpireTime) {
		// return object too
		return &ss.SubscriptionStatus, &ss.TiersData, ErrCacheExpired
	}

	// 4 - return value
	return &ss.SubscriptionStatus, &ss.TiersData, nil
}

func (s *cacheservice) CacheSet(status *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, expireTime time.Time) (err error) {
	// 1 - get existing storage
	ss, err := s.get()
	if err != nil {
		// if there is no record in the cache, let's create it
		ss = newStorageStruct()
	}

	// 2 - update storage
	if status != nil {
		ss.SubscriptionStatus = *status
	}

	if tiers != nil {
		ss.TiersData = *tiers
	}

	ss.ExpireTime = expireTime

	// 3 - save to storage
	return s.set(ss)
}

func (s *cacheservice) IsCacheEnabled() (enabled bool) {
	// 1 - get existing storage
	ss, err := s.get()
	if err != nil {
		return true
	}

	// 2 - check if cache is disabled
	if (ss.DisableUntilTime != time.Time{}) && time.Now().UTC().Before(ss.DisableUntilTime) {
		return false
	}

	return true
}

// will not return error if already enabled
func (s *cacheservice) CacheEnable() (err error) {
	// 1 - get existing storage
	ss, err := s.get()
	if err != nil {
		// if there is no record in the cache, let's create it
		ss = newStorageStruct()
	}

	// 2 - update storage
	ss.DisableUntilTime = time.Time{}

	// 3 - save to storage
	err = s.set(ss)
	if err != nil {
		return ErrCacheDbError
	}
	return nil
}

// will not return error if already disabled
// if currently disabled - will disable for next N minutes
func (s *cacheservice) CacheDisableForNextMinutes(minutes int) (err error) {
	// 1 - get existing storage
	ss, err := s.get()
	if err != nil {
		// if there is no record in the cache, let's create it
		ss = newStorageStruct()
	}

	// 2 - update storage
	ss.DisableUntilTime = time.Now().UTC().Add(time.Minute * time.Duration(minutes))

	// 3 - save to storage
	err = s.set(ss)
	if err != nil {
		return ErrCacheDbError
	}
	return nil
}

// does not take into account if cache is enabled or not, erases always
func (s *cacheservice) CacheClear() (err error) {
	// 1 - get existing storage
	_, err = s.get()
	if err != nil {
		// no error if there is no record in the cache
		return nil
	}

	// 2 - update storage
	ss := newStorageStruct()

	// 3 - save to storage
	err = s.set(ss)
	if err != nil {
		return ErrCacheDbError
	}
	return nil
}

func (s *cacheservice) get() (out *StorageStruct, err error) {
	if s.db == nil {
		return nil, ErrCacheDbNotInitialized
	}

	s.m.Lock()
	defer s.m.Unlock()

	var ss StorageStruct
	err = s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(dbKey))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			// convert value to out
			return json.Unmarshal(val, &ss)
		})
	})

	out = &ss
	return out, err
}

func (s *cacheservice) set(in *StorageStruct) (err error) {
	s.m.Lock()
	defer s.m.Unlock()

	return s.db.Update(func(txn *badger.Txn) error {
		// convert
		bytes, err := json.Marshal(*in)
		if err != nil {
			return err
		}

		return txn.Set([]byte(dbKey), bytes)
	})
}
