package editor

import (
	"fmt"
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
	objectStore    spaceindex.Store
	accountService accountService
}

func (f *ObjectFactory) newParticipant(spaceId string, sb smartblock.SmartBlock, spaceIndex spaceindex.Store) *participant {
	basicComponent := basic.NewBasic(sb, spaceIndex, f.layoutConverter, nil)
	return &participant{
		SmartBlock:       sb,
		DetailsUpdatable: basicComponent,
		objectStore:      spaceIndex,
		accountService:   f.accountService,
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
	ctx.State.SetDetailAndBundledRelation(bundle.RelationKeyLayoutAlign, domain.Int64(model.Block_AlignCenter))

	records, err := p.objectStore.QueryByIds([]string{p.Id()})
	if err != nil {
		return err
	}
	if len(records) > 0 {
		ctx.State.SetDetails(records[0].Details)
	}
	template.InitTemplate(ctx.State,
		template.WithEmpty,
		template.WithTitle,
		template.WithDescription,
		template.WithFeaturedRelationsBlock,
		template.WithLayout(model.ObjectType_participant),
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

func (p *participant) ModifyParticipantAclState(accState spaceinfo.ParticipantAclInfo) error {
	details, err := p.buildParticipantDetails(accState)
	if err != nil {
		return fmt.Errorf("build participant details: %w", err)
	}
	return p.modifyDetails(details)
}

func (p *participant) TryClose(objectTTL time.Duration) (bool, error) {
	return false, nil
}

func (p *participant) modifyDetails(newDetails *domain.Details) (err error) {
	return p.DetailsUpdatable.UpdateDetails(nil, func(current *domain.Details) (*domain.Details, error) {
		return current.Merge(newDetails), nil
	})
}

func (p *participant) buildParticipantDetails(
	accState spaceinfo.ParticipantAclInfo,
) (*domain.Details, error) {
	det := domain.NewDetails()
	det.SetString(bundle.RelationKeyId, accState.Id)
	det.SetString(bundle.RelationKeyIdentity, accState.Identity)
	det.SetString(bundle.RelationKeySpaceId, accState.SpaceId)
	det.SetString(bundle.RelationKeyLastModifiedBy, accState.Id)
	det.SetInt64(bundle.RelationKeyParticipantPermissions, int64(accState.Permissions))
	det.SetInt64(bundle.RelationKeyParticipantStatus, int64(accState.Status))
	det.SetBool(bundle.RelationKeyIsHiddenDiscovery, accState.Status != model.ParticipantStatus_Active)
	if p.accountService.MyParticipantId(p.SpaceID()) == p.Id() {
		accountObjectId, err := p.accountService.GetAccountObjectId()
		if err != nil {
			return nil, fmt.Errorf("get account object id: %w", err)
		}
		det.SetString(bundle.RelationKeyIdentityProfileLink, accountObjectId)
	}
	return det, nil
}
