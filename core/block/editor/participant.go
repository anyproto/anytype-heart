package editor

import (
	"time"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var participantRequiredRelations = []domain.RelationKey{
	bundle.RelationKeyGlobalName,
	bundle.RelationKeyIdentity,
	bundle.RelationKeyBacklinks,
	bundle.RelationKeyParticipantPermissions,
	bundle.RelationKeyParticipantStatus,
	bundle.RelationKeyIdentityProfileLink,
	bundle.RelationKeyIsHiddenDiscovery,
}

type participant struct {
	smartblock.SmartBlock
	basic.DetailsUpdatable
}

func (f *ObjectFactory) newParticipant(sb smartblock.SmartBlock) *participant {
	basicComponent := basic.NewBasic(sb, f.objectStore, f.layoutConverter, nil)
	return &participant{
		SmartBlock:       sb,
		DetailsUpdatable: basicComponent,
	}
}

func (p *participant) Init(ctx *smartblock.InitContext) (err error) {
	// Details come from aclobjectmanager, see buildParticipantDetails
	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, participantRequiredRelations...)

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}

	ctx.State.SetDetailAndBundledRelation(bundle.RelationKeyIsReadonly, pbtypes.Bool(true))
	ctx.State.SetDetailAndBundledRelation(bundle.RelationKeyIsArchived, pbtypes.Bool(false))
	ctx.State.SetDetailAndBundledRelation(bundle.RelationKeyIsHidden, pbtypes.Bool(false))
	ctx.State.SetDetailAndBundledRelation(bundle.RelationKeyLayout, pbtypes.Int64(int64(model.ObjectType_participant)))
	ctx.State.SetDetailAndBundledRelation(bundle.RelationKeyLayoutAlign, pbtypes.Int64(int64(model.Block_AlignCenter)))

	template.InitTemplate(ctx.State,
		template.WithEmpty,
		template.WithTitle,
		template.WithDescription,
		template.WithFeaturedRelations,
		template.WithAddedFeaturedRelation(bundle.RelationKeyType),
		template.WithAddedFeaturedRelation(bundle.RelationKeyBacklinks),
	)
	return nil
}

func (p *participant) ModifyProfileDetails(profileDetails *domain.Details) (err error) {
	details := profileDetails.CopyOnlyKeys(
		bundle.RelationKeyName,
		bundle.RelationKeyDescription,
		bundle.RelationKeyIconImage,
		bundle.RelationKeyGlobalName,
	)
	details.Set(bundle.RelationKeyIdentityProfileLink, pbtypes.String(profileDetails.GetStringOrDefault(bundle.RelationKeyId, "")))
	return p.modifyDetails(details)
}

func (p *participant) ModifyIdentityDetails(profile *model.IdentityProfile) (err error) {
	details := domain.NewDetailsFromMap(map[domain.RelationKey]any{
		bundle.RelationKeyName:        profile.Name,
		bundle.RelationKeyDescription: profile.Description,
		bundle.RelationKeyIconImage:   profile.IconCid,
		bundle.RelationKeyGlobalName:  profile.GlobalName,
	})
	return p.modifyDetails(details)
}

func (p *participant) ModifyParticipantAclState(accState spaceinfo.ParticipantAclInfo) (err error) {
	details := buildParticipantDetails(accState.Id, accState.SpaceId, accState.Identity, accState.Permissions, accState.Status)
	return p.modifyDetails(details)
}

func (p *participant) TryClose(objectTTL time.Duration) (bool, error) {
	return false, nil
}

func (p *participant) modifyDetails(newDetails *domain.Details) (err error) {
	return p.DetailsUpdatable.UpdateDetails(func(current *domain.Details) (*domain.Details, error) {
		return current.Merge(newDetails), nil
	})
}

func buildParticipantDetails(
	id string,
	spaceId string,
	identity string,
	permissions model.ParticipantPermissions,
	status model.ParticipantStatus,
) *domain.Details {
	return domain.NewDetailsFromMap(map[domain.RelationKey]any{
		bundle.RelationKeyId:                     id,
		bundle.RelationKeyIdentity:               identity,
		bundle.RelationKeySpaceId:                spaceId,
		bundle.RelationKeyLastModifiedBy:         id,
		bundle.RelationKeyParticipantPermissions: int64(permissions),
		bundle.RelationKeyParticipantStatus:      int64(status),
		bundle.RelationKeyIsHiddenDiscovery:      status != model.ParticipantStatus_Active,
	})
}
