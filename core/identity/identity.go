package identity

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/identityrepo/identityrepoproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/dgraph-io/badger/v4"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
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
	RegisterIdentity(spaceId string, identity string, encryptionKey crypto.SymKey, observer func(identity string, profile *model.IdentityProfile)) error

	// UnregisterIdentity removes the observer for the identity in specified space
	UnregisterIdentity(spaceId string, identity string)
	// UnregisterIdentitiesInSpace removes all identity observers in the space
	UnregisterIdentitiesInSpace(spaceId string)

	// GetDetails returns the last store details of the identity and provides a way to receive updates via updateHook
	GetDetails(ctx context.Context, identity string) (details *types.Struct, err error)
	// SpaceId returns the spaceId used to store the identities in the objectStore
	SpaceId() string
	app.ComponentRunnable
}

type DetailsModifier interface {
	ModifyDetails(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error)
}

type spaceIdDeriver interface {
	DeriveID(ctx context.Context, spaceType string) (id string, err error)
}

type observer struct {
	callback    func(identity string, profile *model.IdentityProfile)
	initialized bool
}

type service struct {
	dbProvider        datastore.Datastore
	db                *badger.DB
	spaceService      space.Service
	objectStore       objectstore.ObjectStore
	accountService    account.Service
	spaceIdDeriver    spaceIdDeriver
	detailsModifier   DetailsModifier
	coordinatorClient coordinatorclient.CoordinatorClient
	fileService       files.Service
	fileObjectService fileobject.Service
	fileStore         filestore.FileStore
	closing           chan struct{}
	identities        []string
	techSpaceId       string
	personalSpaceId   string

	identityObservePeriod time.Duration
	identityForceUpdate   chan struct{}
	lock                  sync.RWMutex
	// identity => spaceId => observer
	identityObservers      map[string]map[string]*observer
	identityEncryptionKeys map[string]crypto.SymKey
	identityProfileCache   map[string]*model.IdentityProfile
}

