package cache

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "cache"

var log = logger.NewNamed(CName)

var (
	ErrCacheDbError = errors.New("cache db error")

	ErrUnsupportedCacheVersion = errors.New("unsupported cache version")
	ErrCacheDisabled           = errors.New("cache is disabled")
	ErrCacheExpired            = errors.New("cache is empty")
)

const dbKey = "payments/subscription/v1"

type StorageStructV1 struct {
	// to migrate old storage to new format
	CurrentVersion uint16

	// this variable is just for info
	LastUpdated time.Time

	// depending on the type of the subscription the cache will have different lifetime
	// if current time is >= ExpireTime -> cache is expired
	ExpireTime time.Time

	// if this is 0 - then cache is enabled
	DisableUntilTime time.Time

	// v1 of the actual data
	SubscriptionStatus pb.RpcMembershipGetStatusResponse
}

func newStorageStructV1() *StorageStructV1 {
	return &StorageStructV1{
		CurrentVersion:   1,
		LastUpdated:      time.Now().UTC(),
		ExpireTime:       time.Time{},
		DisableUntilTime: time.Time{},
		SubscriptionStatus: pb.RpcMembershipGetStatusResponse{
			// empty struct, but non-nil Data field
			Data: &model.Membership{},
		},
	}
}

type CacheService interface {
	// if cache is disabled -> will return object and ErrCacheDisabled
	// if cache is expired -> will return ErrCacheExpired
	CacheGet() (out *pb.RpcMembershipGetStatusResponse, err error)

	// if cache is disabled -> will return no error
	// if cache is expired -> will return no error
	CacheSet(in *pb.RpcMembershipGetStatusResponse, ExpireTime time.Time) (err error)

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

func (s *cacheservice) CacheGet() (out *pb.RpcMembershipGetStatusResponse, err error) {
	// 1 - check in storage
	ss, err := s.get()
	if err != nil {
		log.Error("can not get subscription status from cache", zap.Error(err))
		// do not translate error here!
		return nil, ErrCacheExpired
	}

	if ss.CurrentVersion != 1 {
		// currently we have only one version, but in future we can have more
		// this error can happen if you "downgrade" the app
		log.Error("unsupported cache version", zap.Uint16("version", ss.CurrentVersion))
		return nil, ErrUnsupportedCacheVersion
	}

	// 2 - check if cache is disabled
	if !s.IsCacheEnabled() {
		// return object too
		return &ss.SubscriptionStatus, ErrCacheDisabled
	}

	// 3 - check if cache is outdated
	if time.Now().UTC().After(ss.ExpireTime) {
		return nil, ErrCacheExpired
	}

	// 4 - return value
	return &ss.SubscriptionStatus, nil
}

func (s *cacheservice) CacheSet(in *pb.RpcMembershipGetStatusResponse, expireTime time.Time) (err error) {
	// 1 - get existing storage
	ss, err := s.get()
	if err != nil {
		// if there is no record in the cache, let's create it
		ss = newStorageStructV1()
	}

	// 2 - update storage
	ss.SubscriptionStatus = *in
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
		ss = newStorageStructV1()
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
		ss = newStorageStructV1()
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
	ss := newStorageStructV1()

	// 3 - save to storage
	err = s.set(ss)
	if err != nil {
		return ErrCacheDbError
	}
	return nil
}

func (s *cacheservice) get() (out *StorageStructV1, err error) {
	if s.db == nil {
		return nil, errors.New("db is not initialized")
	}

	s.m.Lock()
	defer s.m.Unlock()

	var ss StorageStructV1
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

func (s *cacheservice) set(in *StorageStructV1) (err error) {
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
