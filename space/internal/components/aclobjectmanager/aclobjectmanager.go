package aclobjectmanager

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/crypto/cryptoproto"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "common.components.aclobjectmanager"

var log = logger.NewNamed(CName)

type AclObjectManager interface {
	app.ComponentRunnable
}

func New(ownerMetadata []byte) AclObjectManager {
	return &aclObjectManager{
		ownerMetadata: ownerMetadata,
	}
}

type aclObjectManager struct {
	ctx             context.Context
	cancel          context.CancelFunc
	wait            chan struct{}
	waitLoad        chan struct{}
	sp              clientspace.Space
	loadErr         error
	spaceLoader     spaceloader.SpaceLoader
	status          spacestatus.SpaceStatus
	modifier        dependencies.DetailsModifier
	identityService dependencies.IdentityService
	indexer         dependencies.SpaceIndexer
	started         bool

	ownerMetadata []byte
	mx            sync.Mutex
	lastIndexed   string
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
	common := a.sp.CommonSpace()
	common.Acl().SetAclUpdater(a)
	common.Acl().RLock()
	defer common.Acl().RUnlock()
	err := a.processAcl()
	if err != nil {
		log.Error("error processing acl", zap.Error(err))
	}
}

func (a *aclObjectManager) clearAclIndexes() (err error) {
	return a.indexer.RemoveAclIndexes(a.status.SpaceId())
}

func (a *aclObjectManager) deleteObject(identity crypto.PubKey) (err error) {
	// TODO: remove object from cache and clear acl indexes in object store for this object
	a.identityService.UnregisterIdentity(a.sp.Id(), identity.Account())
	return nil
}

func (a *aclObjectManager) processAcl() (err error) {
	common := a.sp.CommonSpace()
	a.mx.Lock()
	lastIndexed := a.lastIndexed
	a.mx.Unlock()
	if lastIndexed == common.Acl().Head().Id {
		return nil
	}
	var diff list.AclAccountDiff
	// get all identities and permissions for us to process
	if lastIndexed == "" {
		diff.Added = common.Acl().AclState().CurrentStates()
	} else {
		diff, err = common.Acl().AclState().ChangedStates(lastIndexed, common.Acl().Head().Id)
		if err != nil {
			return
		}
	}
	decrypt := func(key crypto.PubKey) ([]byte, error) {
		if a.ownerMetadata != nil {
			return a.ownerMetadata, nil
		}
		return common.Acl().AclState().GetMetadata(key, true)
	}
	// decrypt all metadata
	decryptedAdded, err := decryptAll(diff.Added, decrypt)
	if err != nil {
		return
	}
	decryptedChanged, err := decryptAll(diff.Changed, decrypt)
	if err != nil {
		return
	}
	diff.Added = decryptedAdded
	diff.Changed = decryptedChanged
	a.mx.Lock()
	defer a.mx.Unlock()
	err = a.processDiff(diff)
	if err != nil {
		return
	}
	recs, err := common.Acl().AclState().JoinRecords(true)
	if err != nil {
		return
	}
	err = a.processJoinRecords(recs)
	if err != nil {
		return
	}
	a.lastIndexed = common.Acl().Head().Id
	return
}

func (a *aclObjectManager) processDiff(diff list.AclAccountDiff) (err error) {
	for _, state := range diff.Added {
		err := a.updateParticipantFromAclState(a.ctx, state)
		if err != nil {
			return err
		}
		key, err := getSymKey(state.RequestMetadata)
		if err != nil {
			return err
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
	}
	for _, state := range diff.Changed {
		err := a.updateParticipantFromAclState(a.ctx, state)
		if err != nil {
			return err
		}
	}
	for _, state := range diff.Removed {
		err := a.deleteObject(state.PubKey)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *aclObjectManager) processJoinRecords(recs []list.RequestRecord) (err error) {
	for _, rec := range recs {
		err := a.updateParticipantFromAclRequest(a.ctx, rec)
		if err != nil {
			return err
		}
		key, err := getSymKey(rec.RequestMetadata)
		if err != nil {
			return err
		}
		err = a.identityService.RegisterIdentity(a.sp.Id(), rec.RequestIdentity.Account(), key,
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
	}
	return nil
}

func (a *aclObjectManager) updateParticipantFromAclState(ctx context.Context, accState list.AclAccountState) (err error) {
	id := source.NewParticipantId(a.sp.Id(), accState.PubKey.Account())
	_, err = a.sp.GetObject(ctx, id)
	if err != nil {
		return err
	}
	details := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyId.String():                     pbtypes.String(id),
		bundle.RelationKeyIdentity.String():               pbtypes.String(accState.PubKey.Account()),
		bundle.RelationKeyIsReadonly.String():             pbtypes.Bool(true),
		bundle.RelationKeyIsArchived.String():             pbtypes.Bool(false),
		bundle.RelationKeyIsHidden.String():               pbtypes.Bool(false),
		bundle.RelationKeySpaceId.String():                pbtypes.String(a.sp.Id()),
		bundle.RelationKeyType.String():                   pbtypes.String(bundle.TypeKeyParticipant.BundledURL()),
		bundle.RelationKeyLayout.String():                 pbtypes.Float64(float64(model.ObjectType_participant)),
		bundle.RelationKeyLastModifiedBy.String():         pbtypes.String(id),
		bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Active)),
		bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(convertPermissions(accState.Permissions))),
	}}
	return a.modifier.ModifyDetails(id, func(current *types.Struct) (*types.Struct, error) {
		return pbtypes.StructMerge(current, details, false), nil
	})
}

func (a *aclObjectManager) updateParticipantFromIdentity(ctx context.Context, identity string, profile *model.IdentityProfile) (err error) {
	id := source.NewParticipantId(a.sp.Id(), identity)
	_, err = a.sp.GetObject(ctx, id)
	if err != nil {
		return err
	}
	details := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():      pbtypes.String(profile.Name),
		bundle.RelationKeyIconImage.String(): pbtypes.String(profile.IconCid),
	}}
	return a.modifier.ModifyDetails(id, func(current *types.Struct) (*types.Struct, error) {
		return pbtypes.StructMerge(current, details, false), nil
	})
}

func (a *aclObjectManager) updateParticipantFromAclRequest(ctx context.Context, rec list.RequestRecord) (err error) {
	key := fmt.Sprintf("%s_%s", a.sp.Id(), rec.RequestIdentity.Account())
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeParticipant, key)
	if err != nil {
		return
	}
	id := uniqueKey.Marshal()
	_, err = a.sp.GetObject(ctx, uniqueKey.Marshal())
	details := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyId.String():                     pbtypes.String(id),
		bundle.RelationKeyIdentity.String():               pbtypes.String(rec.RequestIdentity.Account()),
		bundle.RelationKeyIsReadonly.String():             pbtypes.Bool(true),
		bundle.RelationKeyIsArchived.String():             pbtypes.Bool(false),
		bundle.RelationKeyIsHidden.String():               pbtypes.Bool(false),
		bundle.RelationKeySpaceId.String():                pbtypes.String(a.sp.Id()),
		bundle.RelationKeyType.String():                   pbtypes.String(bundle.TypeKeyParticipant.BundledURL()),
		bundle.RelationKeyLayout.String():                 pbtypes.Float64(float64(model.ObjectType_participant)),
		bundle.RelationKeyLastModifiedBy.String():         pbtypes.String(id),
		bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Joining)),
		bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_NoPermissions)),
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
	return model.ParticipantPermissions_Reader
}

func decryptAll(states []list.AclAccountState, decrypt func(key crypto.PubKey) ([]byte, error)) (decrypted []list.AclAccountState, err error) {
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