func New(identityObservePeriod time.Duration) Service {
	return &service{
		closing:                make(chan struct{}),
		identityForceUpdate:    make(chan struct{}),
		identityObservePeriod:  identityObservePeriod,
		identityObservers:      make(map[string]map[string]*observer),
		identityEncryptionKeys: make(map[string]crypto.SymKey),
		identityProfileCache:   make(map[string]*model.IdentityProfile),
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.accountService = app.MustComponent[account.Service](a)
	s.spaceIdDeriver = app.MustComponent[spaceIdDeriver](a)
	s.detailsModifier = app.MustComponent[DetailsModifier](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.coordinatorClient = app.MustComponent[coordinatorclient.CoordinatorClient](a)
	s.fileService = app.MustComponent[files.Service](a)
	s.fileObjectService = app.MustComponent[fileobject.Service](a)
	s.fileStore = app.MustComponent[filestore.FileStore](a)
	s.dbProvider = app.MustComponent[datastore.Datastore](a)
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

	err = s.indexIdentityObject(ctx)
	if err != nil {
		return err
	}

	err = s.runLocalProfileSubscriptions(ctx)
	if err != nil {
		return err
	}

	go s.observeIdentitiesLoop()

	return
}

func (s *service) indexIdentityObject(ctx context.Context) error {
	// Index profile
	techSpace, err := s.spaceService.Get(ctx, s.techSpaceId)
	if err != nil {
		return fmt.Errorf("get tech space: %w", err)
	}
	err = techSpace.Do(s.accountService.IdentityObjectId(), func(_ smartblock.SmartBlock) error {
		return nil
	})
	if err != nil {
		return fmt.Errorf("touch profile to index: %w", err)
	}
	return nil
}

func (s *service) Close(ctx context.Context) (err error) {
	close(s.closing)
	close(s.identityForceUpdate)
	return nil
}

func (s *service) SpaceId() string {
	return s.techSpaceId
}

func (s *service) GetDetails(ctx context.Context, profileId string) (details *types.Struct, err error) {
	rec, err := s.objectStore.GetDetails(profileId)
	if err != nil {
		return nil, err
	}

	return rec.Details, nil
}

func getDetailsFromProfile(id, spaceId string, details *types.Struct) *types.Struct {
	name := pbtypes.GetString(details, bundle.RelationKeyName.String())
	description := pbtypes.GetString(details, bundle.RelationKeyDescription.String())
	image := pbtypes.GetString(details, bundle.RelationKeyIconImage.String())
	profileId := pbtypes.GetString(details, bundle.RelationKeyId.String())
	d := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():                pbtypes.String(name),
		bundle.RelationKeyDescription.String():         pbtypes.String(description),
		bundle.RelationKeyId.String():                  pbtypes.String(id),
		bundle.RelationKeyIsReadonly.String():          pbtypes.Bool(true),
		bundle.RelationKeyIsArchived.String():          pbtypes.Bool(false),
		bundle.RelationKeyIsHidden.String():            pbtypes.Bool(false),
		bundle.RelationKeySpaceId.String():             pbtypes.String(spaceId),
		bundle.RelationKeyType.String():                pbtypes.String(bundle.TypeKeyProfile.BundledURL()),
		bundle.RelationKeyIdentityProfileLink.String(): pbtypes.String(profileId),
		bundle.RelationKeyLayout.String():              pbtypes.Float64(float64(model.ObjectType_profile)),
		bundle.RelationKeyLastModifiedBy.String():      pbtypes.String(id),
	}}

	if image != "" {
		d.Fields[bundle.RelationKeyIconImage.String()] = pbtypes.String(image)
	}

	// deprecated, but we have existing profiles which use this, so let's it be up for clients to decide either to render it or not
	iconOption := pbtypes.Get(details, bundle.RelationKeyIconOption.String())
	if iconOption != nil {
		d.Fields[bundle.RelationKeyIconOption.String()] = iconOption
	}

	return d
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

func (s *service) updateIdentityObject(profileDetails *types.Struct) error {
	identityObjectId := s.accountService.IdentityObjectId()
	details := getDetailsFromProfile(identityObjectId, s.techSpaceId, profileDetails)
	err := s.detailsModifier.ModifyDetails(identityObjectId, func(current *types.Struct) (*types.Struct, error) {
		return pbtypes.StructMerge(current, details, false), nil
	})
	if err != nil {
		return fmt.Errorf("modify details: %w", err)
	}

	err = s.pushProfileToIdentityRegistry(context.Background(), profileDetails)
	if err != nil {
		return fmt.Errorf("push profile to identity registry: %w", err)
	}
	return nil
}

func (s *service) prepareIconImageInfo(ctx context.Context, iconImageObjectId string) (iconCid string, iconEncryptionKeys []*model.IdentityProfileEncryptionKey, err error) {
	if iconImageObjectId == "" {
		return "", nil, nil
	}
	fileId, err := s.fileObjectService.GetFileIdFromObject(ctx, iconImageObjectId)
	if err != nil {
		return "", nil, fmt.Errorf("get file id from object: %w", err)
	}
	iconCid = fileId.FileId.String()
	keys, err := s.fileService.FileGetKeys(fileId)
	if err != nil {
		return "", nil, fmt.Errorf("get file keys: %w", err)
	}
	for path, key := range keys.EncryptionKeys {
		iconEncryptionKeys = append(iconEncryptionKeys, &model.IdentityProfileEncryptionKey{
			Path: path,
			Key:  key,
		})
	}
	sort.Slice(iconEncryptionKeys, func(i, j int) bool {
		return iconEncryptionKeys[i].Path < iconEncryptionKeys[j].Path
	})
	return iconCid, iconEncryptionKeys, nil
}

func (s *service) pushProfileToIdentityRegistry(ctx context.Context, profileDetails *types.Struct) error {
	iconImageObjectId := pbtypes.GetString(profileDetails, bundle.RelationKeyIconImage.String())
	iconCid, iconEncryptionKeys, err := s.prepareIconImageInfo(ctx, iconImageObjectId)
	if err != nil {
		return fmt.Errorf("prepare icon image info: %w", err)
	}

	identity := s.accountService.AccountID()
	identityProfile := &model.IdentityProfile{
		Identity:           identity,
		Name:               pbtypes.GetString(profileDetails, bundle.RelationKeyName.String()),
		IconCid:            iconCid,
		IconEncryptionKeys: iconEncryptionKeys,
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
	err = s.coordinatorClient.IdentityRepoPut(ctx, identity, []*identityrepoproto.Data{
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
	observe := func() {
		err := s.observeIdentities(ctx)
		if err != nil {
			log.Error("error observing identities", zap.Error(err))
		}
	}
	for {
		select {
		case <-s.closing:
			return
		case <-s.identityForceUpdate:
			observe()
		case <-ticker.C:
			observe()
		}
	}
}

const identityRepoDataKind = "profile"

// TODO Maybe we need to use backoff in case of error from coordinator
func (s *service) observeIdentities(ctx context.Context) error {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if len(s.identityObservers) == 0 {
		return nil
	}
	identities := make([]string, 0, len(s.identityObservers))
	for identity := range s.identityObservers {
		identities = append(identities, identity)
	}
	identitiesData, err := s.getIdentitiesDataFromRepo(ctx, identities)
	if err != nil {
		return fmt.Errorf("failed to pull identity: %w", err)
	}

	for _, identityData := range identitiesData {
		err := s.handleIdentityData(identityData)
		if err != nil {
			log.Error("error handling identity data", zap.Error(err))
		}
	}
	return nil
}

func (s *service) getIdentitiesDataFromRepo(ctx context.Context, identities []string) ([]*identityrepoproto.DataWithIdentity, error) {
	res, err := s.coordinatorClient.IdentityRepoGet(ctx, identities, []string{identityRepoDataKind})
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
		// TODO Garbage collect old icons
		keys := domain.FileEncryptionKeys{
			FileId:         domain.FileId(profile.IconCid),
			EncryptionKeys: map[string]string{},
		}
		for _, key := range profile.IconEncryptionKeys {
			keys.EncryptionKeys[key.Path] = key.Key
		}
		err := s.fileStore.AddFileKeys(keys)
		if err != nil {
			return fmt.Errorf("store icon encryption keys: %w", err)
		}
	}
	return nil
}

func (s *service) handleIdentityData(identityData *identityrepoproto.DataWithIdentity) error {
	var (
		rawProfile []byte
		profile    *model.IdentityProfile
	)
	for _, data := range identityData.Data {
		if data.Kind == identityRepoDataKind {
			rawProfile = data.Data
			symKey := s.identityEncryptionKeys[identityData.Identity]
			rawProfile, err := symKey.Decrypt(data.Data)
			if err != nil {
				return fmt.Errorf("decrypt identity profile: %w", err)
			}
			profile = new(model.IdentityProfile)
			err = proto.Unmarshal(rawProfile, profile)
			if err != nil {
				return fmt.Errorf("unmarshal identity profile: %w", err)
			}
		}
	}
	if profile == nil {
		return fmt.Errorf("no profile data found")
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
	s.identityForceUpdate <- struct{}{}
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
