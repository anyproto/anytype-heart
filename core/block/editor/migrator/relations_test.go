package migrator

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func Test_migrateRelation(t *testing.T) {
	t.Run("not relation", func(t *testing.T) {
		// given
		migrator := NewMigrationRelations(nil)
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_basic)))

		// when
		migrator.Migrate(s)

		// then
		assert.Equal(t, int64(model.ObjectType_basic), pbtypes.GetInt64(s.Details(), bundle.RelationKeyLayout.String()))
	})
	t.Run("not tag relation", func(t *testing.T) {
		// given
		migrator := NewMigrationRelations(nil)
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_relation)))
		s.SetDetail(bundle.RelationKeyUniqueKey.String(), pbtypes.String("key"))

		// when
		migrator.Migrate(s)

		// then
		assert.Equal(t, int64(model.ObjectType_relation), pbtypes.GetInt64(s.CombinedDetails(), bundle.RelationKeyLayout.String()))
		assert.Equal(t, "key", pbtypes.GetString(s.CombinedDetails(), bundle.RelationKeyUniqueKey.String()))
	})
	t.Run("not tag format relation", func(t *testing.T) {
		// given
		migrator := NewMigrationRelations(nil)
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_relation)))
		s.SetDetail(bundle.RelationKeyUniqueKey.String(), pbtypes.String(bundle.RelationKeyTag.URL()))
		s.SetDetail(bundle.RelationKeyRelationFormat.String(), pbtypes.Int64(int64(model.RelationFormat_object)))

		// when
		migrator.Migrate(s)

		// then
		assert.Equal(t, int64(model.ObjectType_relation), pbtypes.GetInt64(s.CombinedDetails(), bundle.RelationKeyLayout.String()))
		assert.Equal(t, bundle.RelationKeyTag.URL(), pbtypes.GetString(s.CombinedDetails(), bundle.RelationKeyUniqueKey.String()))
		assert.Equal(t, int64(model.RelationFormat_object), pbtypes.GetInt64(s.CombinedDetails(), bundle.RelationKeyRelationFormat.String()))
	})
	t.Run("migrate relation", func(t *testing.T) {
		// given
		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().DeriveObjectID(context.Background(), domain.MustUniqueKey(coresb.SmartBlockTypeObjectType, bundle.TypeKeyTag.String())).Return("id", nil)
		migrator := NewMigrationRelations(space)
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_relation)))
		s.SetDetail(bundle.RelationKeyUniqueKey.String(), pbtypes.String(bundle.RelationKeyTag.URL()))
		s.SetDetail(bundle.RelationKeyRelationFormat.String(), pbtypes.Int64(int64(model.RelationFormat_tag)))

		// when
		migrator.Migrate(s)

		// then
		assert.Equal(t, int64(model.RelationFormat_object), pbtypes.GetInt64(s.CombinedDetails(), bundle.RelationKeyRelationFormat.String()))
		assert.Equal(t, []string{"id"}, pbtypes.GetStringList(s.CombinedDetails(), bundle.RelationKeyRelationFormatObjectTypes.String()))
	})
	t.Run("type tage not exist", func(t *testing.T) {
		// given
		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().DeriveObjectID(context.Background(), domain.MustUniqueKey(coresb.SmartBlockTypeObjectType, bundle.TypeKeyTag.String())).Return("", fmt.Errorf("error"))

		migrator := NewMigrationRelations(space)

		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_relation)))
		s.SetDetail(bundle.RelationKeyUniqueKey.String(), pbtypes.String(bundle.RelationKeyTag.URL()))
		s.SetDetail(bundle.RelationKeyRelationFormat.String(), pbtypes.Int64(int64(model.RelationFormat_tag)))

		// when
		migrator.Migrate(s)

		// then
		assert.Equal(t, int64(model.RelationFormat_object), pbtypes.GetInt64(s.CombinedDetails(), bundle.RelationKeyRelationFormat.String()))
		assert.Len(t, pbtypes.GetStringList(s.CombinedDetails(), bundle.RelationKeyRelationFormatObjectTypes.String()), 0)
	})
}
