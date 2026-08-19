package anyblockbatch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

// writeDocs lays out a bundle in a temp dir and returns its files, sorted the
// way DiscoverJSONFiles returns them.
func writeDocs(t *testing.T, docs map[string]string) []string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range docs {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	files, err := DiscoverJSONFiles(dir)
	require.NoError(t, err)
	return files
}

const wikiPageType = `{"version": 1, "kind": "object_type", "key": "wikiPage", "id": "type-wiki-page"}`

func TestCheckTemplateTargets_ResolvableTargetPasses(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/wiki-page.type.json": wikiPageType,
		"templates/article.json":    `{"version": 1, "type": "template", "template_for": "wikiPage"}`,
	})
	typeIds, err := TypeIds(files)
	require.NoError(t, err)

	bad, err := CheckTemplateTargets(files, typeIds)
	require.NoError(t, err)
	assert.Empty(t, bad)
}

// a bundled type key is not enough: UpdateObjectIDsInRelations passes bundled
// ids through untouched, so "_otpage" would survive import literally and match
// no type in the space
func TestCheckTemplateTargets_BundledTargetIsReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"templates/note.json": `{"version": 1, "type": "template", "template_for": "page"}`,
	})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Equal(t, "page", bad[0].Target)
	assert.Contains(t, ReportTemplateTargets(bad), "bundled")
}

func TestCheckTemplateTargets_UndefinedTargetIsReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"templates/article.json": `{"version": 1, "type": "template", "template_for": "wikiPage"}`,
	})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Equal(t, "wikiPage", bad[0].Target)
}

// a type document with no id has nothing for the detail to point at
func TestCheckTemplateTargets_IdlessTargetIsReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/wiki-page.type.json": `{"version": 1, "kind": "object_type", "key": "wikiPage"}`,
		"templates/article.json":    `{"version": 1, "type": "template", "template_for": "wikiPage"}`,
	})
	typeIds, err := TypeIds(files)
	require.NoError(t, err)

	bad, err := CheckTemplateTargets(files, typeIds)
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Contains(t, bad[0].Reason, "no \"id\"")
}

// a template naming no target type at all belongs to nothing — the app refuses
// to create one (objectcreator.createTemplate), and import does not
func TestCheckTemplateTargets_MissingTemplateForIsReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"templates/orphan.json": `{"version": 1, "type": "template"}`,
	})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Empty(t, bad[0].Target)
}

// an authored targetObjectType is the value the converter keeps, so the
// document is wired whatever templateFor resolves to
func TestCheckTemplateTargets_AuthoredTargetObjectTypePasses(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"templates/article.json": `{"version": 1, "type": "template", "template_for": "page",
		  "properties": {"targetObjectType": "type-page"}}`,
	})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	assert.Empty(t, bad)
}

func TestCheckTemplateTargets_NonTemplatesAreIgnored(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/wiki-page.type.json": wikiPageType,
		"objects/page.json":         `{"version": 1, "type": "wikiPage"}`,
	})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	assert.Empty(t, bad)
}

// A widget target is the one reference in the format whose failure is silent:
// handleLinkBlock rewrites an unresolvable target to _missing_object and
// WidgetObject.Init then removes the link and its wrapper, so the widget is
// gone without an error anywhere.
func TestCheckIndexTargets_Widgets(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/wiki-page.type.json": wikiPageType,
		"objects/home.json":         `{"version": 1, "type": "wikiPage", "id": "page-home"}`,
	})

	index := func(targets ...string) *anyblockjson.Index {
		idx := &anyblockjson.Index{}
		for _, target := range targets {
			idx.Widgets = append(idx.Widgets, anyblockjson.Widget{Target: target})
		}
		return idx
	}

	t.Run("an object in the bundle passes", func(t *testing.T) {
		assert.Empty(t, CheckIndexTargets(index("page-home"), files))
	})

	// the four widget.IsPredefinedWidgetTargetId knows, which handleLinkBlock
	// leaves alone
	t.Run("the importable reserved listings pass", func(t *testing.T) {
		assert.Empty(t, CheckIndexTargets(index("favorite", "recent", "set", "collection"), files))
	})

	t.Run("an unknown id is reported", func(t *testing.T) {
		bad := CheckIndexTargets(index("page-gone"), files)
		require.Len(t, bad, 1)
		assert.Equal(t, "widgets[0]", bad[0].Property)
		assert.Contains(t, bad[0].Reason, "no object with that id")
	})

	// allObjects and recentOpen are real targets in a live space — the All
	// Objects widget comes from WidgetObject's migration 3 — but the importer
	// does not know them, so a bundle naming one loses that widget silently
	t.Run("a reserved listing the importer does not know is reported", func(t *testing.T) {
		for _, target := range []string{"allObjects", "recentOpen"} {
			bad := CheckIndexTargets(index(target), files)
			require.Len(t, bad, 1, target)
			assert.Equal(t, target, bad[0].Target)
			assert.Contains(t, bad[0].Reason, "the importer does not recognise", target)
		}
	})
}

