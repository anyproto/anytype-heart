package v2service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestPatchReceiptIDsMatchReads(t *testing.T) {
	for _, shape := range []string{V2IdsCompact, V2IdsFull} {
		t.Run(shape, func(t *testing.T) {
			ctx := context.Background()
			if shape == V2IdsFull {
				ctx = CtxWithFullIds(ctx)
			}

			t.Run("blocks and table internals", func(t *testing.T) {
				fx := newV2Fixture(t)
				read := editRead(t, editEmptyDoc)
				captured := fx.expectMutate(read, "headB")
				result, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(
					`{"op":"insert_blocks","blocks":[{"type":"table","columns":[{},{}],`+
						`"rows":[{"is_header":true,"cells":["A","B"]},{"cells":[[`+
						`{"type":"paragraph","text":"root"},{"indent":1,"type":"paragraph","text":"child"}],"value"]}]},`+
						`{"type":"paragraph","text":"tail"}]}`), "", false, true)
				require.NoError(t, err)
				// The next GET reads the committed snapshot, as it does in production.
				*read.Snapshot = *snapshotFromState(*captured)
				body, _, err := fx.GetObject(ctx, testSpaceId, "obj1", ObjectQuery{Ids: shape})
				require.NoError(t, err)
				blocks := docBlocks(mustDoc(t, body))
				table := blocks[0]
				cols, rows := columnIdsOf(table), rowIdsOf(table)
				assert.Equal(t, map[string]string{
					"ops[0].blocks[0]":                     blockId(table),
					"ops[0].blocks[0].columns[0]":          cols[0].(string),
					"ops[0].blocks[0].columns[1]":          cols[1].(string),
					"ops[0].blocks[0].rows[0]":             rows[0].(string),
					"ops[0].blocks[0].rows[1]":             rows[1].(string),
					"ops[0].blocks[0].rows[1].cells[0][1]": cellDescendantIds(t, table)[0],
					"ops[0].blocks[1]":                     blockId(blocks[1]),
				}, result.CreatedBlocks)
				assert.Empty(t, result.CreatedViews)
				assert.Len(t, blockId(docBlocks(stateDoc(t, *captured))[1]), 24, "stored IDs remain full")
				if shape == V2IdsCompact {
					assert.Len(t, result.CreatedBlocks["ops[0].blocks[1]"], 5)
				}
				// The response's spelling is directly usable in a subsequent PATCH.
				_, err = fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(fmt.Sprintf(
					`{"op":"update_block","id":%q,"set":{"text":"edited via receipt"}}`, result.CreatedBlocks["ops[0].blocks[1]"])), "", false, true)
				require.NoError(t, err)
				assert.Equal(t, "edited via receipt", docBlocks(stateDoc(t, *captured))[1]["text"])
			})

			for _, tc := range []struct{ name, op, path string }{
				{"insert_view", `{"op":"insert_view","name":"Fresh"}`, "ops[0]"},
				{"update_block", `{"op":"update_block","id":"dataview","set":{"views":[{"id":"viewAll1","name":"All"},{"name":"Fresh"}]}}`, "ops[0].set.views[1]"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					fx := newV2Fixture(t)
					read := editRead(t, editSetDoc)
					read.Snapshot.Details.Fields[bundle.RelationKeyResolvedLayout.String()] = pbtypes.Int64(int64(model.ObjectType_set))
					captured := fx.expectMutate(read, "headB")
					result, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(tc.op), "", false, true)
					require.NoError(t, err)
					*read.Snapshot = *snapshotFromState(*captured)
					// The live reader supplies this derived detail; the editing
					// state's persistent details do not carry it.
					read.Snapshot.Details.Fields[bundle.RelationKeyResolvedLayout.String()] = pbtypes.Int64(int64(model.ObjectType_set))
					body, _, err := fx.GetObject(ctx, testSpaceId, "obj1", ObjectQuery{Ids: shape})
					require.NoError(t, err)
					views := viewsOf(t, docBlocks(mustDoc(t, body))[0])
					assert.Equal(t, map[string]string{tc.path: views[1]["id"].(string)}, result.CreatedViews)
					listed, _, _, err := fx.GetQueryViews(ctx, testSpaceId, "obj1", 0, 100)
					require.NoError(t, err)
					var view map[string]any
					require.NoError(t, json.Unmarshal(listed[1], &view))
					assert.Equal(t, result.CreatedViews[tc.path], view["id"], "receipt also matches the view-list endpoint")
					assert.Empty(t, result.CreatedBlocks)
					if shape == V2IdsCompact {
						assert.Len(t, result.CreatedViews[tc.path], 5)
					} else {
						assert.Len(t, result.CreatedViews[tc.path], 24)
					}
				})
			}

			t.Run("dry run", func(t *testing.T) {
				fx := newV2Fixture(t)
				read := editRead(t, editEmptyDoc)
				fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil)
				result, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(
					`{"op":"insert_blocks","markdown":"one\n\ntwo"}`), "", true, true)
				require.NoError(t, err)
				assert.True(t, result.DryRun)
				require.Len(t, result.CreatedBlocks, 2)
				for _, id := range result.CreatedBlocks {
					if shape == V2IdsCompact {
						assert.Len(t, id, 5)
					} else {
						assert.Len(t, id, 24)
					}
				}
				assert.Empty(t, docBlocks(snapshotDoc(t, read.Snapshot)), "dry-run state is private")
				fx.mutatorMock.AssertNumberOfCalls(t, "MutateObject", 0)
			})
		})
	}
}

func TestCompactReceiptIDsUseFinalDocumentCollisions(t *testing.T) {
	fx := newV2Fixture(t)
	const collided = "1111111111111111117ffff9"
	const reserved = "0000000000000000000aaaa1"
	const unique = "0000000000000000000bbbb1"
	read := editRead(t, `{"formatVersion":"2.0","id":"obj1","type":"page","blocks":[`+
		`{"id":"`+collided+`","type":"paragraph","text":"new"},`+
		`{"id":"2222222222222222227ffff9","type":"paragraph","text":"other"},`+
		`{"id":"`+reserved+`","type":"paragraph","text":"new reserved"},`+
		`{"id":"aaaa1","type":"paragraph","text":"existing meaningful id"},`+
		`{"id":"`+unique+`","type":"paragraph","text":"unique"}]}`)
	edit, err := editFromRead("obj1", read)
	require.NoError(t, err)
	a := newV2StateApplier(fx.Service, testSpaceId, "obj1", edit.SbType, edit.State,
		fx.newCreatingResolvers(context.Background(), testSpaceId, false, true), errKeysFor(context.Background()))
	a.createdBlocks = map[string]string{"collision": collided, "reserved": reserved, "unique": unique, "removed": "0000000000000000000ccccc"}
	blocks, views, err := a.compactReceiptIDs()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"collision": collided, "reserved": reserved, "unique": "bbbb1", "removed": "0000000000000000000ccccc"}, blocks)
	assert.Empty(t, views)
	assert.Equal(t, unique, a.createdBlocks["unique"], "the applier retains stored identities")
}
