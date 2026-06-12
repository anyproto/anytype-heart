package participantwatcher

/*
AI generated

Name: Participant Identity Sync
Scope: space

## Responsibility
- Registers identity observers to receive profile updates for space participants
- Updates participant objects with identity profile data (name, avatar, etc.)
- Updates participant objects with ACL state (permissions, status)
- Handles one-to-one space metadata key retrieval separately from regular spaces

## Documentation
Called by aclobjectmanager for each account in ACL state. WatchParticipant registers
an identity observer that triggers updateParticipantFromIdentity on profile changes.
On Close, unregisters all identity observers for the space.

Participant objects are store-only: all writes go directly to the per-space object
index (ModifyObjectDetails merge), never through a smartblock. The merge fires
subscription events only on real changes; fulltext indexing is enqueued only when
name or description changed (the only participant keys producing fulltext docs).
*/

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

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/participants"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
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
	// WatchPersistedParticipants registers all participant identities found in the
	// space's object index for identity tracking, using previously persisted
	// encryption keys. Used on the skip path when the ACL head hasn't changed since
	// the last full processing; an error means the caller must fall back to full
	// ACL processing.
	WatchPersistedParticipants(ctx context.Context, space clientspace.Space) error
	// GetProcessedAclHeadId returns the ACL head the participants of the space were
	// last fully processed for; empty when never processed
	GetProcessedAclHeadId(ctx context.Context, space clientspace.Space) string
	// SetProcessedAclHeadId persists the ACL head after a full participants processing pass
	SetProcessedAclHeadId(ctx context.Context, space clientspace.Space, headId string) error
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

func (p *participantWatcher) getOneToOneParticipantRequestMetadataKey(space clientspace.Space) (crypto.SymKey, error) {
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

	requestMetadataKeyStr := records[0].Details.GetString(bundle.RelationKeyOneToOneRequestMetadataKey)
	requestMetadataKeyBytes, rerr := base64.StdEncoding.DecodeString(requestMetadataKeyStr)
	if rerr != nil {
		return nil, fmt.Errorf("failed to decode bob onetoone RequestMetadata: %w", rerr)
	}
	key, err := crypto.UnmarshallAESKeyProto(requestMetadataKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("onetoone: failed to unmarshal requestMetadataBytes")
	}

	return key, nil
}

func (p *participantWatcher) getOneToOneKey(space clientspace.Space, state list.AccountState) (key crypto.SymKey, err error) {
	myPubKey := p.accountService.Account().SignKey.GetPublic()
	// it is either me or bob: we don't call WatchParticipant with owner state in aclobjectmanager
	if state.PubKey.Equals(myPubKey) {
		key, err = p.identityService.GetMetadataKey(myPubKey.Account())
		if err != nil {
			return
		}
	} else {
		key, err = p.getOneToOneParticipantRequestMetadataKey(space)
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
		key, err = p.getOneToOneKey(space, state)
	} else {
		key, err = getSymKey(state.RequestMetadata)
	}
	if err != nil {
		return
	}

	err = p.identityService.RegisterIdentity(space.Id(), state.PubKey.Account(), key)
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

func (p *participantWatcher) UpdateParticipantFromAclState(ctx context.Context, space clientspace.Space, accState list.AccountState) error {
	id := domain.NewParticipantId(space.Id(), accState.PubKey.Account())
	details, err := p.buildAclDetails(spaceinfo.ParticipantAclInfo{
		Id:          id,
		SpaceId:     space.Id(),
		Identity:    accState.PubKey.Account(),
		Permissions: convertPermissions(accState.Permissions),
		Status:      convertStatus(accState.Status),
	})
	if err != nil {
		return fmt.Errorf("build participant acl details: %w", err)
	}
	return p.modifyParticipant(ctx, space, id, details)
}

// modifyParticipant merges newDetails into the participant record in the per-space
// object index. Participants have no smartblock state: the store is the only
// persistence, and subscription events fire from the store merge itself.
func (p *participantWatcher) modifyParticipant(ctx context.Context, space clientspace.Space, id string, newDetails *domain.Details) error {
	baseDetails, err := p.buildBaseDetails(ctx, space, id)
	if err != nil {
		return fmt.Errorf("build participant base details: %w", err)
	}
	return participants.ModifyDetails(ctx, p.objectStore, space.Id(), id, baseDetails, newDetails)
}

// buildBaseDetails returns the details every participant record must carry; they are
// written once on record creation (parity with the former smartblock Init + template).
func (p *participantWatcher) buildBaseDetails(ctx context.Context, space clientspace.Space, id string) (*domain.Details, error) {
	typeId, err := space.GetTypeIdByKey(ctx, bundle.TypeKeyParticipant)
	if err != nil {
		return nil, fmt.Errorf("get participant type id: %w", err)
	}
	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyId, id)
	details.SetString(bundle.RelationKeySpaceId, space.Id())
	details.SetString(bundle.RelationKeyType, typeId)
	details.SetString(bundle.RelationKeyCreator, addr.AnytypeProfileId)
	details.SetInt64(bundle.RelationKeyResolvedLayout, int64(model.ObjectType_participant))
	details.SetInt64(bundle.RelationKeyLayoutAlign, int64(model.Block_AlignCenter))
	details.SetBool(bundle.RelationKeyIsReadonly, true)
	details.SetBool(bundle.RelationKeyIsArchived, false)
	details.SetBool(bundle.RelationKeyIsHidden, false)
	return details, nil
}

