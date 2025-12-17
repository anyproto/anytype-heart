package identity

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/identityrepo/identityrepoproto"
	"github.com/anyproto/any-sync/nameservice/nameserviceclient"
	"github.com/anyproto/any-sync/nameservice/nameserviceproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/proto"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileacl"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/conc"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

const CName = "identity"
const identityBatch = 100

var (
	log = logging.Logger("anytype-identity").Desugar()
)

type Service interface {
	GetMyProfileDetails(ctx context.Context) (identity string, metadataKey crypto.SymKey, details *domain.Details)

	UpdateOwnGlobalName(myIdentityGlobalName string)

	RegisterIdentity(spaceId string, identity string, encryptionKey crypto.SymKey, observer func(identity string, profile *model.IdentityProfile)) error

	// UnregisterIdentity removes the observer for the identity in specified space
	UnregisterIdentity(spaceId string, identity string)
	// UnregisterIdentitiesInSpace removes all identity observers in the space
	UnregisterIdentitiesInSpace(spaceId string)
	WaitProfile(ctx context.Context, identity string) *model.IdentityProfile
	WaitProfileWithKey(ctx context.Context, identity string) (*model.IdentityProfileWithKey, error)
	GetMetadataKey(identity string) (crypto.SymKey, error)
	AddIdentityProfile(identityProfile *model.IdentityProfile, key crypto.SymKey) error
	app.ComponentRunnable
}

type observer struct {
	callback    func(identity string, profile *model.IdentityProfile)
	initialized bool
}

type identityRepoClient interface {
	app.Component
	IdentityRepoPut(ctx context.Context, identity string, data []*identityrepoproto.Data) (err error)
	IdentityRepoGet(ctx context.Context, identities []string, kinds []string) (res []*identityrepoproto.DataWithIdentity, err error)
}

type service struct {
	ownProfileSubscription *ownProfileSubscription
	accountService         account.Service
	identityRepoClient     identityRepoClient
	fileAclService         fileacl.Service
	namingService          nameserviceclient.AnyNsClientService

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc

	myIdentity               string
	pushIdentityBatchTimeout time.Duration
	identityObservePeriod    time.Duration
	identityForceUpdate      chan struct{}

	identityProfileCacheStore    keyvaluestore.Store[[]byte]
	identityGlobalNameCacheStore keyvaluestore.Store[string]

	lock sync.Mutex
	// identity => spaceId => observer
	identityObservers      map[string]map[string]*observer
	identityEncryptionKeys map[string]crypto.SymKey
	identityProfileCache   map[string]*model.IdentityProfile
	identityGlobalNames    map[string]*nameserviceproto.NameByAddressResponse
}

func New(identityObservePeriod time.Duration, pushIdentityBatchTimeout time.Duration) Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &service{
		componentCtx:             ctx,
		componentCtxCancel:       cancel,
		identityForceUpdate:      make(chan struct{}),
		identityObservePeriod:    identityObservePeriod,
		identityObservers:        make(map[string]map[string]*observer),
		identityEncryptionKeys:   make(map[string]crypto.SymKey),
		identityProfileCache:     make(map[string]*model.IdentityProfile),
		identityGlobalNames:      make(map[string]*nameserviceproto.NameByAddressResponse),
		pushIdentityBatchTimeout: pushIdentityBatchTimeout,
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.accountService = app.MustComponent[account.Service](a)
	s.identityRepoClient = app.MustComponent[identityRepoClient](a)
	s.fileAclService = app.MustComponent[fileacl.Service](a)
	s.namingService = app.MustComponent[nameserviceclient.AnyNsClientService](a)

	objectStore := app.MustComponent[objectstore.ObjectStore](a)
	spaceService := app.MustComponent[space.Service](a)

	provider := app.MustComponent[anystoreprovider.Provider](a)

	s.identityProfileCacheStore, err = keyvaluestore.New(provider.GetCommonDb(), "identity_profile", keyvaluestore.BytesMarshal, keyvaluestore.BytesUnmarshal)
	if err != nil {
		return fmt.Errorf("init identity profile cache store: %w", err)
	}
	s.identityGlobalNameCacheStore, err = keyvaluestore.New(provider.GetCommonDb(), "global_name", keyvaluestore.StringMarshal, keyvaluestore.StringUnmarshal)
	if err != nil {
		return fmt.Errorf("init global name cache store: %w", err)
	}

	s.ownProfileSubscription = newOwnProfileSubscription(
		spaceService, objectStore, s.accountService, s.identityRepoClient,
		s.fileAclService, s, s.namingService, s.pushIdentityBatchTimeout,
		s.identityGlobalNameCacheStore, s.identityProfileCacheStore,
	)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) (err error) {
	s.myIdentity = s.accountService.AccountID()

	err = s.ownProfileSubscription.run(ctx)
	if err != nil {
		return err
	}

	go s.observeIdentitiesLoop()

	return
}

