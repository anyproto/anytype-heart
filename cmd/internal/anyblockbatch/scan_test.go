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

const wikiPageType = `{"version": 2, "kind": "object_type", "internal_key": "wikiPage", "id": "type-wiki-page"}`

func TestCheckTemplateTargets_ResolvableTargetPasses(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/wiki-page.type.json": wikiPageType,
		"templates/article.json":    `{"version": 2, "kind": "template", "type": "template", "template_for": "wikiPage"}`,
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
		"templates/note.json": `{"version": 2, "kind": "template", "type": "template", "template_for": "page"}`,
	})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Equal(t, "page", bad[0].Target)
	assert.Contains(t, ReportTemplateTargets(bad), "bundled")
}

func TestCheckTemplateTargets_UndefinedTargetIsReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"templates/article.json": `{"version": 2, "kind": "template", "type": "template", "template_for": "wikiPage"}`,
	})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Equal(t, "wikiPage", bad[0].Target)
}

// a type document with no id has nothing for the detail to point at
func TestCheckTemplateTargets_IdlessTargetIsReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/wiki-page.type.json": `{"version": 2, "kind": "object_type", "internal_key": "wikiPage"}`,
		"templates/article.json":    `{"version": 2, "kind": "template", "type": "template", "template_for": "wikiPage"}`,
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
		"templates/orphan.json": `{"version": 2, "kind": "template", "type": "template"}`,
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
		"templates/article.json": `{"version": 2, "kind": "template", "type": "template", "template_for": "page",
		  "properties": {"targetObjectType": "type-page"}}`,
	})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	assert.Empty(t, bad)
}

func TestCheckTemplateTargets_NonTemplatesAreIgnored(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/wiki-page.type.json": wikiPageType,
		"objects/page.json":         `{"version": 2, "type": "wikiPage"}`,
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
		"objects/home.json":         `{"version": 2, "type": "wikiPage", "id": "page-home"}`,
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

	// the whole inventory widget.IsPredefinedWidgetTargetId knows, which
	// handleLinkBlock leaves alone
	t.Run("the importable reserved listings pass", func(t *testing.T) {
		assert.Empty(t, CheckIndexTargets(index(anyblockjson.ReservedWidgetTargets()...), files))
	})

	// The listings used to be bare words, and the pb importer consults the
	// bundle's own id map BEFORE widget.IsPredefinedWidgetTargetId
	// (common.UpdateLinksToObjects), so an object with id `set` captured every
	// widget that meant the Sets listing — with no finding here and no error
	// at import. The bare word is now an ordinary id and the listing is
	// `_set`, which CheckBundleIds forbids any object from claiming.
	t.Run("a bundle object can no longer shadow a listing", func(t *testing.T) {
		shadow := writeDocs(t, map[string]string{
			"types/wiki-page.type.json": wikiPageType,
			"objects/sets.json":         `{"version": 2, "type": "wikiPage", "id": "set"}`,
		})
		assert.Empty(t, CheckIndexTargets(index("_set"), shadow),
			"the listing resolves as a listing, whatever ids the bundle ships")
		for _, target := range anyblockjson.ReservedWidgetTargets() {
			assert.True(t, anyblockjson.IsPlatformId(target), target)
		}
	})

	t.Run("an unknown id is reported", func(t *testing.T) {
		bad := CheckIndexTargets(index("page-gone"), files)
		require.Len(t, bad, 1)
		assert.Equal(t, "widgets[0]", bad[0].Property)
		assert.Contains(t, bad[0].Reason, "no object with that id")
	})

	// This test used to pin the OPPOSITE: _all_objects and _recent_open were
	// real targets in a live space that the importer did not know, so a
	// bundle naming one was refused here rather than losing the widget
	// silently on install. widget.IsPredefinedWidgetTargetId knows the whole
	// inventory since GO-7383, so the same scenario now pins that every
	// reserved listing is importable — the refusal branch stays in
	// CheckIndexTargets as the guard for the day the two inventories come
	// apart again.
	t.Run("every reserved listing survives import", func(t *testing.T) {
		for _, target := range anyblockjson.ReservedWidgetTargets() {
			assert.True(t, anyblockjson.IsImportableWidgetTarget(target), target)
			assert.Empty(t, CheckIndexTargets(index(target), files), target)
		}
	})

	// the auto-widget ledger's entries are target-shaped references (§2c):
	// a reserved listing or a bundle object, and nothing else
	t.Run("auto_widget_targets entries are checked like targets", func(t *testing.T) {
		idx := index("page-home")
		idx.AutoWidgetTargets = []string{"_bin", "page-home", "page-gone"}
		bad := CheckIndexTargets(idx, files)
		require.Len(t, bad, 1)
		assert.Equal(t, "auto_widget_targets[2]", bad[0].Property)
		assert.Equal(t, "page-gone", bad[0].Target)
	})
}

