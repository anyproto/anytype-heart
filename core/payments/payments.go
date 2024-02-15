package payments

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"
)

const CName = "payments"

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
	SubscriptionStatus pb.RpcPaymentsSubscriptionGetStatusResponse
}

func newStorageStructV1() *StorageStructV1 {
	return &StorageStructV1{
		CurrentVersion:   1,
		LastUpdated:      time.Now().UTC(),
		ExpireTime:       time.Time{},
		DisableUntilTime: time.Time{},
	}
}

type Service interface {
	// if cache is disabled -> will return object and ErrCacheDisabled
	// if cache is expired -> will return ErrCacheExpired
	CacheGet() (out *pb.RpcPaymentsSubscriptionGetStatusResponse, err error)

	// if cache is disabled -> will return no error
	// if cache is expired -> will return no error
	CacheSet(in *pb.RpcPaymentsSubscriptionGetStatusResponse, ExpireTime time.Time) (err error)

	IsCacheEnabled() (enabled bool)

	// if already enabled -> will not return error
	CacheEnable() (err error)

	// if already disabled -> will not return error
	// if currently disabled -> will disable GETs for next N minutes
	CacheDisableForNextMinutes(minutes int) (err error)

	// does not take into account if cache is enabled or not, erases always
	CacheClear() (err error)

	app.ComponentRunnable
}

func New() Service {
	return &service{}
}

type service struct {
	db *badger.DB
}

func (r *service) Name() (name string) {
	return CName
}

func (r *service) Init(a *app.App) (err error) {
	db, err := badger.Open(badger.DefaultOptions("payments_cache"))
	if err != nil {
		return err
	}
	r.db = db
	return nil
}

func (r *service) Run(_ context.Context) (err error) {
	return nil
}

func (r *service) Close(_ context.Context) (err error) {
	return r.db.Close()
}

func (r *service) CacheGet() (out *pb.RpcPaymentsSubscriptionGetStatusResponse, err error) {
	// 1 - check in storage
	ss, err := r.get()
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
	if !r.IsCacheEnabled() {
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

func (r *service) CacheSet(in *pb.RpcPaymentsSubscriptionGetStatusResponse, ExpireTime time.Time) (err error) {
	// 1 - get existing storage
	ss, err := r.get()
	if err != nil {
		// if there is no record in the cache, let's create it
		ss = newStorageStructV1()
	}

	// 2 - update storage
	ss.SubscriptionStatus = *in
	ss.ExpireTime = ExpireTime

	// 3 - save to storage
	return r.set(ss)
}

func (r *service) IsCacheEnabled() (enabled bool) {
	// 1 - get existing storage
	ss, err := r.get()
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
func (r *service) CacheEnable() (err error) {
	// 1 - get existing storage
	ss, err := r.get()
	if err != nil {
		// if there is no record in the cache, let's create it
		ss = newStorageStructV1()
	}

	// 2 - update storage
	ss.DisableUntilTime = time.Time{}

	// 3 - save to storage
	err = r.set(ss)
	if err != nil {
		return ErrCacheDbError
	}
	return nil
}

// will not return error if already disabled
// if currently disabled - will disable for next N minutes
func (r *service) CacheDisableForNextMinutes(minutes int) (err error) {
	// 1 - get existing storage
	ss, err := r.get()
	if err != nil {
		// if there is no record in the cache, let's create it
		ss = newStorageStructV1()
	}

	// 2 - update storage
	ss.DisableUntilTime = time.Now().UTC().Add(time.Minute * time.Duration(minutes))

	// 3 - save to storage
	err = r.set(ss)
	if err != nil {
		return ErrCacheDbError
	}
	return nil
}

// does not take into account if cache is enabled or not, erases always
func (r *service) CacheClear() (err error) {
	// 1 - get existing storage
	ss, err := r.get()
	if err != nil {
		// no error if there is no record in the cache
		return nil
	}

	// 2 - update storage
	ss.CurrentVersion = 1
	ss.LastUpdated = time.Now().UTC()
	ss.ExpireTime = time.Time{}
	ss.DisableUntilTime = time.Time{}
	ss.SubscriptionStatus = pb.RpcPaymentsSubscriptionGetStatusResponse{}

	// 3 - save to storage
	err = r.set(ss)
	if err != nil {
		return ErrCacheDbError
	}
	return nil
}

func (r *service) get() (out *StorageStructV1, err error) {
	if r.db == nil {
		return nil, errors.New("db is not initialized")
	}

	err = r.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(dbKey))
		if err != nil {
			return err
		}

		var data []byte
		data, err = item.ValueCopy(data)
		if err != nil {
			return err
		}

		// convert value to out
		var ss StorageStructV1
		err = json.Unmarshal(data, &ss)
		if err != nil {
			return err
		}

		out = &ss
		return nil
	})
	return
}

func (s *service) set(in *StorageStructV1) (err error) {
	return s.db.Update(func(txn *badger.Txn) error {
		// convert
		bytes, err := json.Marshal(*in)
		if err != nil {
			return err
		}

		return txn.Set([]byte(dbKey), bytes)
	})
}
