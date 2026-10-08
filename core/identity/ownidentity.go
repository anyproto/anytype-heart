package identity

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/identityrepo/identityrepoproto"
	"github.com/anyproto/any-sync/nameservice/nameserviceclient"
	"github.com/anyproto/any-sync/nameservice/nameserviceproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/proto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileacl"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

type observerService interface {
	broadcastMyIdentityProfile(identityProfile *model.IdentityProfile)
	refreshMyIdentityProfile()
}

const (
	pushRetryMinDelay = 10 * time.Second
	pushRetryMaxDelay = 10 * time.Minute
)

var errOwnProfileNotLoaded = errors.New("own profile details are not loaded")

type ownProfileSubscription struct {
	spaceService                 space.Service
	objectStore                  objectstore.ObjectStore
	accountService               account.Service
	identityRepoClient           identityRepoClient
	fileAclService               fileacl.Service
	observerService              observerService
	namingService                nameserviceclient.AnyNsClientService
	identityGlobalNameCacheStore keyvaluestore.Store[string]
	identityProfileCacheStore    keyvaluestore.Store[[]byte]

	myIdentity          string
	globalNameUpdatedCh chan string
	gotDetailsCh        chan struct{}

	detailsLock sync.Mutex
	gotDetails  bool
	details     *domain.Details // save details to batch update operation

	pushRunLock              sync.Mutex // serializes pushes, so the last one pushes the latest profile
	pushLock                 sync.Mutex
	pushIdentityTimer        *time.Timer // timer for batching
	pushIdentityBatchTimeout time.Duration
	pushGeneration           int // incremented on every enqueued push
	pushRetryDelay           time.Duration
	pushRetryMinDelay        time.Duration
	pushRetryMaxDelay        time.Duration
	profilePreparedCh        chan struct{} // signals the run loop that the profile with the icon keys was prepared

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
	pushIdentityBatchTimeout time.Duration,
	identityGlobalNameCacheStore keyvaluestore.Store[string],
	identityProfileCacheStore keyvaluestore.Store[[]byte],
) *ownProfileSubscription {
	componentCtx, componentCtxCancel := context.WithCancel(context.Background())
	return &ownProfileSubscription{
		spaceService:                 spaceService,
		objectStore:                  objectStore,
		accountService:               accountService,
		identityRepoClient:           identityRepoClient,
		fileAclService:               fileAclService,
		observerService:              observerService,
		namingService:                namingService,
		globalNameUpdatedCh:          make(chan string),
		gotDetailsCh:                 make(chan struct{}),
		pushIdentityBatchTimeout:     pushIdentityBatchTimeout,
		pushRetryMinDelay:            pushRetryMinDelay,
		pushRetryMaxDelay:            pushRetryMaxDelay,
		profilePreparedCh:            make(chan struct{}, 1),
		componentCtx:                 componentCtx,
		componentCtxCancel:           componentCtxCancel,
		identityGlobalNameCacheStore: identityGlobalNameCacheStore,
		identityProfileCacheStore:    identityProfileCacheStore,
	}
}

func (s *ownProfileSubscription) run(ctx context.Context) (err error) {
	s.myIdentity = s.accountService.AccountID()
	techSpace, err := s.spaceService.GetTechSpace(ctx)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	id, err := techSpace.(*clientspace.TechSpace).TechSpace.AccountObjectId()
	if err != nil {
		return err
	}

	recordsCh := make(chan *domain.Details)
	sub := database.NewSubscription(nil, recordsCh)

	var (
		records  []database.Record
		closeSub func()
	)

	records, closeSub, err = s.objectStore.SpaceIndex(s.spaceService.TechSpaceId()).QueryByIdsAndSubscribeForChanges([]string{id}, sub)
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

			case <-s.profilePreparedCh:
				s.observerService.refreshMyIdentityProfile()
			}
		}
	}()

	return nil
}

