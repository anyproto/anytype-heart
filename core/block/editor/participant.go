package editor

import (
	"time"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type participant struct {
	smartblock.SmartBlock
	basic.AllOperations
}

func (f *ObjectFactory) newParticipant(sb smartblock.SmartBlock) *participant {
	return &participant{
		SmartBlock:    sb,
		AllOperations: basic.NewBasic(sb, f.objectStore, f.layoutConverter),
	}
}

func (p *participant) Init(ctx *smartblock.InitContext) (err error) {
	// Details come from aclobjectmanager, see buildParticipantDetails

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

func (p *participant) TryClose(objectTTL time.Duration) (bool, error) {
	return false, nil
}

func (p *participant) SetDetails(ctx session.Context, details []*pb.RpcObjectSetDetailsDetail, showEvent bool) (err error) {
	return p.AllOperations.SetDetails(ctx, details, showEvent)
}
