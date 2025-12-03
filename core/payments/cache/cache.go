package cache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	proto "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

const CName = "cache"

var log = logger.NewNamed(CName)

var (
	ErrCacheDbNotInitialized   = errors.New("cache db is not initialized")
	ErrCacheDbError            = errors.New("cache db error")
	ErrUnsupportedCacheVersion = errors.New("unsupported cache version")
)

// once you change the cache format, you need to update this variable
// it will cause cache to be dropped and recreated
const cacheLastVersion = 8
const cacheV2LastVersion = 1

const (
	cacheLifetimeDurExplorer = 24 * time.Hour
	cacheLifetimeDurOther    = 10 * time.Minute
)

type StorageStruct struct {
	// not to migrate old storage to new format, but just to check the validity of the cache
	// if format changes - usually we just want to drop the cache and create new one
	// see dbKey above
	CurrentVersion uint16

	// depending on the type of the membership the cache will have different lifetime
	// if current time is >= ExpireTime -> cache is expired
	ExpireTime time.Time

	// actual data
	SubscriptionStatus *model.Membership
	TiersData          []*model.MembershipTierData
}

func newStorageStruct() *StorageStruct {
	return &StorageStruct{
		CurrentVersion:     cacheLastVersion,
		ExpireTime:         time.Time{},
		SubscriptionStatus: &model.Membership{},
		TiersData:          []*model.MembershipTierData{},
	}
}

type StorageV2Struct struct {
	CurrentVersion uint16
	ExpireTime     time.Time
	V2Data         *model.MembershipV2Data
	ProductsData   []*model.MembershipV2Product
}

func newStorageV2Struct() *StorageV2Struct {
	return &StorageV2Struct{
		CurrentVersion: cacheV2LastVersion,
		ExpireTime:     time.Time{},
		V2Data:         &model.MembershipV2Data{},
		ProductsData:   []*model.MembershipV2Product{},
	}
}

type CacheService interface {
	CacheGet() (status *model.Membership, tiers []*model.MembershipTierData, expireTime time.Time, err error)

	// if cache is disabled -> will return no error
	// if cache is expired -> will return no error
	// status or tiers can be nil depending on what you want to update
	CacheSet(status *model.Membership, tiers []*model.MembershipTierData) (err error)

	// does not take into account if cache is enabled or not, erases always
	CacheClear() (err error)

	CacheV2Get() (data *model.MembershipV2Data, expireTime time.Time, err error)
	CacheV2Set(data *model.MembershipV2Data) (err error)
	CacheV2ProductsGet() (products []*model.MembershipV2Product, expireTime time.Time, err error)
	CacheV2ProductsSet(products []*model.MembershipV2Product) (err error)

	app.Component
}

func New() CacheService {
	return &cacheservice{}
}

type cacheservice struct {
	db   keyvaluestore.Store[*StorageStruct]
	dbV2 keyvaluestore.Store[*StorageV2Struct]

	m sync.Mutex
}

func (s *cacheservice) Name() (name string) {
	return CName
}

func (s *cacheservice) Init(a *app.App) (err error) {
	provider := app.MustComponent[anystoreprovider.Provider](a)

	s.db = keyvaluestore.NewJsonFromCollection[*StorageStruct](provider.GetSystemCollection())
	s.dbV2 = keyvaluestore.NewJsonFromCollection[*StorageV2Struct](provider.GetSystemCollection())
	return nil
}

func (s *cacheservice) Run(_ context.Context) (err error) {
	return nil
}

func (s *cacheservice) Close(_ context.Context) (err error) {
	s.m.Lock()
	defer s.m.Unlock()

	s.db = nil
	return nil
}

func (s *cacheservice) CacheGet() (status *model.Membership, tiers []*model.MembershipTierData, expiration time.Time, err error) {
	s.m.Lock()
	defer s.m.Unlock()

	// 1 - check in storage
	ss, err := s.get()
	if err != nil {
		log.Error("can not get membership status from cache", zap.Error(err))
		return nil, nil, time.Time{}, ErrCacheDbError
	}

	if ss.CurrentVersion != cacheLastVersion {
		// currently we have only one version, but in future we can have more
		// this error can happen if you "downgrade" the app
		log.Error("unsupported cache version", zap.Uint16("version", ss.CurrentVersion))
		return nil, nil, time.Time{}, ErrUnsupportedCacheVersion
	}

	// 2 - return value
	return ss.SubscriptionStatus, ss.TiersData, ss.ExpireTime, nil
}

func getExpireTime(latestStatus *model.Membership) time.Time {
	var (
		tier     = uint32(proto.SubscriptionTier_TierUnknown)
		dateEnds = time.Unix(0, 0)
		now      = time.Now().UTC()
	)

	if latestStatus != nil {
		tier = latestStatus.Tier
		dateEnds = time.Unix(int64(latestStatus.DateEnds), 0)
	}

	if tier == uint32(proto.SubscriptionTier_TierExplorer) {
		return now.Add(cacheLifetimeDurExplorer)
	}

	// dateEnds can be 0
	isExpired := now.After(dateEnds)
	timeNext := now.Add(cacheLifetimeDurOther)

	// sub end < now OR no sub end provided (unlimited)
	if isExpired {
		log.Debug("incrementing cache lifetime because membership is isExpired")
		return timeNext
	}

	// sub end >= now
	// return min(sub end, now + timeout)
	if dateEnds.Before(timeNext) {
		log.Debug("incrementing cache lifetime because membership ends soon")
		return dateEnds
	}
	return timeNext
}

