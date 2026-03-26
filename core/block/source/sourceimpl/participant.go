package sourceimpl

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// ParticipantRequiredRelations are participant-specific relation keys that must have
// relation links registered in the smartblock. Used both by the source (to pre-populate
// the parent state) and by participant.Init (to register with the smartblock framework).
var ParticipantRequiredRelations = []domain.RelationKey{
	bundle.RelationKeyGlobalName,
	bundle.RelationKeyIdentity,
	bundle.RelationKeyBacklinks,
	bundle.RelationKeyParticipantPermissions,
	bundle.RelationKeyParticipantStatus,
	bundle.RelationKeyIdentityProfileLink,
	bundle.RelationKeyIsHiddenDiscovery,
}

type participantSource struct {
	id      string
	spaceId string
	store   spaceindex.Store
}

func newParticipantSource(spaceId, id string, store spaceindex.Store) source.Source {
	return &participantSource{
		id:      id,
		spaceId: spaceId,
		store:   store,
	}
}

func (p *participantSource) Id() string {
	return p.id
}

func (p *participantSource) SpaceID() string {
	return p.spaceId
}

func (p *participantSource) Type() smartblock.SmartBlockType {
	return smartblock.SmartBlockTypeParticipant
}

func (p *participantSource) ReadOnly() bool {
	return true
}

func (p *participantSource) ReadDoc(_ context.Context, _ source.ChangeReceiver, _ bool) (doc state.Doc, err error) {
	s := state.NewDoc(p.id, nil).(*state.State)
	s.SetObjectTypeKey(bundle.TypeKeyParticipant)

	records, err := p.store.QueryByIds([]string{p.id})
	if err == nil && len(records) > 0 {
		s.SetDetails(records[0].Details)
	}

	s.SetDetailAndBundledRelation(bundle.RelationKeyIsReadonly, domain.Bool(true))
	s.SetDetailAndBundledRelation(bundle.RelationKeyIsArchived, domain.Bool(false))
	s.SetDetailAndBundledRelation(bundle.RelationKeyIsHidden, domain.Bool(false))
	s.SetDetailAndBundledRelation(bundle.RelationKeyLayoutAlign, domain.Int64(model.Block_AlignCenter))

	// Add relation links for all detail keys so SmartBlock.Init doesn't produce a diff
	var relKeys []domain.RelationKey
	for k := range s.Details().Iterate() {
		if bundle.HasRelation(k) {
			relKeys = append(relKeys, k)
		}
	}
	for k := range s.LocalDetails().Iterate() {
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
		template.WithLayout(model.ObjectType_participant),
	)

	return s, nil
}

func (p *participantSource) PushChange(source.PushChangeParams) (id string, err error) {
	return "", nil
}

func (p *participantSource) Close() error {
	return nil
}

func (p *participantSource) Heads() []string {
	return []string{"todo"} // todo: hash of details
}

func (p *participantSource) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (p *participantSource) GetCreationInfo() (creatorObjectId string, createdDate int64, err error) {
	return addr.AnytypeProfileId, 0, nil
}
