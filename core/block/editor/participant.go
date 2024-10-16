package editor

import (
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
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

	records, err := p.objectStore.QueryByIds([]string{p.Id()})
	if err != nil {
		return err
	}
	if len(records) > 0 {
		ctx.State.SetDetails(records[0].Details)
	}

	return nil
}

func (p *participant) ModifyProfileDetails(profileDetails *types.Struct) (err error) {
	details := pbtypes.CopyStructFields(profileDetails,
		bundle.RelationKeyName.String(),
		bundle.RelationKeyDescription.String(),
		bundle.RelationKeyIconImage.String(),
		bundle.RelationKeyGlobalName.String())
	details.Fields[bundle.RelationKeyIdentityProfileLink.String()] = pbtypes.String(pbtypes.GetString(profileDetails, bundle.RelationKeyId.String()))
	return p.modifyDetails(details)
}

func (p *participant) ModifyIdentityDetails(profile *model.IdentityProfile) (err error) {
	details := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():        pbtypes.String(profile.Name),
		bundle.RelationKeyDescription.String(): pbtypes.String(profile.Description),
		bundle.RelationKeyIconImage.String():   pbtypes.String(profile.IconCid),
		bundle.RelationKeyGlobalName.String():  pbtypes.String(profile.GlobalName),
	}}
	return p.modifyDetails(details)
}

func (p *participant) ModifyParticipantAclState(accState spaceinfo.ParticipantAclInfo) (err error) {
	details := buildParticipantDetails(accState.Id, accState.SpaceId, accState.Identity, accState.Permissions, accState.Status)
	return p.modifyDetails(details)
}

func (p *participant) TryClose(objectTTL time.Duration) (bool, error) {
	return false, nil
}

func (p *participant) modifyDetails(newDetails *types.Struct) (err error) {
	return p.DetailsUpdatable.UpdateDetails(func(current *types.Struct) (*types.Struct, error) {
		return pbtypes.StructMerge(current, newDetails, false), nil
	})
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
