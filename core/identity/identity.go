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
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "identity"

var (
	log = logging.Logger("anytype-identity").Desugar()
)

type Service interface {
	GetMyProfileDetails() (identity string, metadataKey crypto.SymKey, details *types.Struct)

	UpdateIdentities()

	RegisterIdentity(spaceId string, identity string, encryptionKey crypto.SymKey, observer func(identity string, profile *model.IdentityProfile)) error

	// UnregisterIdentity removes the observer for the identity in specified space
	UnregisterIdentity(spaceId string, identity string)
	// UnregisterIdentitiesInSpace removes all identity observers in the space
	UnregisterIdentitiesInSpace(spaceId string)

	app.ComponentRunnable
}

type spaceIdDeriver interface {
	DeriveID(ctx context.Context, spaceType string) (id string, err error)
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
	dbProvider         datastore.Datastore
	db                 *badger.DB
	spaceService       space.Service
	objectStore        objectstore.ObjectStore
	accountService     account.Service
	spaceIdDeriver     spaceIdDeriver
	identityRepoClient identityRepoClient
	fileAclService     fileacl.Service
	wallet             wallet.Wallet
	namingService      nameserviceclient.AnyNsClientService
	closing            chan struct{}
	startedCh          chan struct{}
	techSpaceId        string
	personalSpaceId    string

	myIdentity                string
	currentProfileDetailsLock sync.RWMutex
	currentProfileDetails     *types.Struct // save details to batch update operation
	pushIdentityTimer         *time.Timer   // timer for batching
	pushIdentityBatchTimeout  time.Duration

	identityObservePeriod time.Duration
	identityForceUpdate   chan struct{}
	lock                  sync.RWMutex
	// identity => spaceId => observer
	identityObservers      map[string]map[string]*observer
	identityEncryptionKeys map[string]crypto.SymKey
	identityProfileCache   map[string]*model.IdentityProfile
	identityGlobalNames    map[string]string
}