// --- CheckTargetTypes: the arms must be ordered the way the converter orders
// them. batch.objectTypeIds asks typeIds FIRST and only falls through to the
// bundled url, so a bundle that defines a document with a bundled key takes
// the local arm — whatever the bundle table says.

// Fail-OPEN: `page` is bundled AND defined here without an id. Asking the
// bundle first short-circuits and reports clean, while objectTypeIds takes the
// local arm, finds the empty id, and appends an EMPTY STRING to
// relationFormatObjectTypes — a target that names nothing, is invisible in
// every UI, and re-exports as a shorter list than it went in as.
// (cmd/anyblockconvert TestBatch_IdlessLocalTypeYieldsAnEmptyTargetId pins the
// converter half of this.)
func TestCheckTargetTypes_BundledKeyDefinedLocallyWithoutIdIsReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/page.type.json": `{"version": 1, "kind": "object_type", "key": "page"}`,
		"types/person.type.json": `{"version": 1, "kind": "object_type", "key": "person", "id": "type-person",
		  "type_properties": [{"key": "assignee", "format": "objects", "object_types": ["page"]}]}`,
	})
	typeIds, err := TypeIds(files)
	require.NoError(t, err)
	require.Contains(t, typeIds, "page",
		"the fixture only bites if the id-less type really is registered — that is what makes the converter take the local arm")

	bad, err := CheckTargetTypes(files, typeIds)
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Equal(t, "page", bad[0].Target)
	assert.Contains(t, bad[0].Reason, `no "id"`)
	assert.Contains(t, bad[0].Reason, "prefers it over the bundled type",
		"the author needs to know why a bundled key is being complained about")
}

// The same shadowing with an id is fine: the converter uses the local id, and
// so it has something real to point at.
func TestCheckTargetTypes_BundledKeyDefinedLocallyWithIdPasses(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/page.type.json": `{"version": 1, "kind": "object_type", "key": "page", "id": "type-page"}`,
		"types/person.type.json": `{"version": 1, "kind": "object_type", "key": "person", "id": "type-person",
		  "type_properties": [{"key": "assignee", "format": "objects", "object_types": ["page"]}]}`,
	})
	typeIds, err := TypeIds(files)
	require.NoError(t, err)
	bad, err := CheckTargetTypes(files, typeIds)
	require.NoError(t, err)
	assert.Empty(t, bad, "%s", ReportTargets(bad))
}

// An id-less local type that shadows nothing bundled is reported the same way
// — the reorder must not lose the arm that already worked.
func TestCheckTargetTypes_IdlessLocalTypeIsReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/wiki-page.type.json": `{"version": 1, "kind": "object_type", "key": "wikiPage"}`,
		"types/person.type.json": `{"version": 1, "kind": "object_type", "key": "person", "id": "type-person",
		  "type_properties": [{"key": "assignee", "format": "objects", "object_types": ["wikiPage"]}]}`,
	})
	typeIds, err := TypeIds(files)
	require.NoError(t, err)
	bad, err := CheckTargetTypes(files, typeIds)
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Contains(t, bad[0].Reason, `no "id"`)
	assert.NotContains(t, bad[0].Reason, "prefers it over the bundled type")
}
