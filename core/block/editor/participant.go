package editor

import (
	"time"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
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
	objectStore spaceindex.Store
}

func (f *ObjectFactory) newParticipant(spaceId string, sb smartblock.SmartBlock, spaceIndex spaceindex.Store) *participant {
	basicComponent := basic.NewBasic(sb, spaceIndex, f.layoutConverter, nil, f.lastUsedUpdater)
	return &participant{
		SmartBlock:       sb,
		DetailsUpdatable: basicComponent,
		objectStore:      spaceIndex,
	}
}

func (p *participant) Init(ctx *smartblock.InitContext) (err error) {
	// Details come from aclobjectmanager, see buildParticipantDetails
	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, participantRequiredRelations...)

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}

	ctx.State.SetDetailAndBundledRelation(bundle.RelationKeyIsReadonly, domain.Bool(true))
	ctx.State.SetDetailAndBundledRelation(bundle.RelationKeyIsArchived, domain.Bool(false))
	ctx.State.SetDetailAndBundledRelation(bundle.RelationKeyIsHidden, domain.Bool(false))
	ctx.State.SetDetailAndBundledRelation(bundle.RelationKeyLayout, domain.Int64(model.ObjectType_participant))
	ctx.State.SetDetailAndBundledRelation(bundle.RelationKeyLayoutAlign, domain.Int64(model.Block_AlignCenter))

	template.InitTemplate(ctx.State,
		template.WithEmpty,
		template.WithTitle,
		template.WithDescription,
		template.WithFeaturedRelations,
		template.WithAddedFeaturedRelation(bundle.RelationKeyType),
		template.WithAddedFeaturedRelation(bundle.RelationKeyBacklinks),
	)

	records, err := p.objectStore.QueryByIds([]string{p.Id()})
	if err != nil {
		return err
	}
	if len(records) > 0 {
		ctx.State.SetDetails(records[0].Details)
	}

	return nil
}

func (p *participant) ModifyProfileDetails(profileDetails *domain.Details) (err error) {
	details := profileDetails.CopyOnlyKeys(
		bundle.RelationKeyName,
		bundle.RelationKeyDescription,
		bundle.RelationKeyIconImage,
		bundle.RelationKeyGlobalName,
	)
	details.SetString(bundle.RelationKeyIdentityProfileLink, profileDetails.GetString(bundle.RelationKeyId))
	return p.modifyDetails(details)
}

func (p *participant) ModifyIdentityDetails(profile *model.IdentityProfile) (err error) {
	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyName, profile.Name)
	details.SetString(bundle.RelationKeyDescription, profile.Description)
	details.SetString(bundle.RelationKeyIconImage, profile.IconCid)
	details.SetString(bundle.RelationKeyGlobalName, profile.GlobalName)
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
	det := domain.NewDetails()
	det.SetString(bundle.RelationKeyId, id)
	det.SetString(bundle.RelationKeyIdentity, identity)
	det.SetString(bundle.RelationKeySpaceId, spaceId)
	det.SetString(bundle.RelationKeyLastModifiedBy, id)
	det.SetInt64(bundle.RelationKeyParticipantPermissions, int64(permissions))
	det.SetInt64(bundle.RelationKeyParticipantStatus, int64(status))
	det.SetBool(bundle.RelationKeyIsHiddenDiscovery, status != model.ParticipantStatus_Active)
	return det
}
