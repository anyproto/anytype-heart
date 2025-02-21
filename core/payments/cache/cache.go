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

	"github.com/anyproto/anytype-heart/pb"
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
const cacheLastVersion = 7

const (
	cacheLifetimeDurExplorer = 24 * time.Hour
	cacheLifetimeDurOther    = 10 * time.Minute
)

type StorageStruct struct {
	// not to migrate old storage to new format, but just to check the validity of the cache
	// if format changes - usually we just want to drop the cache and create new one
	// see dbKey above
	CurrentVersion uint16

	// this variable is just for info
	LastUpdated time.Time

	// depending on the type of the membership the cache will have different lifetime
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
		CurrentVersion:   cacheLastVersion,
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
	CacheGet() (status *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, err error)

	// if cache is disabled -> will return no error
	// if cache is expired -> will return no error
	// status or tiers can be nil depending on what you want to update
	CacheSet(status *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error)

	IsCacheDisabled() (disabled bool)

	IsCacheExpired() (expired bool)

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
	db keyvaluestore.Store[*StorageStruct]

	m sync.Mutex
}

func (s *cacheservice) Name() (name string) {
	return CName
}

func (s *cacheservice) Init(a *app.App) (err error) {
	provider := app.MustComponent[anystoreprovider.Provider](a)

	s.db = keyvaluestore.NewJsonFromCollection[*StorageStruct](provider.GetSystemCollection())
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

func (s *cacheservice) CacheGet() (status *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, err error) {
	s.m.Lock()
	defer s.m.Unlock()

	// 1 - check in storage
	ss, err := s.get()
	if err != nil {
		log.Error("can not get membership status from cache", zap.Error(err))
		return nil, nil, ErrCacheDbError
	}

	if ss.CurrentVersion != cacheLastVersion {
		// currently we have only one version, but in future we can have more
		// this error can happen if you "downgrade" the app
		log.Error("unsupported cache version", zap.Uint16("version", ss.CurrentVersion))
		return nil, nil, ErrUnsupportedCacheVersion
	}

	// 2 - return value
	return &ss.SubscriptionStatus, &ss.TiersData, nil
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

func (s *cacheservice) CacheSet(status *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error) {
	s.m.Lock()
	defer s.m.Unlock()

	var latestStatus *model.Membership

	// 1 - get existing storage
	ss, err := s.get()
	if err != nil {
		// if there is no record in the cache, let's create it
		ss = newStorageStruct()
	} else {
		latestStatus = ss.SubscriptionStatus.Data
	}

	// 2 - update storage
	if status != nil {
		ss.SubscriptionStatus = *status
		latestStatus = status.Data
	}

	if tiers != nil {
		ss.TiersData = *tiers
	}

	ss.ExpireTime = getExpireTime(latestStatus)

	// 3 - save to storage
	return s.set(ss)
}

func (s *cacheservice) IsCacheDisabled() (disabled bool) {
	s.m.Lock()
	defer s.m.Unlock()

	// 1 - get existing storage
	ss, err := s.get()
	if err != nil {
		return false
	}

	// 2 - check if cache is disabled
	if !ss.DisableUntilTime.IsZero() && time.Now().UTC().Before(ss.DisableUntilTime) {
		return true
	}

	return false
}

func (s *cacheservice) IsCacheExpired() (expired bool) {
	s.m.Lock()
	defer s.m.Unlock()

	// 1 - get existing storage
	ss, err := s.get()
	if err != nil {
		return true
	}

	// 2 - check if cache is outdated
	if time.Now().UTC().After(ss.ExpireTime) {
		return true
	}

	return false
}

// will not return error if already enabled
func (s *cacheservice) CacheEnable() (err error) {
	s.m.Lock()
	defer s.m.Unlock()

	// 1 - get existing storage
	ss, err := s.get()
	if err != nil {
		// if there is no record in the cache, let's create it
		ss = newStorageStruct()
	}

	// 2 - update storage
	ss.DisableUntilTime = time.Time{}

	// 3 - save to storage
	return s.set(ss)
}

// will not return error if already disabled
// if currently disabled - will disable for next N minutes
func (s *cacheservice) CacheDisableForNextMinutes(minutes int) (err error) {
	s.m.Lock()
	defer s.m.Unlock()

	// 1 - get existing storage
	ss, err := s.get()
	if err != nil {
		// if there is no record in the cache, let's create it
		ss = newStorageStruct()
	}

	// 2 - update storage
	ss.DisableUntilTime = time.Now().UTC().Add(time.Minute * time.Duration(minutes))

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
