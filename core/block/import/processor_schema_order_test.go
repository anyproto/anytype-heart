package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

func TestSplitSchemaSnapshots(t *testing.T) {
	snapshot := func(id string, sbType smartblock.SmartBlockType) *common.Snapshot {
		return &common.Snapshot{
			Id:       id,
			Snapshot: &common.SnapshotModel{SbType: sbType},
		}
	}
	ids := func(snapshots []*common.Snapshot) []string {
		res := make([]string, 0, len(snapshots))
		for _, s := range snapshots {
			res = append(res, s.Id)
		}
		return res
	}

	t.Run("types and relations are separated from the objects that depend on them", func(t *testing.T) {
		// given: an archive that interleaves objects with their schema, as exports do
		snapshots := []*common.Snapshot{
			snapshot("task", smartblock.SmartBlockTypePage),
			snapshot("taskType", smartblock.SmartBlockTypeObjectType),
			snapshot("project", smartblock.SmartBlockTypePage),
			snapshot("statusRelation", smartblock.SmartBlockTypeRelation),
			snapshot("doneOption", smartblock.SmartBlockTypeRelationOption),
			snapshot("template", smartblock.SmartBlockTypeTemplate),
		}
		want := struct {
			schema []string
			rest   []string
		}{
			schema: []string{"taskType", "statusRelation", "doneOption"},
			rest:   []string{"task", "project", "template"},
		}

		// when
		schema, rest := splitSchemaSnapshots(snapshots)

		// then
		assert.Equal(t, want.schema, ids(schema))
		assert.Equal(t, want.rest, ids(rest))
	})

	t.Run("archive without schema objects", func(t *testing.T) {
		// given
		snapshots := []*common.Snapshot{
			snapshot("task", smartblock.SmartBlockTypePage),
		}

		// when
		schema, rest := splitSchemaSnapshots(snapshots)

		// then
		assert.Empty(t, schema)
		assert.Equal(t, []string{"task"}, ids(rest))
	})
}
