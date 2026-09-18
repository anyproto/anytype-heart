package api

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestSnapshotRootId(t *testing.T) {
	t.Run("finds the smartblock root among the blocks", func(t *testing.T) {
		// given
		snapshot := &model.SmartBlockSnapshotBase{Blocks: []*model.Block{
			{Id: "p1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "hi"}}},
			{Id: "root1", Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
		}}

		// then
		assert.Equal(t, "root1", snapshotRootId(snapshot))
	})

	t.Run("no root and nil snapshot yield empty", func(t *testing.T) {
		assert.Empty(t, snapshotRootId(nil))
		assert.Empty(t, snapshotRootId(&model.SmartBlockSnapshotBase{Blocks: []*model.Block{{Id: "p1"}}}))
	})
}

func TestBundledIdsToInstall(t *testing.T) {
	t.Run("bundled keys map to their source ids, custom keys are skipped", func(t *testing.T) {
		// given
		relationKeys := []domain.RelationKey{bundle.RelationKeyDueDate, "customKey"}
		typeKeys := []domain.TypeKey{bundle.TypeKeyTask, "customType"}
		want := []string{
			bundle.RelationKeyDueDate.BundledURL(),
			"_ot" + string(bundle.TypeKeyTask),
		}

		// when
		got := bundledIdsToInstall(relationKeys, typeKeys)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("nothing bundled yields an empty list", func(t *testing.T) {
		assert.Empty(t, bundledIdsToInstall([]domain.RelationKey{"x"}, []domain.TypeKey{"y"}))
	})
}
