package editor

import (
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// ParticipantRequiredRelations are participant-specific relation keys that must have
// relation links registered in the smartblock.
var ParticipantRequiredRelations = []domain.RelationKey{
	bundle.RelationKeyGlobalName,
	bundle.RelationKeyIdentity,
	bundle.RelationKeyBacklinks,
	bundle.RelationKeyParticipantPermissions,
	bundle.RelationKeyParticipantStatus,
	bundle.RelationKeyIdentityProfileLink,
	bundle.RelationKeyIsHiddenDiscovery,
}

// participant is a read-only view over a store-only object: all participant data
// lives in the per-space object index and is written there directly by
// participantwatcher and the identity service. This editor only materializes the
// stored details into a state for ObjectOpen/ObjectShow.
type participant struct {
	smartblock.SmartBlock
	objectStore spaceindex.Store
}

func (f *ObjectFactory) newParticipant(sb smartblock.SmartBlock, spaceIndex spaceindex.Store) *participant {
	return &participant{
		SmartBlock:  sb,
		objectStore: spaceIndex,
	}
}

func (p *participant) Init(ctx *smartblock.InitContext) (err error) {
	s := ctx.Doc.(*state.State)
	s.SetObjectTypeKey(bundle.TypeKeyParticipant)
	records, err := p.objectStore.QueryByIds([]string{s.RootId()})
	if err == nil && len(records) > 0 {
		s.SetDetails(records[0].Details)
	}

	// always overwrite this
	s.SetDetailAndBundledRelation(bundle.RelationKeyLayout, domain.Int64(int64(model.ObjectType_participant)))
	s.SetDetailAndBundledRelation(bundle.RelationKeyIsReadonly, domain.Bool(true))
	s.SetDetailAndBundledRelation(bundle.RelationKeyIsArchived, domain.Bool(false))
	s.SetDetailAndBundledRelation(bundle.RelationKeyIsHidden, domain.Bool(false))
	s.SetDetailAndBundledRelation(bundle.RelationKeyLayoutAlign, domain.Int64(model.Block_AlignCenter))

	// Add relation links for all detail keys so SmartBlock.Init doesn't produce a diff
	var relKeys []domain.RelationKey
	for _, k := range s.Details().Keys() {
		if bundle.HasRelation(k) {
			relKeys = append(relKeys, k)
		}
	}
	for _, k := range s.LocalDetails().Keys() {
		if bundle.HasRelation(k) {
			relKeys = append(relKeys, k)
		}
	}

	// Also add participant-specific required relations that SmartBlock.Init will add to the child
	relKeys = append(relKeys, ParticipantRequiredRelations...)
	s.AddBundledRelationLinks(relKeys...)

	template.InitTemplate(s,
		template.WithEmpty,
		template.WithTitle,
		template.WithDescription,
		template.WithFeaturedRelationsBlock,
	)

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	return nil
}
