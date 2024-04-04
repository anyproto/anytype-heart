package aclobjectmanager

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/debugstat"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/crypto/cryptoproto"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/aclnotifications"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "common.components.aclobjectmanager"

var log = logger.NewNamed(CName)

type AclObjectManager interface {
	app.ComponentRunnable
}

func New(ownerMetadata []byte) AclObjectManager {
	return &aclObjectManager{
		ownerMetadata:     ownerMetadata,
		addedParticipants: make(map[string]struct{}),
	}
}

type aclObjectManager struct {
	ctx                 context.Context
	cancel              context.CancelFunc
	wait                chan struct{}
	waitLoad            chan struct{}
	sp                  clientspace.Space
	loadErr             error
	spaceLoader         spaceloader.SpaceLoader
	status              spacestatus.SpaceStatus
	modifier            dependencies.DetailsModifier
	identityService     dependencies.IdentityService
	indexer             dependencies.SpaceIndexer
	statService         debugstat.StatService
	started             bool
	notificationService aclnotifications.AclNotification

	ownerMetadata     []byte
	mx                sync.Mutex
	lastIndexed       string
	addedParticipants map[string]struct{}
}

func (a *aclObjectManager) ProvideStat() any {
	select {
	case <-a.waitLoad:
		if a.loadErr != nil {
			return parseAcl(nil, a.status.SpaceId())
		}
		return parseAcl(a.sp.CommonSpace().Acl(), a.status.SpaceId())
	default:
		return parseAcl(nil, a.status.SpaceId())
	}
}

func (a *aclObjectManager) StatId() string {
	return a.status.SpaceId()
}

func (a *aclObjectManager) StatType() string {
	return CName
}

func (a *aclObjectManager) UpdateAcl(aclList list.AclList) {
	err := a.processAcl()
	if err != nil {
		log.Error("error processing acl", zap.Error(err))
	}
}

func (a *aclObjectManager) Init(ap *app.App) (err error) {
	a.spaceLoader = ap.MustComponent(spaceloader.CName).(spaceloader.SpaceLoader)
	a.modifier = app.MustComponent[dependencies.DetailsModifier](ap)
	a.identityService = app.MustComponent[dependencies.IdentityService](ap)
	a.indexer = app.MustComponent[dependencies.SpaceIndexer](ap)
	a.status = app.MustComponent[spacestatus.SpaceStatus](ap)
	a.notificationService = app.MustComponent[aclnotifications.AclNotification](ap)
	a.statService, _ = ap.Component(debugstat.CName).(debugstat.StatService)
	if a.statService == nil {
		a.statService = debugstat.NewNoOp()
	}
	a.statService.AddProvider(a)
	a.waitLoad = make(chan struct{})
	a.wait = make(chan struct{})
	return nil
}

func (a *aclObjectManager) Name() (name string) {
	return CName
}

func (a *aclObjectManager) Run(ctx context.Context) (err error) {
	err = a.clearAclIndexes()
	if err != nil {
		return
	}
	a.started = true
	a.ctx, a.cancel = context.WithCancel(context.Background())
	go a.waitSpace()
	go a.process()
	return
}

func (a *aclObjectManager) Close(ctx context.Context) (err error) {
	if !a.started {
		return
	}
	a.cancel()
	<-a.wait
	a.identityService.UnregisterIdentitiesInSpace(a.status.SpaceId())
	a.statService.RemoveProvider(a)
	return
}

func (a *aclObjectManager) waitSpace() {
	a.sp, a.loadErr = a.spaceLoader.WaitLoad(a.ctx)
	close(a.waitLoad)
}

func (a *aclObjectManager) process() {
	defer close(a.wait)
	select {
	case <-a.ctx.Done():
		return
	case <-a.waitLoad:
		if a.loadErr != nil {
			return
		}
		break
	}

	err := a.initAndRegisterMyIdentity(a.ctx)
	if err != nil {
		log.Error("init my identity", zap.Error(err))
	}

	common := a.sp.CommonSpace()
	common.Acl().SetAclUpdater(a)
	common.Acl().RLock()
	defer common.Acl().RUnlock()
	err = a.processAcl()
	if err != nil {
		log.Error("error processing acl", zap.Error(err))
	}
}

func (a *aclObjectManager) sendNotifications(common commonspace.Space) {
	permissions := common.Acl().AclState().Permissions(common.Acl().AclState().AccountKey().GetPublic())
	a.notificationService.AddRecords(common.Acl().(list.AclList), permissions, a.sp.Id(), spaceinfo.AccountStatusActive)
}

