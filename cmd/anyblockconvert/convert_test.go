package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// convertDoc writes a document to a temp dir and converts it, the way the
// directory walk does.
func convertDoc(t *testing.T, b *batch, name, body string) (model.SmartBlockType, *model.SmartBlockSnapshotBase) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	_, sbType, snap, err := convertFile(dir, path, b, false, nil)
	require.NoError(t, err)
	return sbType, snap
}

// A template's target type has to reach the snapshot as the targetObjectType
// detail: that is the relation a type's templates are queried by
// (core/block/template/templateimpl.queryTemplatesByType). objectTypes[1]
// alone is a derived cache nothing reconstructs it from, so a template
// converted without the detail imports as an object no type lists.
func TestConvert_TemplateTargetTypeBecomesDetail(t *testing.T) {
	b := newBatch(nil, map[string]string{"wikiPage": "type-wiki-page"})
	sbType, snap := convertDoc(t, b, "wiki-article.template.json", `{
	  "version": 2,
	  "kind": "template",
	  "id": "template-wiki-article",
	  "type": "template",
	  "template_for": "wikiPage",
	  "properties": {"name": "Wiki Article"}
	}`)

	require.Equal(t, model.SmartBlockType_Template, sbType)
	assert.Equal(t, []string{"ot-template", "ot-wikiPage"}, snap.ObjectTypes)
	assert.Equal(t, "type-wiki-page",
		snap.Details.GetFields()[detailTargetObjectType].GetStringValue(),
		"the target type document's own id, so the pb importer relinks it")
}

// An authored targetObjectType (what a round-tripped export carries) stays
// authoritative, and lands as a plain string: object-format property values
// normalize to single-element lists (§11), but this relation is maxCount 1 and
// every reader takes it as a string.
func TestConvert_AuthoredTargetObjectTypeWinsAndIsScalar(t *testing.T) {
	b := newBatch(nil, map[string]string{"wikiPage": "type-wiki-page"})
	_, snap := convertDoc(t, b, "wiki-guide.template.json", `{
	  "version": 2,
	  "kind": "template",
	  "id": "template-wiki-guide",
	  "type": "template",
	  "template_for": "wikiPage",
	  "properties": {"name": "Wiki Guide", "targetObjectType": "type-authored"}
	}`)

	assert.Equal(t, "type-authored",
		snap.Details.GetFields()[detailTargetObjectType].GetStringValue())
}

// Nothing to wire: a non-template keeps its details untouched even when its
// type key happens to be one the batch defines.
func TestConvert_NonTemplateGetsNoTargetObjectType(t *testing.T) {
	b := newBatch(nil, map[string]string{"wikiPage": "type-wiki-page"})
	_, snap := convertDoc(t, b, "page.json", `{
	  "version": 2,
	  "id": "page-1",
	  "type": "wikiPage",
	  "properties": {"name": "A page"}
	}`)

	assert.NotContains(t, snap.Details.GetFields(), detailTargetObjectType)
}

// convert surfaces the per-document warning tier: an authored thing that
// converts but silently does nothing must not be invisible in the tool that
// produces the archive. A groupBy on a table view is exactly that — it
// survives the round trip, so nothing downstream drops it, but no view
// type other than kanban/calendar ever groups by it (§6.2).
func TestConvert_SurfacesDocumentWarnings(t *testing.T) {
	b := newBatch(nil, nil)
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
	  "version": 2,
	  "id": "set-1",
	  "properties": {"name": "Tasks"},
	  "blocks": [{"type": "dataview", "views": [{"name": "All", "group_by": "status"}]}]
	}`), 0o644))

	var warnings []string
	_, _, snap, err := convertFile(dir, path, b, false, func(is anyblockjson.Issue) {
		warnings = append(warnings, is.String())
	})

	require.NoError(t, err)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "do not group")

	var view *model.BlockContentDataviewView
	for _, blk := range snap.Blocks {
		if dv := blk.GetDataview(); dv != nil {
			require.Len(t, dv.Views, 1)
			view = dv.Views[0]
		}
	}
	require.NotNil(t, view, "dataview block survived")
	assert.Equal(t, "status", view.GroupRelationKey,
		"the warned group_by is kept, not dropped — a table view just never honours it")
}

// A document with no id gets one derived from its file path, so a filename is
// an id-minting surface like any other. `_` opens the platform's address space
// (§1) and no bundle-local id may enter it, but refusing the file would make a
// perfectly legal filename illegal — so the prefix is escaped instead. Only the
// prefix: `completion_status` is a real key and `my_page` a fine id.
func TestFallbackSeed_DoesNotMintAPlatformId(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"_drafts.json", "drafts"},
		{"__notes.json", "notes"},
		{"objects/_set.json", "objects-_set"},
		{"my_page.json", "my_page"},
		{"a b.json", "a-b"},
	} {
		seed := fallbackSeed("/in", filepath.Join("/in", tc.path))
		assert.Equal(t, tc.want, seed, tc.path)
		assert.False(t, anyblockjson.IsPlatformId(genIdFactory(seed)()),
			"a generated id must not enter the platform namespace: %s", tc.path)
	}
}