// CheckBundleIds is what makes the reserved `_` namespace hold. Without it the
// rename only moves the shadowing target: an object with id `_set` captures
// the Sets widget exactly the way one with id `set` used to, because the
// importer still resolves through the bundle's ids first.
func TestCheckBundleIds(t *testing.T) {
	t.Run("an ordinary bundle passes", func(t *testing.T) {
		files := writeDocs(t, map[string]string{
			"types/wiki-page.type.json": wikiPageType,
			"objects/home.json":         `{"version": 2, "type": "wikiPage", "id": "page-home"}`,
			"objects/under.json":        `{"version": 2, "type": "wikiPage", "id": "my_page_2"}`,
		})
		bad, err := CheckBundleIds(files)
		require.NoError(t, err)
		assert.Empty(t, bad, "`_` is only reserved as a PREFIX")
	})

	// every reserved name, so adding a listing without adding a ban is not a
	// thing that can happen
	t.Run("no reserved listing or screen can be a bundle id", func(t *testing.T) {
		reserved := append(anyblockjson.ReservedWidgetTargets(),
			anyblockjson.HomepageWidgets, anyblockjson.HomepageGraph)
		// …and neither can what those are called on the WIRE. This is the half
		// the prefix does not cover and the half that decides whether the
		// rename accomplished anything: anyblockconvert translates a widget
		// target `_set` to `set` before writing the link, and
		// common.handleLinkBlock then resolves `set` through the bundle's own
		// id map BEFORE asking widget.IsPredefinedWidgetTargetId. Ban only the
		// prefix and an object with id `set` captures the Sets widget exactly
		// as it did before the rename — the collision moved, nothing else.
		for _, r := range reserved {
			if wire := anyblockjson.WireWidgetTarget(r); wire != r {
				reserved = append(reserved, wire)
			}
			if wire := anyblockjson.WireHomepage(r); wire != r {
				reserved = append(reserved, wire)
			}
		}
		require.Contains(t, reserved, "set", "the wire spellings must be in this list, or it proves nothing")
		require.Contains(t, reserved, "graph")
		for _, id := range reserved {
			files := writeDocs(t, map[string]string{
				"types/wiki-page.type.json": wikiPageType,
				"objects/shadow.json":       `{"version": 2, "type": "wikiPage", "id": "` + id + `"}`,
			})
			bad, err := CheckBundleIds(files)
			require.NoError(t, err, id)
			require.Len(t, bad, 1, id)
			assert.Equal(t, id, bad[0].Target)
			assert.Equal(t, "id", bad[0].Property)
		}
	})

	// the prefix is the rule, not the six words: a bundled object's own
	// address is just as unmintable
	t.Run("a bundled platform address cannot be a bundle id", func(t *testing.T) {
		files := writeDocs(t, map[string]string{
			"types/wiki-page.type.json": wikiPageType,
			"objects/page.json":         `{"version": 2, "type": "wikiPage", "id": "_otpage"}`,
		})
		bad, err := CheckBundleIds(files)
		require.NoError(t, err)
		require.Len(t, bad, 1)
		assert.Contains(t, bad[0].Reason, "platform")
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
		"types/page.type.json": `{"version": 2, "kind": "object_type", "internal_key": "page"}`,
		"types/person.type.json": `{"version": 2, "kind": "object_type", "internal_key": "person", "id": "type-person",
		  "type_settings": {"property_definitions": [{"property": "assignee", "format": "objects", "object_types": ["page"]}]}}`,
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
		"types/page.type.json": `{"version": 2, "kind": "object_type", "internal_key": "page", "id": "type-page"}`,
		"types/person.type.json": `{"version": 2, "kind": "object_type", "internal_key": "person", "id": "type-person",
		  "type_settings": {"property_definitions": [{"property": "assignee", "format": "objects", "object_types": ["page"]}]}}`,
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
		"types/wiki-page.type.json": `{"version": 2, "kind": "object_type", "internal_key": "wikiPage"}`,
		"types/person.type.json": `{"version": 2, "kind": "object_type", "internal_key": "person", "id": "type-person",
		  "type_settings": {"property_definitions": [{"property": "assignee", "format": "objects", "object_types": ["wikiPage"]}]}}`,
	})
	typeIds, err := TypeIds(files)
	require.NoError(t, err)
	bad, err := CheckTargetTypes(files, typeIds)
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Contains(t, bad[0].Reason, `no "id"`)
	assert.NotContains(t, bad[0].Reason, "prefers it over the bundled type")
}