func (a *aclObjectManager) initAndRegisterMyIdentity(ctx context.Context) error {
	myIdentity, metadataKey, profileDetails := a.identityService.GetMyProfileDetails()
	id := domain.NewParticipantId(a.sp.Id(), myIdentity)
	_, err := a.sp.GetObject(ctx, id)
	if err != nil {
		return err
	}
	details := buildParticipantDetails(id, a.sp.Id(), myIdentity, model.ParticipantPermissions_Owner, model.ParticipantStatus_Active)
	details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(pbtypes.GetString(profileDetails, bundle.RelationKeyName.String()))
	details.Fields[bundle.RelationKeyDescription.String()] = pbtypes.String(pbtypes.GetString(profileDetails, bundle.RelationKeyDescription.String()))
	details.Fields[bundle.RelationKeyIconImage.String()] = pbtypes.String(pbtypes.GetString(profileDetails, bundle.RelationKeyIconImage.String()))
	details.Fields[bundle.RelationKeyIdentityProfileLink.String()] = pbtypes.String(pbtypes.GetString(profileDetails, bundle.RelationKeyId.String()))
	details.Fields[bundle.RelationKeyGlobalName.String()] = pbtypes.String(pbtypes.GetString(profileDetails, bundle.RelationKeyGlobalName.String()))
	err = a.modifier.ModifyDetails(id, func(current *types.Struct) (*types.Struct, error) {
		return pbtypes.StructMerge(current, details, false), nil
	})
	if err != nil {
		return err
	}
	err = a.identityService.RegisterIdentity(a.sp.Id(), myIdentity, metadataKey,
		func(identity string, profile *model.IdentityProfile) {
			err := a.updateParticipantFromIdentity(a.ctx, identity, profile)
			if err != nil {
				log.Error("error updating participant from identity", zap.Error(err))
			}
		},
	)
	if err != nil {
		return err
	}
	a.mx.Lock()
	a.addedParticipants[myIdentity] = struct{}{}
	a.mx.Unlock()
	return nil
}

func (a *aclObjectManager) clearAclIndexes() (err error) {
	return a.indexer.RemoveAclIndexes(a.status.SpaceId())
}

func (a *aclObjectManager) processAcl() (err error) {
	common := a.sp.CommonSpace()
	a.mx.Lock()
	lastIndexed := a.lastIndexed
	a.mx.Unlock()
	if lastIndexed == common.Acl().Head().Id {
		a.mx.Lock()
		a.sendNotifications(common)
		a.mx.Unlock()
		return nil
	}
	decrypt := func(key crypto.PubKey) ([]byte, error) {
		if a.ownerMetadata != nil {
			return a.ownerMetadata, nil
		}
		return common.Acl().AclState().GetMetadata(key, true)
	}
	states := common.Acl().AclState().CurrentAccounts()
	// decrypt all metadata
	states, err = decryptAll(states, decrypt)
	if err != nil {
		return
	}
	a.mx.Lock()
	defer a.mx.Unlock()
	aclHeadId := a.status.GetLatestAclHeadId()
	var upToDate bool
	if aclHeadId != "" {
		_, err := common.Acl().Get(aclHeadId)
		if err == nil {
			upToDate = true
		}
	} else {
		upToDate = true
	}
	err = a.processStates(states, upToDate, common.Acl().AclState().Identity())
	if err != nil {
		return
	}
	a.lastIndexed = common.Acl().Head().Id
	a.sendNotifications(common)
	return
}

func (a *aclObjectManager) processStates(states []list.AccountState, upToDate bool, myIdentity crypto.PubKey) (err error) {
	var numActiveUsers int
	for _, state := range states {
		if state.Permissions.NoPermissions() && state.PubKey.Equals(myIdentity) && upToDate {
			return a.status.SetPersistentStatus(spaceinfo.AccountStatusRemoving)
		}
		if !state.Permissions.NoPermissions() {
			numActiveUsers++
		}
		err := a.updateParticipantFromAclState(a.ctx, state)
		if err != nil {
			return err
		}
		key, err := getSymKey(state.RequestMetadata)
		if err != nil {
			return err
		}
		accKey := state.PubKey.Account()
		if _, exists := a.addedParticipants[state.PubKey.Account()]; exists {
			continue
		}
		err = a.identityService.RegisterIdentity(a.sp.Id(), state.PubKey.Account(), key,
			func(identity string, profile *model.IdentityProfile) {
				err := a.updateParticipantFromIdentity(a.ctx, identity, profile)
				if err != nil {
					log.Error("error updating participant from identity", zap.Error(err))
				}
			},
		)
		if err != nil {
			return err
		}
		a.addedParticipants[accKey] = struct{}{}
	}
	isEmpty := len(a.sp.CommonSpace().Acl().AclState().Invites()) == 0 && numActiveUsers == 1
	return a.status.SetAclIsEmpty(isEmpty)
}

