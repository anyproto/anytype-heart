package migrator

import (
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type MigrationTags struct{}

func NewMigrationTags() Migrator {
	return &MigrationTags{}
}

func (m *MigrationTags) Migrate(s *state.State) {
	if layout, _ := s.Layout(); layout == model.ObjectType_relationOption {
		relationKey := pbtypes.GetString(s.CombinedDetails(), bundle.RelationKeyRelationKey.String())
		if relationKey != bundle.RelationKeyTag.String() {
			return
		}
		s.SetObjectTypeKey(bundle.TypeKeyTag)
		s.RemoveRelation(bundle.RelationKeyRelationKey.String())
		s.RemoveRelation(bundle.RelationKeyUniqueKey.String())
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_tag)))
	}
}
