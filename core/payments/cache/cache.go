package cache

/*
AI generated

Name: Membership Data Cache
Scope: global

## Responsibility
- Caches membership status and tier data to reduce network calls to payment node
- Supports V1 (membership/tiers) and V2 (membership/products) data formats
- Calculates expiration based on membership tier (Explorer=24h, others=10min)

## External State
- System collection: PaymentCacheKey (V1), PaymentCacheV2Key (V2) - versioned to auto-invalidate on format change
- V2 entries also persist per-resource "fetched" markers and last success times
  (MetaVersion 1); older entries are migrated on read without dropping the cache
*/

import (
	"context"
	"errors"
	"time"

	anystore "github.com/anyproto/any-store"
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

	// MetaVersion is 0 for entries written before the fetched markers existed
	// (legacy); they are migrated on read, see migrateLegacyV2. The key stays
	// the same, so no cache is dropped on upgrade.
	MetaVersion uint16 `json:",omitempty"`
	// StatusFetched/ProductsFetched mark a resource that was successfully
	// fetched at least once (an empty result included), as opposed to the
	// empty placeholder written when only the other resource was stored
	StatusFetched   bool `json:",omitempty"`
	ProductsFetched bool `json:",omitempty"`
	// zero = unknown (legacy data)
	StatusLastSuccessAt   time.Time `json:",omitempty"`
	ProductsLastSuccessAt time.Time `json:",omitempty"`
}

const storageV2MetaVersion = 1

// V2Snapshot is the persisted V2 state of both resources
type V2Snapshot struct {
	Status              *model.MembershipV2Data
	StatusFetched       bool
	StatusLastSuccessAt time.Time

	Products              []*model.MembershipV2Product
	ProductsFetched       bool
	ProductsLastSuccessAt time.Time

	ExpireTime time.Time
}

// V2Commit is a field-scoped write: only resources with Set* = true are
// written (an empty result included), the other one is left untouched.
type V2Commit struct {
	SetStatus bool
	Status    *model.MembershipV2Data

	SetProducts bool
	Products    []*model.MembershipV2Product

	// SuccessAt is stored as the last success time of every written resource
	SuccessAt time.Time
	// RenewExpiry renews the shared expiry; without it the expiry is kept
	RenewExpiry bool
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

	// CacheV2Load returns the persisted V2 state. A missing entry is an empty
	// snapshot (nothing fetched), not an error. ctx bounds the wait for the
	// cache lock and the read.
	CacheV2Load(ctx context.Context) (snapshot V2Snapshot, err error)
	// CacheV2Commit atomically writes only the resources set in c; ctx bounds
	// the wait for the cache lock, the read and the write
	CacheV2Commit(ctx context.Context, c V2Commit) (err error)

	app.Component
}

func New() CacheService {
	return &cacheservice{m: make(chan struct{}, 1)}
}

func (s *cacheservice) lock() {
	s.m <- struct{}{}
}