func (a *aclObjectManager) updateParticipantFromAclState(ctx context.Context, accState list.AccountState) (err error) {
	id := domain.NewParticipantId(a.sp.Id(), accState.PubKey.Account())
	_, err = a.sp.GetObject(ctx, id)
	if err != nil {
		return err
	}
	details := buildParticipantDetails(
		id,
		a.sp.Id(),
		accState.PubKey.Account(),
		convertPermissions(accState.Permissions),
		convertStatus(accState.Status))
	return a.modifier.ModifyDetails(id, func(current *types.Struct) (*types.Struct, error) {
		return pbtypes.StructMerge(current, details, false), nil
	})
}

func (a *aclObjectManager) updateParticipantFromIdentity(ctx context.Context, identity string, profile *model.IdentityProfile) (err error) {
	id := domain.NewParticipantId(a.sp.Id(), identity)
	_, err = a.sp.GetObject(ctx, id)
	if err != nil {
		return err
	}
	details := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():        pbtypes.String(profile.Name),
		bundle.RelationKeyDescription.String(): pbtypes.String(profile.Description),
		bundle.RelationKeyIconImage.String():   pbtypes.String(profile.IconCid),
		bundle.RelationKeyGlobalName.String():  pbtypes.String(profile.GlobalName),
	}}
	return a.modifier.ModifyDetails(id, func(current *types.Struct) (*types.Struct, error) {
		return pbtypes.StructMerge(current, details, false), nil
	})
}

func convertPermissions(permissions list.AclPermissions) model.ParticipantPermissions {
	switch aclrecordproto.AclUserPermissions(permissions) {
	case aclrecordproto.AclUserPermissions_Writer:
		return model.ParticipantPermissions_Writer
	case aclrecordproto.AclUserPermissions_Reader:
		return model.ParticipantPermissions_Reader
	case aclrecordproto.AclUserPermissions_Owner:
		return model.ParticipantPermissions_Owner
	}
	return model.ParticipantPermissions_NoPermissions
}

func convertStatus(status list.AclStatus) model.ParticipantStatus {
	switch status {
	case list.StatusJoining:
		return model.ParticipantStatus_Joining
	case list.StatusActive:
		return model.ParticipantStatus_Active
	case list.StatusRemoved:
		return model.ParticipantStatus_Removed
	case list.StatusDeclined:
		return model.ParticipantStatus_Declined
	case list.StatusRemoving:
		return model.ParticipantStatus_Removing
	case list.StatusCanceled:
		return model.ParticipantStatus_Canceled
	}
	return model.ParticipantStatus_Active
}

func decryptAll(states []list.AccountState, decrypt func(key crypto.PubKey) ([]byte, error)) (decrypted []list.AccountState, err error) {
	for _, state := range states {
		res, err := decrypt(state.PubKey)
		if err != nil {
			return nil, err
		}
		state.RequestMetadata = res
		decrypted = append(decrypted, state)
	}
	return
}

func getSymKey(metadata []byte) (crypto.SymKey, error) {
	md := &model.Metadata{}
	err := md.Unmarshal(metadata)
	if err != nil {
		return nil, err
	}
	keyProto := &cryptoproto.Key{}
	err = keyProto.Unmarshal(md.GetIdentity().GetProfileSymKey())
	if err != nil {
		return nil, err
	}
	return crypto.UnmarshallAESKey(keyProto.Data)
}

func buildParticipantDetails(
	id string,
	spaceId string,
	identity string,
	permissions model.ParticipantPermissions,
	status model.ParticipantStatus,
) *types.Struct {
	return &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyId.String():                     pbtypes.String(id),
		bundle.RelationKeyIdentity.String():               pbtypes.String(identity),
		bundle.RelationKeySpaceId.String():                pbtypes.String(spaceId),
		bundle.RelationKeyLastModifiedBy.String():         pbtypes.String(id),
		bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(permissions)),
		bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(status)),
		bundle.RelationKeyIsHiddenDiscovery.String():      pbtypes.Bool(status != model.ParticipantStatus_Active),
	}}
}
