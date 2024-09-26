package identity

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type observerService interface {
	broadcastMyIdentityProfile(identityProfile *model.IdentityProfile)
}

type ownProfileSubscription struct {
	spaceService       space.Service
	objectStore        objectstore.ObjectStore
	accountService     account.Service
	identityRepoClient identityRepoClient
	fileAclService     fileacl.Service
	observerService    observerService
	namingService      nameserviceclient.AnyNsClientService
	dbProvider         datastore.Datastore
	db                 *badger.DB

	myIdentity          string
	globalNameUpdatedCh chan string
	gotDetailsCh        chan struct{}

	detailsLock sync.Mutex
	gotDetails  bool
	details     *types.Struct // save details to batch update operation

	pushIdentityTimer        *time.Timer // timer for batching
	pushIdentityBatchTimeout time.Duration

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
}

func newOwnProfileSubscription(
	spaceService space.Service,
	objectStore objectstore.ObjectStore,
	accountService account.Service,
	identityRepoClient identityRepoClient,
	fileAclService fileacl.Service,
	observerService observerService,
	namingService nameserviceclient.AnyNsClientService,
	dbProvider datastore.Datastore,
	pushIdentityBatchTimeout time.Duration,
) *ownProfileSubscription {
	componentCtx, componentCtxCancel := context.WithCancel(context.Background())
	return &ownProfileSubscription{
		spaceService:             spaceService,
		objectStore:              objectStore,
		accountService:           accountService,
		identityRepoClient:       identityRepoClient,
		fileAclService:           fileAclService,
		observerService:          observerService,
		namingService:            namingService,
		globalNameUpdatedCh:      make(chan string),
		gotDetailsCh:             make(chan struct{}),
		pushIdentityBatchTimeout: pushIdentityBatchTimeout,
		componentCtx:             componentCtx,
		componentCtxCancel:       componentCtxCancel,
		dbProvider:               dbProvider,
	}
}

func (s *ownProfileSubscription) run(ctx context.Context) (err error) {
	s.db, err = s.dbProvider.LocalStorage()
	if err != nil {
		return err
	}

	s.myIdentity = s.accountService.AccountID()

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

	records, closeSub, err = s.objectStore.SpaceId(personalSpace.Id()).QueryByIDAndSubscribeForChanges([]string{profileObjectId}, sub)
	if err != nil {
		return err
	}
	go func() {
		select {
		case <-s.componentCtx.Done():
			closeSub()
			return
		}
	}()

	if len(records) > 0 {
		s.handleOwnProfileDetails(records[0].Details)
	}

	go s.fetchGlobalName(s.componentCtx, s.namingService)

	go func() {
		for {
			select {
			case <-s.componentCtx.Done():
				return
			case rec, ok := <-recordsCh:
				if !ok {
					return
				}
				s.handleOwnProfileDetails(rec)

			case globalName := <-s.globalNameUpdatedCh:
				s.handleGlobalNameUpdate(globalName)
			}
		}
	}()

	return nil
}

func (s *ownProfileSubscription) close() {
	s.componentCtxCancel()
}

func (s *ownProfileSubscription) enqueuePush() {
	if s.pushIdentityTimer == nil {
		s.pushIdentityTimer = time.AfterFunc(0, func() {
			pushErr := s.pushProfileToIdentityRegistry(s.componentCtx)
			if pushErr != nil {
				log.Error("push profile to identity registry", zap.Error(pushErr))
			}
		})
	} else {
		s.pushIdentityTimer.Reset(s.pushIdentityBatchTimeout)
	}
}

func (s *ownProfileSubscription) handleOwnProfileDetails(profileDetails *types.Struct) {
	if profileDetails == nil {
		return
	}
	s.detailsLock.Lock()
	if !s.gotDetails {
		close(s.gotDetailsCh)
		s.gotDetails = true
	}

	if s.details == nil {
		s.details = &types.Struct{
			Fields: map[string]*types.Value{},
		}
	}
	for _, key := range []domain.RelationKey{
		bundle.RelationKeyId,
		bundle.RelationKeyName,
		bundle.RelationKeyDescription,
		bundle.RelationKeyIconImage,
	} {
		if _, ok := profileDetails.Fields[key.String()]; ok {
			s.details.Fields[key.String()] = pbtypes.CopyVal(profileDetails.Fields[key.String()])
		}
	}
	identityProfile := s.prepareIdentityProfile()
	s.detailsLock.Unlock()

	s.observerService.broadcastMyIdentityProfile(identityProfile)
	s.enqueuePush()
}