func (s *ownProfileSubscription) close() {
	s.componentCtxCancel()

	s.pushLock.Lock()
	defer s.pushLock.Unlock()
	if s.pushIdentityTimer != nil {
		s.pushIdentityTimer.Stop()
	}
}

func (s *ownProfileSubscription) enqueuePush() {
	s.pushLock.Lock()
	defer s.pushLock.Unlock()
	s.pushGeneration++
	s.pushRetryDelay = 0
	if s.pushIdentityTimer == nil {
		s.pushIdentityTimer = time.AfterFunc(0, s.push)
	} else {
		s.pushIdentityTimer.Reset(s.pushIdentityBatchTimeout)
	}
}

// push pushes the own profile to the identity repo. A failed push is retried with a growing
// delay until it succeeds or a newer change is enqueued: e.g. the keys of the icon file can
// arrive from another device after the profile details, and nothing else would push again.
func (s *ownProfileSubscription) push() {
	s.pushRunLock.Lock()
	defer s.pushRunLock.Unlock()
	if s.componentCtx.Err() != nil {
		return
	}
	s.pushLock.Lock()
	generation := s.pushGeneration
	s.pushLock.Unlock()

	profile, err := s.prepareOwnIdentityProfile()
	if errors.Is(err, errOwnProfileNotLoaded) {
		// the profile is pushed once its details are loaded
		return
	}
	if err != nil {
		err = fmt.Errorf("prepare own identity profile: %w", err)
	} else {
		// the cached own profile may lack the icon keys that are readable now; it is refreshed
		// even if the upload below fails
		select {
		case s.profilePreparedCh <- struct{}{}:
		default:
		}
		err = s.pushProfileToIdentityRegistry(s.componentCtx, profile)
	}
	if err == nil || s.componentCtx.Err() != nil {
		return
	}
	log.Error("push profile to identity registry", zap.Error(err))

	s.pushLock.Lock()
	defer s.pushLock.Unlock()
	if generation != s.pushGeneration || s.componentCtx.Err() != nil {
		// a newer change is already scheduled, or the component is closed
		return
	}
	s.pushRetryDelay = min(max(2*s.pushRetryDelay, s.pushRetryMinDelay), s.pushRetryMaxDelay)
	s.pushIdentityTimer.Reset(s.pushRetryDelay)
}

func (s *ownProfileSubscription) handleOwnProfileDetails(profileDetails *domain.Details) {
	if profileDetails == nil {
		return
	}
	s.detailsLock.Lock()
	if !s.gotDetails {
		close(s.gotDetailsCh)
		s.gotDetails = true
	}

	if s.details == nil {
		s.details = domain.NewDetails()
	}
	for _, key := range []domain.RelationKey{
		bundle.RelationKeyId,
		bundle.RelationKeyName,
		bundle.RelationKeyDescription,
		bundle.RelationKeyGlobalName,
	} {
		if v, ok := profileDetails.TryString(key); ok {
			s.details.SetString(key, v)
		}
	}
	// Resolve file object ID to file CID so that iconImage is consistent
	// with what the identity repo stores and what other peers receive.
	if iconObjectId, ok := profileDetails.TryString(bundle.RelationKeyIconImage); ok {
		iconCid, _, err := s.prepareIconImageInfo(iconObjectId)
		if err != nil {
			log.Error("handleOwnProfileDetails: resolve icon to file cid", zap.Error(err))
			iconCid = iconObjectId
		}
		s.details.SetString(bundle.RelationKeyIconImage, iconCid)
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
		s.details = domain.NewDetails()
	}
	s.details.SetString(bundle.RelationKeyGlobalName, globalName)
	identityProfile := s.prepareIdentityProfile()
	s.detailsLock.Unlock()

	err := s.identityGlobalNameCacheStore.Set(context.Background(), s.myIdentity, globalName)
	if err != nil {
		log.Error("save global name", zap.String("identity", s.myIdentity), zap.Error(err))
	}

	s.observerService.broadcastMyIdentityProfile(identityProfile)

	s.enqueuePush()
}

