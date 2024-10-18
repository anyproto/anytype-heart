package migrator

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestMigrationTags_Migrate(t *testing.T) {
	t.Run("not relation option", func(t *testing.T) {
		// given
		migrator := NewMigrationTags()
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_basic)))

		// when
		migrator.Migrate(s)

		// then
		assert.Equal(t, int64(model.ObjectType_basic), pbtypes.GetInt64(s.Details(), bundle.RelationKeyLayout.String()))
	})
	t.Run("not tag relation option", func(t *testing.T) {
		// given
		migrator := NewMigrationTags()
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_relationOption)))
		s.SetDetail(bundle.RelationKeyRelationKey.String(), pbtypes.String("key"))

		// when
		migrator.Migrate(s)

		// then
		assert.Equal(t, int64(model.ObjectType_relationOption), pbtypes.GetInt64(s.Details(), bundle.RelationKeyLayout.String()))
		assert.Equal(t, "key", pbtypes.GetString(s.Details(), bundle.RelationKeyRelationKey.String()))
	})
	t.Run("migrate relation option", func(t *testing.T) {
		// given
		migrator := NewMigrationTags()
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_relationOption)))
		s.SetDetail(bundle.RelationKeyRelationKey.String(), pbtypes.String(bundle.RelationKeyTag.String()))
		s.SetDetail(bundle.RelationKeyUniqueKey.String(), pbtypes.String("key"))

		// when
		migrator.Migrate(s)

		// then
		assert.Equal(t, int64(model.ObjectType_tag), pbtypes.GetInt64(s.CombinedDetails(), bundle.RelationKeyLayout.String()))
		assert.Empty(t, pbtypes.GetString(s.CombinedDetails(), bundle.RelationKeyUniqueKey.String()))
		assert.Empty(t, pbtypes.GetString(s.CombinedDetails(), bundle.RelationKeyRelationKey.String()))
	})
}