func (s *cacheservice) CacheSet(status *model.Membership, tiers []*model.MembershipTierData) (err error) {
	s.m.Lock()
	defer s.m.Unlock()

	var latestStatus *model.Membership

	// 1 - get existing storage
	ss, err := s.get()
	if err != nil {
		// if there is no record in the cache, let's create it
		ss = newStorageStruct()
	} else {
		latestStatus = ss.SubscriptionStatus
	}

	// 2 - update storage
	if status != nil {
		ss.SubscriptionStatus = status
		latestStatus = status
	}

	if tiers != nil {
		ss.TiersData = tiers
	}

	ss.ExpireTime = getExpireTime(latestStatus)

	// 3 - save to storage
	return s.set(ss)
}

// does not take into account if cache is enabled or not, erases always
func (s *cacheservice) CacheClear() (err error) {
	s.m.Lock()
	defer s.m.Unlock()

	// 1 - get existing storage
	_, err = s.get()
	if err != nil {
		// no error if there is no record in the cache
		return nil
	}

	// 2 - update storage
	ss := newStorageStruct()

	// 3 - save to storage
	return s.set(ss)
}

func (s *cacheservice) get() (out *StorageStruct, err error) {
	if s.db == nil {
		return nil, ErrCacheDbNotInitialized
	}
	return s.db.Get(context.Background(), anystoreprovider.SystemKeys.PaymentCacheKey(cacheLastVersion))
}

func (s *cacheservice) set(in *StorageStruct) (err error) {
	if s.db == nil {
		return ErrCacheDbNotInitialized
	}
	return s.db.Set(context.Background(), anystoreprovider.SystemKeys.PaymentCacheKey(cacheLastVersion), in)
}

func (s *cacheservice) getV2() (out *StorageV2Struct, err error) {
	if s.dbV2 == nil {
		return nil, ErrCacheDbNotInitialized
	}
	return s.dbV2.Get(context.Background(), anystoreprovider.SystemKeys.PaymentCacheV2Key(cacheV2LastVersion))
}

func (s *cacheservice) setV2(in *StorageV2Struct) (err error) {
	if s.dbV2 == nil {
		return ErrCacheDbNotInitialized
	}
	return s.dbV2.Set(context.Background(), anystoreprovider.SystemKeys.PaymentCacheV2Key(cacheV2LastVersion), in)
}

func getExpireTimeV2() time.Time {
	// Use standard 10 minute cache lifetime for V2
	return time.Now().UTC().Add(cacheLifetimeDurOther)
}

func (s *cacheservice) CacheV2Get() (data *model.MembershipV2Data, expiration time.Time, err error) {
	s.m.Lock()
	defer s.m.Unlock()

	// 1 - check in storage
	ss, err := s.getV2()
	if err != nil {
		log.Error("can not get membership V2 status from cache", zap.Error(err))
		return nil, time.Time{}, ErrCacheDbError
	}

	if ss.CurrentVersion != cacheV2LastVersion {
		log.Error("unsupported V2 cache version", zap.Uint16("version", ss.CurrentVersion))
		return nil, time.Time{}, ErrUnsupportedCacheVersion
	}

	// 2 - return value
	return ss.V2Data, ss.ExpireTime, nil
}

func (s *cacheservice) CacheV2Set(data *model.MembershipV2Data) (err error) {
	s.m.Lock()
	defer s.m.Unlock()

	// 1 - get existing storage
	ss, err := s.getV2()
	if err != nil {
		// if there is no record in the cache, let's create it
		ss = newStorageV2Struct()
	}

	// 2 - update storage
	if data != nil {
		ss.V2Data = data
	}

	ss.ExpireTime = getExpireTimeV2()

	// 3 - save to storage
	return s.setV2(ss)
}

func (s *cacheservice) CacheV2ProductsGet() (products []*model.MembershipV2Product, expiration time.Time, err error) {
	s.m.Lock()
	defer s.m.Unlock()

	// 1 - check in storage
	ss, err := s.getV2()
	if err != nil {
		log.Error("can not get membership V2 products from cache", zap.Error(err))
		return nil, time.Time{}, ErrCacheDbError
	}

	if ss.CurrentVersion != cacheV2LastVersion {
		log.Error("unsupported V2 cache version", zap.Uint16("version", ss.CurrentVersion))
		return nil, time.Time{}, ErrUnsupportedCacheVersion
	}

	// 2 - return value
	return ss.ProductsData, ss.ExpireTime, nil
}

func (s *cacheservice) CacheV2ProductsSet(products []*model.MembershipV2Product) (err error) {
	s.m.Lock()
	defer s.m.Unlock()

	// 1 - get existing storage
	ss, err := s.getV2()
	if err != nil {
		// if there is no record in the cache, let's create it
		ss = newStorageV2Struct()
	}

	// 2 - update storage
	if products != nil {
		ss.ProductsData = products
	}

	ss.ExpireTime = getExpireTimeV2()

	// 3 - save to storage
	return s.setV2(ss)
}
