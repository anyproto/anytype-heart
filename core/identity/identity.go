package identity

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/identityrepo/identityrepoproto"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "identity"

var (
	log = logging.Logger("anytype-identity").Desugar()
)

type Service interface {
	// TODO guarantee callback call at least once
	RegisterIdentity(spaceId string, identity string, encryptionKey symmetric.Key, observer func(identity string, profile *model.IdentityProfile)) error

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

type service struct {
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
	lock                  sync.RWMutex
	// identity => spaceId => observer
	identityObservers      map[string]map[string]func(identity string, profile *model.IdentityProfile)
	identityEncryptionKeys map[string]symmetric.Key
	identityProfileCache   map[string]*model.IdentityProfile
}

func New(identityObservePeriod time.Duration) Service {
	return &service{
		identityObservePeriod:  identityObservePeriod,
		identityObservers:      make(map[string]map[string]func(identity string, profile *model.IdentityProfile)),
		identityEncryptionKeys: make(map[string]symmetric.Key),
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
	s.closing = make(chan struct{})
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) (err error) {
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

	// TODO Temp for testing purposes
	err = s.RegisterIdentity("space1", "AAj9HKbneHRsiEbbGj7Lhm2WJHzYNVwnz3qe2Mncn2mF49Wx", symmetric.Key{}, func(identity string, profile *model.IdentityProfile) {
		fmt.Println("OBSERVED IDENTITY DATA for", identity, profile)
	})
	if err != nil {
		return err
	}

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

func (s *service) pushProfileToIdentityRegistry(ctx context.Context, profileDetails *types.Struct) error {
	iconImageObjectId := pbtypes.GetString(profileDetails, bundle.RelationKeyIconImage.String())
	var (
		iconCid            string
		iconEncryptionKeys []*model.IdentityProfileEncryptionKey
	)
	if iconImageObjectId != "" {
		fileId, err := s.fileObjectService.GetFileIdFromObject(ctx, iconImageObjectId)
		if err != nil {
			return fmt.Errorf("get file id from object: %w", err)
		}
		iconCid = fileId.FileId.String()
		keys, err := s.fileService.FileGetKeys(fileId)
		if err != nil {
			return fmt.Errorf("get file keys: %w", err)
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
	// TODO Encrypt data using metadata symmetric key

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

	fmt.Println("PUSHED IDENTITY DATA for", s.accountService.AccountID(), identityProfile)
	return nil
}

func (s *service) observeIdentitiesLoop() {
	ticker := time.NewTicker(s.identityObservePeriod)
	defer ticker.Stop()

	ctx := context.Background()
	for {
		select {
		case <-s.closing:
			return
		case <-ticker.C:
			err := s.observeIdentities(ctx)
			if err != nil {
				log.Error("error observing identities", zap.Error(err))
			}
		}
	}
}

const identityRepoDataKind = "profile"

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
	identitiesData, err := s.coordinatorClient.IdentityRepoGet(ctx, identities, []string{identityRepoDataKind})
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

func (s *service) handleIdentityData(identityData *identityrepoproto.DataWithIdentity) error {
	var profile *model.IdentityProfile
	// TODO Decrypt
	for _, data := range identityData.Data {
		if data.Kind == identityRepoDataKind {
			profile = new(model.IdentityProfile)
			err := proto.Unmarshal(data.Data, profile)
			if err != nil {
				return fmt.Errorf("unmarshal identity profile: %w", err)
			}
		}
	}
	if profile == nil {
		return fmt.Errorf("no profile data found")
	}

	prevProfile, ok := s.identityProfileCache[identityData.Identity]
	if ok && proto.Equal(prevProfile, profile) {
		return nil
	}

	if len(profile.IconEncryptionKeys) > 0 {
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

	// TODO Store profile in badger

	s.identityProfileCache[identityData.Identity] = profile
	observers := s.identityObservers[identityData.Identity]
	for _, obs := range observers {
		obs(identityData.Identity, profile)
	}
	return nil
}

func (s *service) RegisterIdentity(spaceId string, identity string, encryptionKey symmetric.Key, observer func(identity string, profile *model.IdentityProfile)) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if key, ok := s.identityEncryptionKeys[identity]; ok {
		if !slices.Equal(key.Bytes(), encryptionKey.Bytes()) {
			return fmt.Errorf("encryption key for identity %s already exists and do not match new key", identity)
		}
	} else {
		s.identityEncryptionKeys[identity] = encryptionKey
	}

	observers := s.identityObservers[identity]
	if observers == nil {
		observers = make(map[string]func(identity string, profile *model.IdentityProfile))
		s.identityObservers[identity] = observers
	}

	s.identityObservers[identity][spaceId] = observer

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