func (s *service) Close(ctx context.Context) (err error) {
	s.componentCtxCancel()
	s.ownProfileSubscription.close()
	return nil
}

func (s *service) UpdateOwnGlobalName(myIdentityGlobalName string) {
	// we update globalName of local identity directly because Naming Node is not registering new name immediately
	s.ownProfileSubscription.updateGlobalName(myIdentityGlobalName)
}

func (s *service) WaitProfile(ctx context.Context, identity string) *model.IdentityProfile {
	profile := s.getProfileFromCache(identity)
	if profile != nil {
		return profile
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.componentCtx.Done():
			return nil
		case <-ticker.C:
			profile = s.getProfileFromCache(identity)
			if profile != nil {
				return profile
			}
		}
	}
}
func (s *service) GetMetadataKey(identity string) (crypto.SymKey, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	key, ok := s.identityEncryptionKeys[identity]
	if !ok {
		// FIXME We have a race condition somewhere and our own key could not be indexed yet at this moment.
		// Derive a key as a temporarily solution
		if s.myIdentity == identity {
			_, key, err := domain.DeriveAccountMetadata(s.accountService.Keys().SignKey)
			if err != nil {
				return nil, err
			}
			s.identityEncryptionKeys[identity] = key
			return key, nil
		}
		return nil, fmt.Errorf("identityEncryptionKey doesnt exist for identity")
	}

	return key, nil
}

func (s *service) WaitProfileWithKey(ctx context.Context, identity string) (*model.IdentityProfileWithKey, error) {
	profile := s.WaitProfile(ctx, identity)
	if profile == nil {
		return nil, fmt.Errorf("wait profile: got nil profile")
	}
	key, err := s.GetMetadataKey(identity)
	if err != nil {
		return nil, err
	}

	keyBytes, err := key.Marshall()
	if err != nil {
		return nil, err
	}

	return &model.IdentityProfileWithKey{
		IdentityProfile: profile,
		RequestMetadata: keyBytes,
	}, nil
}

func (s *service) getProfileFromCache(identity string) *model.IdentityProfile {
	s.lock.Lock()
	defer s.lock.Unlock()
	if profile, ok := s.identityProfileCache[identity]; ok {
		return profile
	}
	return nil
}

func (s *service) observeIdentitiesLoop() {
	ticker := time.NewTicker(s.identityObservePeriod)
	defer ticker.Stop()

	observe := func() {
		err := s.observeIdentities(s.componentCtx)
		if err != nil {
			log.Error("error observing identities", zap.Error(err))
		}
	}
	for {
		select {
		case <-s.componentCtx.Done():
			return
		case <-s.identityForceUpdate:
			ticker.Reset(s.identityObservePeriod)
			observe()
		case <-ticker.C:
			observe()
		}
	}
}

const identityRepoDataKind = "profile"

func (s *service) observeIdentities(ctx context.Context) error {
	identities := s.listRegisteredIdentities()
	allIdentitiesData := s.getIdentityData(ctx, identities)

	if err := s.fetchGlobalNames(identities); err != nil {
		log.Error("error fetching global names of guest identities from Naming Service", zap.Error(err))
	}

	for _, identityData := range allIdentitiesData {
		err := s.broadcastIdentityProfile(identityData)
		if err != nil {
			log.Error("error handling identity data", zap.Error(err))
		}
	}
	return nil
}

func (s *service) getIdentityData(ctx context.Context, identities []string) []*identityrepoproto.DataWithIdentity {
	batches := lo.Chunk(identities, identityBatch)
	allIdentitiesData, err := conc.MapErr(batches, func(batch []string) ([]*identityrepoproto.DataWithIdentity, error) {
		return s.getIdentitiesDataFromRepo(ctx, batch)
	})
	if err != nil {
		log.Error("failed to pull identity", zap.Error(err))
	}
	return lo.Flatten(allIdentitiesData)
}

func (s *service) listRegisteredIdentities() []string {
	s.lock.Lock()
	defer s.lock.Unlock()

	if len(s.identityObservers) == 0 {
		return nil
	}
	identities := make([]string, 0, len(s.identityObservers)-1)
	for identity := range s.identityObservers {
		if identity == s.myIdentity {
			continue
		}
		identities = append(identities, identity)
	}
	return identities
}