// The manifest `files` map binds a file document to its bytes (§2c, v0.47),
// and every failure it can have is silent at import — the file object
// arrives, its content does not. Refusals cover what the map STATES: a key
// naming no document, a path escaping the bundle, a promised blob that is
// not there. A bundle whose map omits a file document passes — whether an
// absent blob is intended is the thin-bundle marker's future question
// (SPEC §15 #20), and refusing it today would refuse every pre-v0.47
// bundle.
//
// How this can fail: stat the blob against the WALK root instead of the
// index's own directory (a nested bundle's bindings all read missing);
// stop cleaning the path before the escape check (`a/../../x` reads as
// contained); or require an entry per file document (every legacy bundle
// turns invalid overnight).
func TestCheckManifestFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "files"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "files", "file-1.anyblock.json"),
		[]byte(`{"version": 2, "kind": "file_object", "id": "file-1"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "files", "file-1.png"), []byte("bytes"), 0o644))
	files, err := DiscoverJSONFiles(dir)
	require.NoError(t, err)

	manifest := func(entries map[string]string) *anyblockjson.Index {
		return &anyblockjson.Index{Manifest: &anyblockjson.Manifest{Files: entries}}
	}

	t.Run("a binding whose document and blob both exist passes", func(t *testing.T) {
		assert.Empty(t, CheckManifestFiles(manifest(map[string]string{"file-1": "files/file-1.png"}), dir, files))
	})
	t.Run("no map, nothing to check", func(t *testing.T) {
		assert.Empty(t, CheckManifestFiles(&anyblockjson.Index{}, dir, files))
	})
	t.Run("a key naming no document is a binding for nothing", func(t *testing.T) {
		got := CheckManifestFiles(manifest(map[string]string{"ghost": "files/file-1.png"}), dir, files)
		require.Len(t, got, 1)
		assert.Contains(t, got[0].Reason, "no document")
	})
	t.Run("a path that climbs out of the bundle is refused", func(t *testing.T) {
		got := CheckManifestFiles(manifest(map[string]string{"file-1": "../outside.png"}), dir, files)
		require.Len(t, got, 1)
		assert.Contains(t, got[0].Reason, "outside the bundle")
		got = CheckManifestFiles(manifest(map[string]string{"file-1": "files/../../outside.png"}), dir, files)
		require.Len(t, got, 1)
		assert.Contains(t, got[0].Reason, "outside the bundle")
	})
	t.Run("a promised blob that is not there is refused", func(t *testing.T) {
		got := CheckManifestFiles(manifest(map[string]string{"file-1": "files/file-1.pdf"}), dir, files)
		require.Len(t, got, 1)
		assert.Contains(t, got[0].Reason, "missing")
	})
}

// A FAT bundle's manifest-bound .json BLOB is content, not a document
// (§2c, v0.47): 12 corpus file objects carry file_ext == "json", and only
// the `files` map can tell their bytes from an authored bare-.json
// document. Discovery excludes exactly what the map binds — nothing more:
// an unbound .json stays a document, because authored bundles legitimately
// name documents with a bare extension.
//
// How this can fail: skip by extension convention instead of the map
// (every authored bare-.json document vanishes from every batch); or fail
// discovery on a broken index (a bundle with one bad index.json loses its
// whole document set instead of its blob exclusion).
func TestDiscoverJSONFiles_SkipsManifestBoundBlobs(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		path := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	write("objects/home.json", `{"version": 2, "id": "home"}`)
	write("files/data.anyblock.json", `{"version": 2, "kind": "file_object", "id": "data"}`)
	write("files/data.json", `{"whatever": "a JSON blob that is file CONTENT"}`)
	write("index.json", `{"version": 2, "manifest": {"files": {"data": "files/data.json"}}}`)

	files, err := DiscoverJSONFiles(dir)
	require.NoError(t, err)
	rels := make([]string, 0, len(files))
	for _, f := range files {
		rel, _ := filepath.Rel(dir, f)
		rels = append(rels, filepath.ToSlash(rel))
	}
	assert.ElementsMatch(t, []string{"objects/home.json", "files/data.anyblock.json"}, rels,
		"the bound blob is skipped; the documents — double-extended or bare — stay")
}

// A file document a PRESENT manifest map does not bind is warned about —
// bytes that did not travel, the signature of a partially failed export.
// A bundle with NO map stays silent: metadata-only exports (files off,
// pre-v0.47) are a legitimate mode, and warning on every one of them would
// train readers to ignore the warning.
func TestUnboundFileDocuments(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"files/bound.anyblock.json":   `{"version": 2, "kind": "file_object", "id": "bound"}`,
		"files/unbound.anyblock.json": `{"version": 2, "kind": "file_object", "id": "unbound"}`,
		"objects/page.json":           `{"version": 2, "id": "page"}`,
	})

	withMap := &anyblockjson.Index{Manifest: &anyblockjson.Manifest{
		Files: map[string]string{"bound": "files/bound.png"}}}
	assert.Equal(t, []string{"unbound"}, UnboundFileDocuments(withMap, files),
		"only the file document the map skips; pages are not file documents")

	assert.Empty(t, UnboundFileDocuments(&anyblockjson.Index{}, files),
		"no map, no warning — metadata-only bundles are a mode, not a defect")
}
