package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// convertDoc writes a document to a temp dir and converts it, the way the
// directory walk does.
func convertDoc(t *testing.T, b *batch, name, body string) (model.SmartBlockType, *model.SmartBlockSnapshotBase) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	_, sbType, snap, err := convertFile(dir, path, b, false)
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
	  "version": 1,
	  "id": "template-wiki-article",
	  "type": "template",
	  "templateFor": "wikiPage",
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
// normalize to single-element lists, but this relation is maxCount 1 and
// every reader takes it as a string.
func TestConvert_AuthoredTargetObjectTypeWinsAndIsScalar(t *testing.T) {
	b := newBatch(nil, map[string]string{"wikiPage": "type-wiki-page"})
	_, snap := convertDoc(t, b, "wiki-guide.template.json", `{
	  "version": 1,
	  "id": "template-wiki-guide",
	  "type": "template",
	  "templateFor": "wikiPage",
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
	  "version": 1,
	  "id": "page-1",
	  "type": "wikiPage",
	  "properties": {"name": "A page"}
	}`)

	assert.NotContains(t, snap.Details.GetFields(), detailTargetObjectType)
}