func New(identityObservePeriod time.Duration, pushIdentityBatchTimeout time.Duration) Service {
	return &service{
		startedCh:                make(chan struct{}),
		closing:                  make(chan struct{}),
		identityForceUpdate:      make(chan struct{}),
		identityObservePeriod:    identityObservePeriod,
		identityObservers:        make(map[string]map[string]*observer),
		identityEncryptionKeys:   make(map[string]crypto.SymKey),
		identityProfileCache:     make(map[string]*model.IdentityProfile),
		pushIdentityBatchTimeout: pushIdentityBatchTimeout,
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.accountService = app.MustComponent[account.Service](a)
	s.spaceIdDeriver = app.MustComponent[spaceIdDeriver](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.identityRepoClient = app.MustComponent[identityRepoClient](a)
	s.fileAclService = app.MustComponent[fileacl.Service](a)
	s.dbProvider = app.MustComponent[datastore.Datastore](a)
	s.wallet = app.MustComponent[wallet.Wallet](a)
	s.namingService = app.MustComponent[nameserviceclient.AnyNsClientService](a)
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
	s.techSpaceId, err = s.spaceIdDeriver.DeriveID(ctx, spacecore.TechSpaceType)
	if err != nil {
		return err
	}

	s.personalSpaceId, err = s.spaceIdDeriver.DeriveID(ctx, spacecore.SpaceType)
	if err != nil {
		return err
	}

	s.myIdentity = s.accountService.AccountID()

	err = s.runLocalProfileSubscriptions(ctx)
	if err != nil {
		return err
	}

	go s.observeIdentitiesLoop()

	return
}

func (s *service) Close(ctx context.Context) (err error) {
	close(s.closing)
	return nil
}

func (s *service) runLocalProfileSubscriptions(ctx context.Context) (err error) {
	uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeProfilePage, "")
	if err != nil {
		return err
	}
	personalSpace, err := s.spaceService.GetPersonalSpace(ctx)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	profileObjectId, err := personalSpace.DeriveObjectID(ctx, uniqueKey)
	if err != nil {
		return err
	}

	recordsCh := make(chan *types.Struct)
	sub := database.NewSubscription(nil, recordsCh)

	var (
		records  []database.Record
		closeSub func()
	)

	records, closeSub, err = s.objectStore.QueryByIDAndSubscribeForChanges([]string{profileObjectId}, sub)
	if err != nil {
		return err
	}
	go func() {
		select {
		case <-s.closing:
			closeSub()
			return
		}
	}()

	if len(records) > 0 {
		err := s.updateIdentityObject(records[0].Details)
		if err != nil {
			log.Error("error updating identity object", zap.Error(err))
		}
	}

	go func() {
		for {
			rec, ok := <-recordsCh
			if !ok {
				return
			}
			err := s.updateIdentityObject(rec)
			if err != nil {
				log.Error("error updating identity object", zap.Error(err))
			}
		}
	}()

	return nil
}

func (s *service) GetMyProfileDetails() (identity string, metadataKey crypto.SymKey, details *types.Struct) {
	<-s.startedCh
	s.currentProfileDetailsLock.RLock()
	defer s.currentProfileDetailsLock.RUnlock()

	return s.myIdentity, s.spaceService.AccountMetadataSymKey(), s.currentProfileDetails
}

func (s *service) UpdateIdentities() {
	select {
	case s.identityForceUpdate <- struct{}{}:
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
		case <-s.closing:
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
	s.lock.RLock()
	if profile, ok := s.identityProfileCache[identity]; ok {
		s.lock.RUnlock()
		return profile
	}
	s.lock.RUnlock()
	return nil
}

func (s *service) updateIdentityObject(profileDetails *types.Struct) error {
	s.cacheProfileDetails(profileDetails)
	if s.pushIdentityTimer == nil {
		s.pushIdentityTimer = time.AfterFunc(0, func() {
			pushErr := s.pushProfileToIdentityRegistry(context.Background())
			if pushErr != nil {
				log.Error("push profile to identity registry", zap.Error(pushErr))
			}
		})
	} else {
		s.pushIdentityTimer.Reset(s.pushIdentityBatchTimeout)
	}

	return nil
}

func (s *service) cacheProfileDetails(details *types.Struct) {
	if details == nil {
		return
	}
	s.currentProfileDetailsLock.Lock()
	if s.currentProfileDetails == nil {
		close(s.startedCh)
	}
	s.currentProfileDetails = details
	s.currentProfileDetailsLock.Unlock()

	identityProfile := &model.IdentityProfile{
		Identity:    s.myIdentity,
		Name:        pbtypes.GetString(details, bundle.RelationKeyName.String()),
		Description: pbtypes.GetString(details, bundle.RelationKeyDescription.String()),
		IconCid:     pbtypes.GetString(details, bundle.RelationKeyIconImage.String()),
		GlobalName:  pbtypes.GetString(details, bundle.RelationKeyGlobalName.String()),
	}

	s.lock.RLock()
	defer s.lock.RUnlock()
	observers, ok := s.identityObservers[s.myIdentity]
	if ok {
		for _, obs := range observers {
			obs.callback(s.myIdentity, identityProfile)
		}
	}
}

func (s *service) prepareIconImageInfo(ctx context.Context, iconImageObjectId string) (iconCid string, iconEncryptionKeys []*model.FileEncryptionKey, err error) {
	if iconImageObjectId == "" {
		return "", nil, nil
	}
	return s.fileAclService.GetInfoForFileSharing(ctx, iconImageObjectId)
}

func (s *service) pushProfileToIdentityRegistry(ctx context.Context) error {
	s.currentProfileDetailsLock.RLock()
	defer s.currentProfileDetailsLock.RUnlock()

	iconImageObjectId := pbtypes.GetString(s.currentProfileDetails, bundle.RelationKeyIconImage.String())
	iconCid, iconEncryptionKeys, err := s.prepareIconImageInfo(ctx, iconImageObjectId)
	if err != nil {
		return fmt.Errorf("prepare icon image info: %w", err)
	}

	identity := s.accountService.AccountID()
	identityProfile := &model.IdentityProfile{
		Identity:           identity,
		Name:               pbtypes.GetString(s.currentProfileDetails, bundle.RelationKeyName.String()),
		Description:        pbtypes.GetString(s.currentProfileDetails, bundle.RelationKeyDescription.String()),
		IconCid:            iconCid,
		IconEncryptionKeys: iconEncryptionKeys,
		GlobalName:         pbtypes.GetString(s.currentProfileDetails, bundle.RelationKeyGlobalName.String()),
	}
	data, err := proto.Marshal(identityProfile)
	if err != nil {
		return fmt.Errorf("marshal identity profile: %w", err)
	}

	symKey := s.spaceService.AccountMetadataSymKey()
	data, err = symKey.Encrypt(data)
	if err != nil {
		return fmt.Errorf("encrypt data: %w", err)
	}

	signature, err := s.accountService.SignData(data)
	if err != nil {
		return fmt.Errorf("failed to sign profile data: %w", err)
	}
	err = s.identityRepoClient.IdentityRepoPut(ctx, identity, []*identityrepoproto.Data{
		{
			Kind:      "profile",
			Data:      data,
			Signature: signature,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to push identity: %w", err)
	}
	return nil
}

func (s *service) observeIdentitiesLoop() {
	ticker := time.NewTicker(s.identityObservePeriod)
	defer ticker.Stop()

	ctx := context.Background()
	observe := func(forceUpdate bool) {
		err := s.observeIdentities(ctx, forceUpdate)
		if err != nil {
			log.Error("error observing identities", zap.Error(err))
		}
	}
	for {
		select {
		case <-s.closing:
			return
		case <-s.identityForceUpdate:
			observe(true)
		case <-ticker.C:
			observe(false)
		}
	}
}

const identityRepoDataKind = "profile"

// TODO Maybe we need to use backoff in case of error from coordinator
func (s *service) observeIdentities(ctx context.Context, globalNamesForceUpdate bool) error {
	s.lock.RLock()
	defer s.lock.RUnlock()

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

func (s *service) getIdentitiesDataFromRepo(ctx context.Context, identities []string) ([]*identityrepoproto.DataWithIdentity, error) {
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

	if globalName, found := s.identityGlobalNames[identityData.Identity]; found {
		profile.GlobalName = globalName
	}

	prevProfile, ok := s.identityProfileCache[identityData.Identity]
	hasUpdates := !ok || !proto.Equal(prevProfile, profile)

	if hasUpdates {
		err := s.indexIconImage(profile)
		if err != nil {
			return fmt.Errorf("index icon image: %w", err)
		}
		err = s.cacheIdentityProfile(rawProfile, profile)
		if err != nil {
			return fmt.Errorf("put identity profile: %w", err)
		}
	}

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
	return nil
}

func (s *service) findProfile(identityData *identityrepoproto.DataWithIdentity) (profile *model.IdentityProfile, rawProfile []byte, err error) {
	for _, data := range identityData.Data {
		if data.Kind == identityRepoDataKind {
			rawProfile = data.Data
			symKey := s.identityEncryptionKeys[identityData.Identity]
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
	return profile, rawProfile, nil
}

func (s *service) fetchGlobalNames(identities []string, forceUpdate bool) error {
	if s.identityGlobalNames != nil && !forceUpdate {
		return nil
	}
	response, err := s.namingService.BatchGetNameByAnyId(context.Background(), &nameserviceproto.BatchNameByAnyIdRequest{AnyAddresses: identities})
	if err != nil {
		return err
	}
	if response == nil {
		return nil
	}
	s.identityGlobalNames = make(map[string]string, len(identities))
	for i, anyID := range identities {
		if !response.Results[i].Found {
			continue
		}
		if anyID == s.myIdentity {
			s.currentProfileDetailsLock.RLock()
			details := pbtypes.CopyStruct(s.currentProfileDetails)
			s.currentProfileDetailsLock.RUnlock()
			details.Fields[bundle.RelationKeyGlobalName.String()] = pbtypes.String(response.Results[i].Name)
			if err = s.objectStore.UpdateObjectDetails(pbtypes.GetString(details, bundle.RelationKeyId.String()), details); err != nil {
				return err
			}
		} else {
			s.identityGlobalNames[anyID] = response.Results[i].Name
		}
	}
	return nil
}

func (s *service) cacheIdentityProfile(rawProfile []byte, profile *model.IdentityProfile) error {
	s.identityProfileCache[profile.Identity] = profile
	return badgerhelper.SetValue(s.db, makeIdentityProfileKey(profile.Identity), rawProfile)
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