func (s *ownProfileSubscription) prepareIdentityProfile() *model.IdentityProfile {
	return &model.IdentityProfile{
		Identity:    s.myIdentity,
		Name:        s.details.GetString(bundle.RelationKeyName),
		Description: s.details.GetString(bundle.RelationKeyDescription),
		IconCid:     s.details.GetString(bundle.RelationKeyIconImage),
		GlobalName:  s.details.GetString(bundle.RelationKeyGlobalName),
	}
}

// isLoaded reports whether the own profile details have been loaded
func (s *ownProfileSubscription) isLoaded() bool {
	s.detailsLock.Lock()
	defer s.detailsLock.Unlock()
	return s.gotDetails
}

func (s *ownProfileSubscription) pushProfileToIdentityRegistry(ctx context.Context, identityProfile *model.IdentityProfile) error {
	encryptedIdentityProfileBytes, err := proto.Marshal(identityProfile)
	if err != nil {
		return fmt.Errorf("marshal identity profile: %w", err)
	}

	symKey := s.spaceService.AccountMetadataSymKey()
	encryptedIdentityProfileBytes, err = symKey.Encrypt(encryptedIdentityProfileBytes)
	if err != nil {
		return fmt.Errorf("encrypt data: %w", err)
	}

	signature, err := s.accountService.SignData(encryptedIdentityProfileBytes)
	if err != nil {
		return fmt.Errorf("failed to sign profile data: %w", err)
	}

	err = s.identityRepoClient.IdentityRepoPut(ctx, s.myIdentity, []*identityrepoproto.Data{
		{
			Kind:      identityRepoDataKind,
			Data:      encryptedIdentityProfileBytes,
			Signature: signature,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to push identity: %w", err)
	}

	return s.identityProfileCacheStore.Set(context.Background(), identityProfile.Identity, encryptedIdentityProfileBytes)
}

// prepareOwnIdentityProfile returns the own profile as it is pushed to the identity repo, with
// the icon encryption keys. It returns errOwnProfileNotLoaded until the profile details are
// loaded: the global name can arrive first, and a profile without a name must not overwrite
// the pushed one.
func (s *ownProfileSubscription) prepareOwnIdentityProfile() (*model.IdentityProfile, error) {
	s.detailsLock.Lock()
	defer s.detailsLock.Unlock()
	if !s.gotDetails {
		return nil, errOwnProfileNotLoaded
	}

	iconImageObjectId := s.details.GetString(bundle.RelationKeyIconImage)
	iconCid, iconEncryptionKeys, err := s.prepareIconImageInfo(iconImageObjectId)
	if err != nil {
		return nil, fmt.Errorf("prepare icon image info: %w", err)
	}

	identity := s.accountService.AccountID()
	return &model.IdentityProfile{
		Identity:           identity,
		Name:               s.details.GetString(bundle.RelationKeyName),
		Description:        s.details.GetString(bundle.RelationKeyDescription),
		IconCid:            iconCid,
		IconEncryptionKeys: iconEncryptionKeys,
		GlobalName:         s.details.GetString(bundle.RelationKeyGlobalName),
	}, nil
}

func (s *ownProfileSubscription) prepareIconImageInfo(iconImageObjectId string) (iconCid string, iconEncryptionKeys []*model.FileEncryptionKey, err error) {
	if iconImageObjectId == "" {
		return "", nil, nil
	}
	return s.fileAclService.GetInfoForFileSharing(iconImageObjectId)
}

func (s *ownProfileSubscription) getDetails(ctx context.Context) (identity string, metadataKey crypto.SymKey, details *domain.Details) {
	select {
	case <-s.gotDetailsCh:

	case <-ctx.Done():
		return "", nil, nil
	case <-s.componentCtx.Done():
		return "", nil, nil
	}
	s.detailsLock.Lock()
	defer s.detailsLock.Unlock()

	detailsCopy := s.details.Copy()
	return s.myIdentity, s.spaceService.AccountMetadataSymKey(), detailsCopy
}