func (s *ownProfileSubscription) fetchGlobalName(ctx context.Context, ns nameserviceclient.AnyNsClientService) {
	if ns == nil {
		log.Error("error fetching global name of our own identity from Naming Service as the service is not initialized")
		return
	}
	response, err := ns.GetNameByAnyId(ctx, &nameserviceproto.NameByAnyIdRequest{AnyAddress: s.myIdentity})
	if err != nil || response == nil {
		log.Error("error fetching global name of our own identity from Naming Service", zap.Error(err))
		return
	}
	if !response.Found {
		log.Debug("globalName was not found for our own identity in Naming Service")
		return
	}
	s.updateGlobalName(response.Name)
}

func (s *ownProfileSubscription) updateGlobalName(globalName string) {
	select {
	case <-s.componentCtx.Done():
		return
	case s.globalNameUpdatedCh <- globalName:
		return
	}
}

func (s *ownProfileSubscription) handleGlobalNameUpdate(globalName string) {
	s.detailsLock.Lock()
	if s.details == nil {
		s.details = &types.Struct{
			Fields: map[string]*types.Value{},
		}
	}
	s.details.Fields[bundle.RelationKeyGlobalName.String()] = pbtypes.String(globalName)
	identityProfile := s.prepareIdentityProfile()
	s.detailsLock.Unlock()

	err := badgerhelper.SetValue(s.db, makeGlobalNameKey(s.myIdentity), globalName)
	if err != nil {
		log.Error("save global name", zap.String("identity", s.myIdentity), zap.Error(err))
	}

	s.observerService.broadcastMyIdentityProfile(identityProfile)

	s.enqueuePush()
}

func (s *ownProfileSubscription) prepareIdentityProfile() *model.IdentityProfile {
	return &model.IdentityProfile{
		Identity:    s.myIdentity,
		Name:        pbtypes.GetString(s.details, bundle.RelationKeyName.String()),
		Description: pbtypes.GetString(s.details, bundle.RelationKeyDescription.String()),
		IconCid:     pbtypes.GetString(s.details, bundle.RelationKeyIconImage.String()),
		GlobalName:  pbtypes.GetString(s.details, bundle.RelationKeyGlobalName.String()),
	}
}

func (s *ownProfileSubscription) pushProfileToIdentityRegistry(ctx context.Context) error {
	identityProfile, err := s.prepareOwnIdentityProfile()
	if err != nil {
		return fmt.Errorf("prepare own identity profile: %w", err)
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

	err = s.identityRepoClient.IdentityRepoPut(ctx, s.myIdentity, []*identityrepoproto.Data{
		{
			Kind:      "profile",
			Data:      data,
			Signature: signature,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to push identity: %w", err)
	}

	return badgerhelper.SetValue(s.db, makeIdentityProfileKey(identityProfile.Identity), data)
}

func (s *ownProfileSubscription) prepareOwnIdentityProfile() (*model.IdentityProfile, error) {
	s.detailsLock.Lock()
	defer s.detailsLock.Unlock()

	iconImageObjectId := pbtypes.GetString(s.details, bundle.RelationKeyIconImage.String())
	iconCid, iconEncryptionKeys, err := s.prepareIconImageInfo(iconImageObjectId)
	if err != nil {
		return nil, fmt.Errorf("prepare icon image info: %w", err)
	}

	identity := s.accountService.AccountID()
	return &model.IdentityProfile{
		Identity:           identity,
		Name:               pbtypes.GetString(s.details, bundle.RelationKeyName.String()),
		Description:        pbtypes.GetString(s.details, bundle.RelationKeyDescription.String()),
		IconCid:            iconCid,
		IconEncryptionKeys: iconEncryptionKeys,
		GlobalName:         pbtypes.GetString(s.details, bundle.RelationKeyGlobalName.String()),
	}, nil
}

func (s *ownProfileSubscription) prepareIconImageInfo(iconImageObjectId string) (iconCid string, iconEncryptionKeys []*model.FileEncryptionKey, err error) {
	if iconImageObjectId == "" {
		return "", nil, nil
	}
	return s.fileAclService.GetInfoForFileSharing(iconImageObjectId)
}

func (s *ownProfileSubscription) getDetails(ctx context.Context) (identity string, metadataKey crypto.SymKey, details *types.Struct) {
	select {
	case <-s.gotDetailsCh:

	case <-ctx.Done():
		return "", nil, nil
	case <-s.componentCtx.Done():
		return "", nil, nil
	}
	s.detailsLock.Lock()
	defer s.detailsLock.Unlock()

	detailsCopy := pbtypes.CopyStruct(s.details, true)
	return s.myIdentity, s.spaceService.AccountMetadataSymKey(), detailsCopy
}