func (s *service) getIdentitiesDataFromRepo(ctx context.Context, identities []string) ([]*identityrepoproto.DataWithIdentity, error) {
	res, err := s.identityRepoClient.IdentityRepoGet(ctx, identities, []string{identityRepoDataKind})
	if err != nil {
		return s.processFailedIdentities(res, identities)
	}
	return res, nil
}

func (s *service) processFailedIdentities(res []*identityrepoproto.DataWithIdentity, failedIdentities []string) ([]*identityrepoproto.DataWithIdentity, error) {
	for _, identity := range failedIdentities {
		rawData, err := s.identityProfileCacheStore.Get(context.Background(), identity)
		if errors.Is(err, anystore.ErrDocNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("get: %w", err)
		}
		res = append(res, &identityrepoproto.DataWithIdentity{
			Identity: identity,
			Data: []*identityrepoproto.Data{
				{
					Kind: identityRepoDataKind,
					Data: rawData,
				},
			},
		})
	}
	return res, nil
}

func (s *service) indexIconImage(profile *model.IdentityProfile) error {
	if len(profile.IconEncryptionKeys) > 0 {
		return s.fileAclService.StoreFileKeys(domain.FileId(profile.IconCid), profile.IconEncryptionKeys)
	}
	return nil
}

func (s *service) broadcastIdentityProfile(identityData *identityrepoproto.DataWithIdentity) error {
	profile, rawProfile, err := s.findProfile(identityData)
	if err != nil {
		return fmt.Errorf("find profile: %w", err)
	}

	s.lock.Lock()
	if globalName, found := s.identityGlobalNames[identityData.Identity]; found && globalName.Found {
		profile.GlobalName = globalName.Name
	}

	prevProfile, ok := s.identityProfileCache[identityData.Identity]
	hasUpdates := !ok || !prevProfile.Equal(profile)

	observers := s.identityObservers[identityData.Identity]
	for _, obs := range observers {
		// Run callback at least once for each observer
		if !obs.initialized {
			obs.initialized = true
			obs.callback(identityData.Identity, profile)
		} else if hasUpdates {
			obs.callback(identityData.Identity, profile)
		}
	}
	s.identityProfileCache[profile.Identity] = profile
	s.lock.Unlock()

	if hasUpdates {
		err := s.indexIconImage(profile)
		if err != nil {
			return fmt.Errorf("index icon image: %w", err)
		}

		return s.identityProfileCacheStore.Set(context.Background(), profile.Identity, rawProfile)
	}

	return nil
}

// AddIdentityProfile puts identity profile to cache from external place (e.g. from onetoone inbox).
// Returns immediately if key already exists.
func (s *service) AddIdentityProfile(profile *model.IdentityProfile, key crypto.SymKey) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	if _, ok := s.identityEncryptionKeys[profile.Identity]; ok {
		log.Info("addIdentityProfile: profile key already exists, skip", zap.String("identity", profile.Identity))
		return nil
	}

	profileBytes, err := proto.Marshal(profile)
	if err != nil {
		return err
	}

	encryptedProfileBytes, err := key.Encrypt(profileBytes)
	if err != nil {
		return err
	}

	s.identityEncryptionKeys[profile.Identity] = key

	err = s.indexIconImage(profile)
	if err != nil {
		log.Error("addIdentityProfile: index icon error", zap.Error(err))
	}

	return s.identityProfileCacheStore.Set(context.Background(), profile.Identity, encryptedProfileBytes)
}

func (s *service) broadcastMyIdentityProfile(identityProfile *model.IdentityProfile) {
	s.lock.Lock()
	defer s.lock.Unlock()
	observers, ok := s.identityObservers[s.myIdentity]
	if ok {
		for _, obs := range observers {
			obs.callback(s.myIdentity, identityProfile)
		}
	}
}

func (s *service) findProfile(identityData *identityrepoproto.DataWithIdentity) (profile *model.IdentityProfile, rawProfile []byte, err error) {
	s.lock.Lock()
	key := s.identityEncryptionKeys[identityData.Identity]
	s.lock.Unlock()

	return extractProfile(identityData, key)
}