func (p *participantWatcher) buildAclDetails(info spaceinfo.ParticipantAclInfo) (*domain.Details, error) {
	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyId, info.Id)
	details.SetString(bundle.RelationKeyIdentity, info.Identity)
	details.SetString(bundle.RelationKeySpaceId, info.SpaceId)
	details.SetString(bundle.RelationKeyLastModifiedBy, info.Id)
	details.SetInt64(bundle.RelationKeyParticipantPermissions, int64(info.Permissions))
	details.SetInt64(bundle.RelationKeyParticipantStatus, int64(info.Status))
	details.SetBool(bundle.RelationKeyIsHiddenDiscovery, info.Status != model.ParticipantStatus_Active)
	if p.myParticipantId(info.SpaceId) == info.Id {
		accountObjectId, err := p.techSpace.AccountObjectId()
		if err != nil {
			return nil, fmt.Errorf("get account object id: %w", err)
		}
		details.SetString(bundle.RelationKeyIdentityProfileLink, accountObjectId)
	}
	return details, nil
}

func (p *participantWatcher) WatchPersistedParticipants(ctx context.Context, space clientspace.Space) error {
	records, err := p.objectStore.SpaceIndex(space.Id()).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_participant)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query participants: %w", err)
	}
	p.mx.Lock()
	defer p.mx.Unlock()
	for _, record := range records {
		identity := record.Details.GetString(bundle.RelationKeyIdentity)
		if identity == "" {
			continue
		}
		if _, exists := p.addedParticipants[identity]; exists {
			continue
		}
		// nil key: reuse the key persisted during the last full ACL processing pass
		err = p.identityService.RegisterIdentity(space.Id(), identity, nil)
		if err != nil {
			return fmt.Errorf("register identity %s with persisted key: %w", identity, err)
		}
		p.addedParticipants[identity] = struct{}{}
	}
	return nil
}

func (p *participantWatcher) GetProcessedAclHeadId(ctx context.Context, space clientspace.Space) string {
	headId, err := p.objectStore.SpaceIndex(space.Id()).GetLastIndexedHeadsHash(ctx, participants.AclHeadMarkerId)
	if err != nil {
		log.Warn("get processed acl head id", zap.Error(err))
		return ""
	}
	return headId
}

func (p *participantWatcher) SetProcessedAclHeadId(ctx context.Context, space clientspace.Space, headId string) error {
	err := p.objectStore.SpaceIndex(space.Id()).SaveLastIndexedHeadsHash(ctx, participants.AclHeadMarkerId, headId)
	if err != nil {
		return fmt.Errorf("save processed acl head id: %w", err)
	}
	return nil
}

func (p *participantWatcher) myParticipantId(spaceId string) string {
	return domain.NewParticipantId(spaceId, p.accountService.Account().SignKey.GetPublic().Account())
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
	case aclrecordproto.AclUserPermissions_Admin:
		return model.ParticipantPermissions_Admin
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
