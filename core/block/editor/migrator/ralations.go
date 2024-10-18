package migrator

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type MigrationRelations struct {
	space smartblock.Space
}

func NewMigrationRelations(space smartblock.Space) Migrator {
	return &MigrationRelations{space: space}
}

func (m *MigrationRelations) Migrate(s *state.State) {
	if layout, _ := s.Layout(); layout == model.ObjectType_relation {
		relationKey := pbtypes.GetString(s.CombinedDetails(), bundle.RelationKeyUniqueKey.String())
		relationFormat := pbtypes.GetInt64(s.CombinedDetails(), bundle.RelationKeyRelationFormat.String())
		if relationKey == bundle.RelationKeyTag.URL() && relationFormat == int64(model.RelationFormat_tag) {
			m.migrateTag(s)
		}
	}
}

func (m *MigrationRelations) migrateTag(s *state.State) {
	uniqueKey := domain.MustUniqueKey(coresb.SmartBlockTypeObjectType, bundle.TypeKeyTag.String())
	tagTypeId, err := m.space.DeriveObjectID(context.Background(), uniqueKey)
	if err != nil {
		log.Errorf("failed to migrate state: %v", err)
	} else {
		s.SetDetail(bundle.RelationKeyRelationFormatObjectTypes.String(), pbtypes.StringList([]string{tagTypeId}))
	}
	s.SetDetail(bundle.RelationKeyRelationFormat.String(), pbtypes.Int64(int64(model.RelationFormat_object)))
}
