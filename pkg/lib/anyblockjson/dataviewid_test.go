package anyblockjson

// The primary dataview keeps the editor's fixed block id (§7). Without it the
// editor's WithDataviewIDIfNotExists finds no "dataview" block and adds a
// second, empty one next to the configured one.

import (
	"fmt"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// blockIds returns every non-root block id in document order.
func blockIds(t *testing.T, snap *model.SmartBlockSnapshotBase, rootId string) []string {
	t.Helper()
	ids := make([]string, 0, len(snap.Blocks))
	for _, b := range snap.Blocks {
		if b.Id != rootId {
			ids = append(ids, b.Id)
		}
	}
	return ids
}

func importDoc(t *testing.T, doc string) *model.SmartBlockSnapshotBase {
	t.Helper()
	_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	return snap
}

func TestImport_PrimaryDataviewGetsFixedId(t *testing.T) {
	t.Run("type document", func(t *testing.T) {
		doc := `{"version": 1, "kind": "objectType", "id": "t1", "key": "wikiCategory",
			"blocks": [{"type": "dataview", "views": [{"name": "All"}]}]}`
		snap := importDoc(t, doc)
		assert.Equal(t, []string{"dataview"}, blockIds(t, snap, "t1"))
	})

	// sets and collections are kind:page — the convention is not type-specific,
	// so the rule must not key on kind.
	t.Run("collection document", func(t *testing.T) {
		doc := `{"version": 1, "id": "c1", "type": "collection",
			"blocks": [{"type": "dataview", "isCollection": true, "views": [{"name": "All"}]}]}`
		snap := importDoc(t, doc)
		assert.Equal(t, []string{"dataview"}, blockIds(t, snap, "c1"))
	})

	// objectId means the block views *another* set: an inline dataview, which
	// must keep a generated id or it would shadow the object's own.
	t.Run("inline view keeps generated id", func(t *testing.T) {
		doc := `{"version": 1, "id": "p1",
			"blocks": [{"type": "dataview", "objectId": "otherSet", "views": [{"name": "All"}]}]}`
		snap := importDoc(t, doc)
		assert.Equal(t, []string{"g1"}, blockIds(t, snap, "p1"))
	})

	t.Run("nested dataview keeps generated id", func(t *testing.T) {
		doc := `{"version": 1, "id": "p1", "blocks": [
			{"type": "callout", "text": "wrapper"},
			{"type": "dataview", "indent": 1, "views": [{"name": "All"}]}]}`
		snap := importDoc(t, doc)
		assert.Equal(t, []string{"g1", "g2"}, blockIds(t, snap, "p1"))
	})

	t.Run("only the first is pinned", func(t *testing.T) {
		doc := `{"version": 1, "kind": "objectType", "id": "t1", "key": "k", "blocks": [
			{"type": "dataview", "views": [{"name": "A"}]},
			{"type": "dataview", "views": [{"name": "B"}]}]}`
		snap := importDoc(t, doc)
		ids := blockIds(t, snap, "t1")
		require.Len(t, ids, 2)
		assert.Equal(t, "dataview", ids[0])
		assert.NotEqual(t, "dataview", ids[1])
	})

	// an explicit id stays authoritative; pinning must not mint a duplicate.
	t.Run("explicit claim wins", func(t *testing.T) {
		doc := `{"version": 1, "kind": "objectType", "id": "t1", "key": "k", "blocks": [
			{"type": "dataview", "views": [{"name": "A"}]},
			{"type": "dataview", "id": "dataview", "views": [{"name": "B"}]}]}`
		snap := importDoc(t, doc)
		ids := blockIds(t, snap, "t1")
		require.Len(t, ids, 2)
		assert.NotEqual(t, "dataview", ids[0])
		assert.Equal(t, "dataview", ids[1])
	})
}

// omitIds used to break every dataview-backed object: the export dropped the
// fixed id and the re-import could not put it back.
func TestRoundtrip_OmitIdsKeepsPrimaryDataview(t *testing.T) {
	snapshot := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "t1", ChildrenIds: []string{"dataview"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "dataview", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{{
					Id:   "v1",
					Name: "Section directory",
					Relations: []*model.BlockContentDataviewRelation{
						{Key: "name", IsVisible: true, Width: 180},
					},
				}},
			}}},
		},
		Details: &types.Struct{Fields: map[string]*types.Value{
			"id": {Kind: &types.Value_StringValue{StringValue: "t1"}},
		}},
		Key: "wikiCategory",
	}

	for _, omit := range []bool{false, true} {
		t.Run(fmt.Sprintf("omitIds=%v", omit), func(t *testing.T) {
			opts := testOptions()
			opts.OmitIds = omit
			data, err := Marshal(model.SmartBlockType_STType, snapshot, opts)
			require.NoError(t, err)

			_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
			require.NoError(t, err)

			var dv *model.Block
			for _, b := range back.Blocks {
				if b.GetDataview() != nil {
					dv = b
				}
			}
			require.NotNil(t, dv, "dataview block survived")
			assert.Equal(t, "dataview", dv.Id)
			require.Len(t, dv.GetDataview().Views, 1)
			require.Len(t, dv.GetDataview().Views[0].Relations, 1)
			assert.Equal(t, int32(180), dv.GetDataview().Views[0].Relations[0].Width)
		})
	}
}