func extractProfile(identityData *identityrepoproto.DataWithIdentity, symKey crypto.SymKey) (profile *model.IdentityProfile, rawData []byte, err error) {
	for _, data := range identityData.Data {
		if data.Kind == identityRepoDataKind {
			rawData = data.Data
			rawProfile, err := symKey.Decrypt(data.Data)
			if err != nil {
				return nil, nil, fmt.Errorf("decrypt identity profile: %w", err)
			}
			profile = new(model.IdentityProfile)
			err = proto.Unmarshal(rawProfile, profile)
			if err != nil {
				return nil, nil, fmt.Errorf("unmarshal identity profile: %w", err)
			}
		}
	}
	if profile == nil {
		return nil, nil, fmt.Errorf("no profile data found")
	}
	return profile, rawData, nil
}

func (s *service) fetchGlobalNames(identities []string) error {
	s.lock.Lock()
	if len(s.identityGlobalNames) == len(identities) {
		s.lock.Unlock()
		return nil
	}
	s.lock.Unlock()

	response, err := s.namingService.BatchGetNameByAnyId(s.componentCtx, &nameserviceproto.BatchNameByAnyIdRequest{AnyAddresses: identities})
	if err != nil {
		return err
	}
	if response == nil {
		return nil
	}
	for i, identity := range identities {
		result := response.Results[i]
		s.lock.Lock()
		s.identityGlobalNames[identity] = result
		s.lock.Unlock()

		err := s.identityGlobalNameCacheStore.Set(context.Background(), identity, result.Name)
		if err != nil {
			log.Error("save global name", zap.String("identity", identity), zap.Error(err))
		}
	}
	return nil
}

func makeIdentityProfileKey(identity string) []byte {
	return []byte("/identity_profile/" + identity)
}

func makeGlobalNameKey(identity string) []byte {
	return []byte("/identity_global_name/" + identity)
}

func (s *service) getCachedIdentityProfile(identity string) (*identityrepoproto.DataWithIdentity, error) {
	rawData, err := s.identityProfileCacheStore.Get(context.Background(), identity)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &identityrepoproto.DataWithIdentity{
		Identity: identity,
		Data: []*identityrepoproto.Data{
			{
				Kind: identityRepoDataKind,
				Data: rawData,
			},
		},
	}, nil
}

func (s *service) getCachedGlobalName(identity string) (string, error) {
	rawData, err := s.identityGlobalNameCacheStore.Get(context.Background(), identity)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return rawData, nil
}

func (s *service) RegisterIdentity(spaceId string, identity string, encryptionKey crypto.SymKey, observerCallback func(identity string, profile *model.IdentityProfile)) error {
	cachedProfile, err := s.getCachedIdentityProfile(identity)
	if err != nil {
		log.Warn("register identity: get cached profile", zap.Error(err))
	}
	cachedGlobalName, err := s.getCachedGlobalName(identity)
	if err != nil {
		log.Warn("register identity: get cached global name", zap.Error(err))
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if key, ok := s.identityEncryptionKeys[identity]; ok {
		if !key.Equals(encryptionKey) {
			return fmt.Errorf("encryption key for identity %s already exists and do not match new key", identity)
		}
	} else {
		s.identityEncryptionKeys[identity] = encryptionKey
	}

	observers := s.identityObservers[identity]
	if observers == nil {
		observers = make(map[string]*observer)
		s.identityObservers[identity] = observers
	}

	var isInitialized bool
	if cachedProfile != nil {
		profile, _, err := extractProfile(cachedProfile, encryptionKey)
		if err == nil {
			if cachedGlobalName != "" {
				profile.GlobalName = cachedGlobalName
			}
			s.identityProfileCache[identity] = profile
			observerCallback(identity, profile)
			isInitialized = true
		} else {
			log.Warn("register identity: extract profile", zap.Error(err))
		}
	}

	if obs, ok := observers[spaceId]; ok {
		obs.callback = observerCallback
	} else {
		s.identityObservers[identity][spaceId] = &observer{
			callback:    observerCallback,
			initialized: isInitialized,
		}
	}

	if identity == s.myIdentity {
		ownProfile := s.ownProfileSubscription.prepareIdentityProfile()
		observerCallback(identity, ownProfile)
		return nil
	}

	select {
	case s.identityForceUpdate <- struct{}{}:
	default:
	}
	return nil
}

func (s *service) UnregisterIdentity(spaceId string, identity string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	observers := s.identityObservers[identity]
	if observers == nil {
		return
	}
	delete(observers, spaceId)
}

func (s *service) UnregisterIdentitiesInSpace(spaceId string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for _, observers := range s.identityObservers {
		delete(observers, spaceId)
	}
}

func (s *service) GetMyProfileDetails(ctx context.Context) (identity string, metadataKey crypto.SymKey, details *domain.Details) {
	return s.ownProfileSubscription.getDetails(ctx)
}