// lockCtx acquires the cache lock unless ctx ends first
func (s *cacheservice) lockCtx(ctx context.Context) error {
	select {
	case s.m <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *cacheservice) unlock() {
	<-s.m
}

type cacheservice struct {
	db   keyvaluestore.Store[*StorageStruct]
	dbV2 keyvaluestore.Store[*StorageV2Struct]

	// m is a ctx-aware mutex (capacity-1 semaphore): V2 callers hold the
	// payments state lock while they wait for it, so their wait is bounded
	m chan struct{}
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
	s.lock()
	defer s.unlock()

	s.db = nil
	s.dbV2 = nil
	return nil
}

func (s *cacheservice) CacheGet() (status *model.Membership, tiers []*model.MembershipTierData, expiration time.Time, err error) {
	s.lock()
	defer s.unlock()

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
	s.lock()
	defer s.unlock()

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
	s.lock()
	defer s.unlock()

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

func (s *cacheservice) getV2(ctx context.Context) (out *StorageV2Struct, err error) {
	if s.dbV2 == nil {
		return nil, ErrCacheDbNotInitialized
	}
	return s.dbV2.Get(ctx, anystoreprovider.SystemKeys.PaymentCacheV2Key(cacheV2LastVersion))
}

func (s *cacheservice) setV2(ctx context.Context, in *StorageV2Struct) (err error) {
	if s.dbV2 == nil {
		return ErrCacheDbNotInitialized
	}
	return s.dbV2.Set(ctx, anystoreprovider.SystemKeys.PaymentCacheV2Key(cacheV2LastVersion), in)
}

func getExpireTimeV2() time.Time {
	// Use standard 10 minute cache lifetime for V2
	return time.Now().UTC().Add(cacheLifetimeDurOther)
}

// CacheV2Lifetime is how long a successful V2 fetch counts as fresh
func CacheV2Lifetime() time.Duration {
	return cacheLifetimeDurOther
}

func (s *cacheservice) CacheV2Load(ctx context.Context) (snapshot V2Snapshot, err error) {
	if err = s.lockCtx(ctx); err != nil {
		return V2Snapshot{}, err
	}
	defer s.unlock()

	ss, err := s.getV2(ctx)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return V2Snapshot{}, nil
	}
	if err != nil {
		log.Error("can not get membership V2 from cache", zap.Error(err))
		return V2Snapshot{}, ErrCacheDbError
	}
	if ss.CurrentVersion != cacheV2LastVersion {
		log.Error("unsupported V2 cache version", zap.Uint16("version", ss.CurrentVersion))
		return V2Snapshot{}, ErrUnsupportedCacheVersion
	}
	migrateLegacyV2(ss)

	return V2Snapshot{
		Status:                ss.V2Data,
		StatusFetched:         ss.StatusFetched,
		StatusLastSuccessAt:   ss.StatusLastSuccessAt,
		Products:              ss.ProductsData,
		ProductsFetched:       ss.ProductsFetched,
		ProductsLastSuccessAt: ss.ProductsLastSuccessAt,
		ExpireTime:            ss.ExpireTime,
	}, nil
}

func (s *cacheservice) CacheV2Commit(ctx context.Context, c V2Commit) (err error) {
	if err = s.lockCtx(ctx); err != nil {
		return err
	}
	defer s.unlock()

	ss, err := s.getV2(ctx)
	switch {
	case errors.Is(err, anystore.ErrDocNotFound):
		ss = newStorageV2Struct()
	case err != nil:
		// never replace the other resource with a placeholder because of a
		// read failure
		return err
	case ss.CurrentVersion != cacheV2LastVersion:
		ss = newStorageV2Struct()
	default:
		migrateLegacyV2(ss)
	}
	ss.MetaVersion = storageV2MetaVersion

	if c.SetStatus {
		ss.V2Data = c.Status
		if ss.V2Data == nil {
			ss.V2Data = &model.MembershipV2Data{}
		}
		ss.StatusFetched = true
		ss.StatusLastSuccessAt = c.SuccessAt.UTC()
	}
	if c.SetProducts {
		ss.ProductsData = c.Products
		if ss.ProductsData == nil {
			ss.ProductsData = []*model.MembershipV2Product{}
		}
		ss.ProductsFetched = true
		ss.ProductsLastSuccessAt = c.SuccessAt.UTC()
	}
	if c.RenewExpiry {
		ss.ExpireTime = getExpireTimeV2()
	}

	return s.setV2(ctx, ss)
}

// migrateLegacyV2 derives the fetched markers for an entry written before
// they existed. Conservative: a resource counts as fetched only if it has
// actual content, so the placeholder status written along with products (and
// a genuinely empty fetch) stays "never fetched". Its age is unknown: the last
// success time stays zero and is never derived from the shared expiry.
func migrateLegacyV2(ss *StorageV2Struct) {
	if ss.MetaVersion != 0 {
		return
	}
	ss.StatusFetched = v2DataHasContent(ss.V2Data)
	ss.ProductsFetched = len(ss.ProductsData) > 0
	ss.StatusLastSuccessAt = time.Time{}
	ss.ProductsLastSuccessAt = time.Time{}
}

func v2DataHasContent(d *model.MembershipV2Data) bool {
	if d == nil {
		return false
	}
	return len(d.Products) > 0 || d.NextInvoice != nil || d.TeamOwnerID != "" || d.PaymentProvider != model.MembershipV2_None
}
