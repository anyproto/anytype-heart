package identity

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/identityrepo/identityrepoproto"
	"github.com/anyproto/any-sync/nameservice/nameserviceclient"
	"github.com/anyproto/any-sync/nameservice/nameserviceproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/dgraph-io/badger/v4"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileacl"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

const CName = "identity"

var (
	log = logging.Logger("anytype-identity").Desugar()
)

type Service interface {
	GetMyProfileDetails(ctx context.Context) (identity string, metadataKey crypto.SymKey, details *types.Struct)

	UpdateGlobalNames(myIdentityGlobalName string)

	RegisterIdentity(spaceId string, identity string, encryptionKey crypto.SymKey, observer func(identity string, profile *model.IdentityProfile)) error

	// UnregisterIdentity removes the observer for the identity in specified space
	UnregisterIdentity(spaceId string, identity string)
	// UnregisterIdentitiesInSpace removes all identity observers in the space
	UnregisterIdentitiesInSpace(spaceId string)

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
	dbProvider             datastore.Datastore
	db                     *badger.DB
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
	globalNamesForceUpdate   chan struct{}

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
		globalNamesForceUpdate:   make(chan struct{}),
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
	s.dbProvider = app.MustComponent[datastore.Datastore](a)
	s.namingService = app.MustComponent[nameserviceclient.AnyNsClientService](a)

	objectStore := app.MustComponent[objectstore.ObjectStore](a)
	spaceService := app.MustComponent[space.Service](a)

	s.ownProfileSubscription = newOwnProfileSubscription(spaceService, objectStore, s.accountService, s.identityRepoClient, s.fileAclService, s, s.pushIdentityBatchTimeout)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) (err error) {
	s.db, err = s.dbProvider.LocalStorage()
	if err != nil {
		return err
	}

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

func (s *service) UpdateGlobalNames(myIdentityGlobalName string) {
	// we update globalName of local identity directly because Naming Node is not registering new name immediately
	s.updateMyIdentityGlobalName(myIdentityGlobalName)
	select {
	case s.globalNamesForceUpdate <- struct{}{}:
	default:
	}
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

	ctx := context.Background()
	observe := func(globalNamesForceUpdate bool) {
		err := s.observeIdentities(ctx, globalNamesForceUpdate)
		if err != nil {
			log.Error("error observing identities", zap.Error(err))
		}
	}
	for {
		select {
		case <-s.componentCtx.Done():
			return
		case <-s.identityForceUpdate:
			observe(false)
		case <-s.globalNamesForceUpdate:
			observe(true)
		case <-ticker.C:
			observe(false)
		}
	}
}

const identityRepoDataKind = "profile"

func (s *service) observeIdentities(ctx context.Context, globalNamesForceUpdate bool) error {
	identities := s.listRegisteredIdentities()

	identitiesData, err := s.getIdentitiesDataFromRepo(ctx, identities)
	if err != nil {
		return fmt.Errorf("failed to pull identity: %w", err)
	}

	if err = s.fetchGlobalNames(append(identities, s.myIdentity), globalNamesForceUpdate); err != nil {
		log.Error("error fetching identities global names from Naming Service", zap.Error(err))
	}

	for _, identityData := range identitiesData {
		err := s.broadcastIdentityProfile(identityData)
		if err != nil {
			log.Error("error handling identity data", zap.Error(err))
		}
	}
	return nil
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
	if len(identities) == 0 {
		return nil, nil
	}
	res, err := s.identityRepoClient.IdentityRepoGet(ctx, identities, []string{identityRepoDataKind})
	if err == nil {
		return res, nil
	}
	log.Info("get identities data from remote repo", zap.Error(err))

	res = make([]*identityrepoproto.DataWithIdentity, 0, len(identities))
	err = s.db.View(func(txn *badger.Txn) error {
		for _, identity := range identities {
			rawData, err := badgerhelper.GetValueTxn(txn, makeIdentityProfileKey(identity), badgerhelper.UnmarshalBytes)
			if badgerhelper.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
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
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get identities data from local cache: %w", err)
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
	hasUpdates := !ok || !proto.Equal(prevProfile, profile)

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

		return badgerhelper.SetValue(s.db, makeIdentityProfileKey(profile.Identity), rawProfile)
	}

	return nil
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

func (s *service) fetchGlobalNames(identities []string, forceUpdate bool) error {
	s.lock.Lock()
	if len(s.identityGlobalNames) == len(identities) && !forceUpdate {
		s.lock.Unlock()
		return nil
	}
	s.lock.Unlock()

	response, err := s.namingService.BatchGetNameByAnyId(context.Background(), &nameserviceproto.BatchNameByAnyIdRequest{AnyAddresses: identities})
	if err != nil {
		return err
	}
	if response == nil {
		return nil
	}
	for i, anyID := range identities {
		s.lock.Lock()
		s.identityGlobalNames[anyID] = response.Results[i]
		s.lock.Unlock()
		if anyID == s.myIdentity && response.Results[i].Found {
			s.updateMyIdentityGlobalName(response.Results[i].Name)
		}
	}
	return nil
}

func (s *service) updateMyIdentityGlobalName(name string) {
	s.ownProfileSubscription.updateGlobalName(name)
}

func makeIdentityProfileKey(identity string) []byte {
	return []byte("/identity_profile/" + identity)
}

func (s *service) RegisterIdentity(spaceId string, identity string, encryptionKey crypto.SymKey, observerCallback func(identity string, profile *model.IdentityProfile)) error {
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

	if obs, ok := observers[spaceId]; ok {
		obs.callback = observerCallback
	} else {
		s.identityObservers[identity][spaceId] = &observer{
			callback:    observerCallback,
			initialized: false,
		}
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

func (s *service) GetMyProfileDetails(ctx context.Context) (identity string, metadataKey crypto.SymKey, details *types.Struct) {
	return s.ownProfileSubscription.getDetails(ctx)
}
