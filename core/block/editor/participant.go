package editor

import (
	"time"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple/link"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type participant struct {
	smartblock.SmartBlock
}

func (f *ObjectFactory) newParticipant(sb smartblock.SmartBlock) *participant {
	return &participant{
		SmartBlock: sb,
	}
}

func (p *participant) Init(ctx *smartblock.InitContext) (err error) {
	// Details come from aclobjectmanager, see buildParticipantDetails

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	if p.Space().IsPersonal() {
		p.AddHook(func(info smartblock.ApplyInfo) (err error) {
			addTempLinkToOldProfile(info.State)
			return nil
		}, smartblock.HookBeforeApply)
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

func (p *participant) TryClose(objectTTL time.Duration) (bool, error) {
	return false, nil
}

func addTempLinkToOldProfile(state *state.State) {
	profileId := pbtypes.GetString(state.CombinedDetails(), bundle.RelationKeyIdentityProfileLink.String())
	if profileId == "" {
		return
	}
	id := "link_to_profile"
	if state.Get(id) != nil {
		return
	}
	if state.Get(state.RootId()) == nil {
		return
	}

	b := link.NewLink(&model.Block{
		Id: id,
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: profileId,
				CardStyle:     model.BlockContentLink_Card,
			},
		},
	})
	state.Add(b)
	_ = state.InsertTo("featuredRelations", model.Block_Bottom, id)
}
