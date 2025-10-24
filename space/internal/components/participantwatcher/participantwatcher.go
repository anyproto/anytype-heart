package participantwatcher

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/crypto/cryptoproto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "common.components.participantwatcher"

var log = logger.NewNamed(CName)

type ParticipantWatcher interface {
	app.ComponentRunnable
	WatchParticipant(ctx context.Context, space clientspace.Space, accState list.AccountState) error
	UpdateParticipantFromAclState(ctx context.Context, space clientspace.Space, accState list.AccountState) error
}

type participant interface {
	ModifyIdentityDetails(profile *model.IdentityProfile) (err error)
	ModifyProfileDetails(profileDetails *domain.Details) (err error)
	ModifyParticipantAclState(accState spaceinfo.ParticipantAclInfo) (err error)
}

var _ ParticipantWatcher = (*participantWatcher)(nil)

type participantWatcher struct {
	identityService   dependencies.IdentityService
	accountService    accountservice.Service
	objectStore       objectstore.ObjectStore
	techSpace         techspace.TechSpace
	status            spacestatus.SpaceStatus
	mx                sync.Mutex
	addedParticipants map[string]struct{}
}

func New() ParticipantWatcher {
	return &participantWatcher{
		addedParticipants: map[string]struct{}{},
	}
}

func (p *participantWatcher) getOneToOneParticipantKey(space clientspace.Space) (crypto.SymKey, error) {
	records, err := p.objectStore.SpaceIndex(p.techSpace.TechSpaceId()).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyTargetSpaceId,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(space.Id()),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_spaceView)),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("onetoone: failed to query type object: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("onetoone: failed to query spaceview")
	}

	requestMetadataStr := records[0].Details.GetString(bundle.RelationKeyOneToOneRequestMetadata)
	requestMetadataBytes, rerr := base64.StdEncoding.DecodeString(requestMetadataStr)
	if rerr != nil {
		return nil, fmt.Errorf("failed to decode bob onetoone RequestMetadata: %w", rerr)
	}
	key, err := crypto.UnmarshallAESKeyProto(requestMetadataBytes)
	if err != nil {
		return nil, fmt.Errorf("onetoone: failed to unmarshal requestMetadataBytes")
	}

	return key, nil
}

func (p *participantWatcher) getOneToOneKey(ctx context.Context, space clientspace.Space, state list.AccountState) (key crypto.SymKey, err error) {
	myPubKey := p.accountService.Account().SignKey.GetPublic()
	// it is either me or bob: we don't call WatchParticipant with owner state in aclobjectmanager
	if state.PubKey.Equals(myPubKey) {
		idWithProfileKey := p.identityService.WaitProfile(ctx, myPubKey.Account())
		key = idWithProfileKey.RequestMetadataKey
	} else {
		key, err = p.getOneToOneParticipantKey(space)
		if err != nil {
			return
		}
	}
	return

}
func (p *participantWatcher) WatchParticipant(ctx context.Context, space clientspace.Space, state list.AccountState) (err error) {
	p.mx.Lock()
	defer p.mx.Unlock()
	accKey := state.PubKey.Account()
	if _, exists := p.addedParticipants[state.PubKey.Account()]; exists {
		return
	}
	var key crypto.SymKey

	if space.IsOneToOne() {
		key, err = p.getOneToOneKey(ctx, space, state)
	} else {
		key, err = getSymKey(state.RequestMetadata)
	}
	if err != nil {
		return
	}

	err = p.identityService.RegisterIdentity(space.Id(), state.PubKey.Account(), key, func(identity string, profile *model.IdentityProfile) {
		err := p.updateParticipantFromIdentity(ctx, space, identity, profile)
		if err != nil {
			log.Error("error updating participant from identity", zap.Error(err))
		}
	})
	if err != nil {
		return err
	}
	p.addedParticipants[accKey] = struct{}{}
	return
}

func (p *participantWatcher) Init(a *app.App) (err error) {
	p.identityService = app.MustComponent[dependencies.IdentityService](a)
	p.status = app.MustComponent[spacestatus.SpaceStatus](a)
	p.accountService = app.MustComponent[accountservice.Service](a)
	p.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	p.techSpace = app.MustComponent[techspace.TechSpace](a)
	return nil
}

func (p *participantWatcher) Name() (name string) {
	return CName
}

func (p *participantWatcher) Close(ctx context.Context) (err error) {
	p.identityService.UnregisterIdentitiesInSpace(p.status.SpaceId())
	return
}

func (p *participantWatcher) UpdateAccountParticipantFromProfile(ctx context.Context, space clientspace.Space) error {
	myIdentity, _, profileDetails := p.identityService.GetMyProfileDetails(ctx)
	id := domain.NewParticipantId(space.Id(), myIdentity)
	return space.Do(id, func(sb smartblock.SmartBlock) error {
		return sb.(participant).ModifyProfileDetails(profileDetails)
	})
}

func (p *participantWatcher) UpdateParticipantFromAclState(ctx context.Context, space clientspace.Space, accState list.AccountState) error {
	id := domain.NewParticipantId(space.Id(), accState.PubKey.Account())
	return space.Do(id, func(sb smartblock.SmartBlock) error {
		return sb.(participant).ModifyParticipantAclState(spaceinfo.ParticipantAclInfo{
			Id:          id,
			SpaceId:     space.Id(),
			Identity:    accState.PubKey.Account(),
			Permissions: convertPermissions(accState.Permissions),
			Status:      convertStatus(accState.Status),
		})
	})
}

func (p *participantWatcher) updateParticipantFromIdentity(ctx context.Context, space clientspace.Space, identity string, profile *model.IdentityProfile) (err error) {
	id := domain.NewParticipantId(space.Id(), identity)
	return space.Do(id, func(sb smartblock.SmartBlock) error {
		return sb.(participant).ModifyIdentityDetails(profile)
	})
}

func (p *participantWatcher) Run(ctx context.Context) error {
	return nil
}

func getSymKey(metadata []byte) (crypto.SymKey, error) {
	md := &model.Metadata{}
	err := md.Unmarshal(metadata)
	if err != nil {
		return nil, err
	}
	keyProto := &cryptoproto.Key{}
	err = keyProto.UnmarshalVT(md.GetIdentity().GetProfileSymKey())
	if err != nil {
		return nil, err
	}
	return crypto.UnmarshallAESKey(keyProto.Data)
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
