package anyblockbatch

// The lints read TRANSLATED type slots (`type`, `template_for`,
// `type_properties[].object_types`) and look them up in TypeIds, which is
// keyed by the UNTRANSLATED envelope `key` (SPEC §2). Every test below is a
// bundle where those two spellings differ, which is the only way the defect
// can show: a slot spelled as a term its own `type_internal_keys` legend binds
// elsewhere.
//
// Each test states which way it fails without the fix — fail-closed (a valid
// bundle rejected, which anyblockconvert turns into a hard error) or
// fail-open (a dangling reference the lint waves through, the case the lint
// exists to catch).

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// customTypeKey is a space-minted (bson) type key, the shape a real space
// gives a user-created type — the stored key a legend entry binds a spelling
// to.
const customTypeKey = "69bbfc78877a91b1d12d1a7c"

// customType is that type's own document, so the bundle can address it.
const customType = `{"version": 1, "kind": "object_type", "internal_key": "` + customTypeKey + `", "id": "type-custom"}`

// --- template_for ----------------------------------------------------------

// Fail-closed: `template_for` spells a term the document's legend binds to a
// stored key the bundle DOES define. Without resolution the lookup misses and
// the bundle is rejected — anyblockconvert exits non-zero on a bundle it
// converts correctly.
func TestCheckTemplateTargets_LegendBackedTargetPasses(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/custom.type.json": customType,
		"templates/article.json": `{"version": 1, "kind": "template", "type": "template", "template_for": "wiki_page",
		  "type_internal_keys": {"wiki_page": "` + customTypeKey + `"}}`,
	})
	typeIds, err := TypeIds(files)
	require.NoError(t, err)

	bad, err := CheckTemplateTargets(files, typeIds)
	require.NoError(t, err)
	assert.Empty(t, bad)
}

// Fail-OPEN, the important one: `template_for` spells `wikiPage`, which is
// ANOTHER type document's stored key, while this document's legend binds that
// spelling to a type the bundle does not define. The raw lookup hits the other
// type and reports nothing; the converter resolves to the bson key, finds no
// local type, and leaves targetObjectType unset — the template imports
// belonging to no type, silently.
func TestCheckTemplateTargets_TermCollidingWithAnotherTypesKeyIsReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/wiki-page.type.json": wikiPageType, // key "wikiPage", id "type-wiki-page"
		"templates/article.json": `{"version": 1, "kind": "template", "type": "template", "template_for": "wikiPage",
		  "type_internal_keys": {"wikiPage": "` + customTypeKey + `"}}`,
	})
	typeIds, err := TypeIds(files)
	require.NoError(t, err)
	require.Equal(t, "type-wiki-page", typeIds["wikiPage"],
		"the fixture only bites if the colliding spelling really is another type's stored key")

	bad, err := CheckTemplateTargets(files, typeIds)
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Equal(t, "wikiPage", bad[0].Target)
	assert.Contains(t, bad[0].Reason, "no such type")
	assert.Contains(t, bad[0].Reason, customTypeKey, "the reason must name the key the converter will look for")
}

// Fail-open on the GATE: the type term is an ordinary custom key, so nothing
// about the document LOOKS like a template — but `kind` says it is one, the
// codec builds a Template, and the lint has to check it. This used to be
// decided by resolving the type term through the legend, and a document whose
// gate the lint got wrong was skipped whole, its missing target unreported.
func TestCheckTemplateTargets_TheKindMakesADocumentATemplate(t *testing.T) {
	doc := `{"version": 1, "kind": "template", "type": "wiki_page",
		"type_internal_keys": {"wiki_page": "` + customTypeKey + `"}}`
	requireCodecSeesATemplate(t, doc, true)

	files := writeDocs(t, map[string]string{"templates/orphan.json": doc})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Empty(t, bad[0].Target)
	assert.Contains(t, bad[0].Reason, `no "template_for"`)
}

// The mirror, fail-closed: `type` IS spelled `template` — this is a page
// whose object type is the Template type, which is legal — and the kind says
// page, so the document is not a template. Validation agrees and refuses
// `/template_for` on it. The lint must not demand a target the format forbids.
func TestCheckTemplateTargets_TheTypeTermDoesNotMakeATemplate(t *testing.T) {
	for name, doc := range map[string]string{
		"the literal spelling":     `{"version": 1, "kind": "page", "type": "template"}`,
		"a legend onto the key":    `{"version": 1, "kind": "page", "type": "wiki_page", "type_internal_keys": {"wiki_page": "template"}}`,
		"no kind, ordinary object": `{"version": 1, "type": "wikiPage"}`,
	} {
		t.Run(name, func(t *testing.T) {
			requireCodecSeesATemplate(t, doc, false)

			files := writeDocs(t, map[string]string{"objects/not-a-template.json": doc})
			bad, err := CheckTemplateTargets(files, map[string]string{})
			require.NoError(t, err)
			assert.Empty(t, bad)
		})
	}
}

// requireCodecSeesATemplate pins the fixture to the codec's own verdict, so a
// test claiming "the converter builds a template here" cannot quietly stop
// being true.
func requireCodecSeesATemplate(t *testing.T, doc string, want bool) {
	t.Helper()
	require.NoError(t, anyblockjson.Validate([]byte(doc)),
		"the fixture must be a document the codec accepts, or the lint would never see it")
	sbType, _, err := anyblockjson.Unmarshal([]byte(doc), anyblockjson.Options{})
	require.NoError(t, err)
	assert.Equal(t, want, sbType == model.SmartBlockType_Template)
}

// --- object_types ----------------------------------------------------------

// Fail-closed: object_types names a term the legend backs with a stored key
// the bundle defines.
func TestCheckTargetTypes_LegendBackedTargetPasses(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/custom.type.json": customType,
		"types/person.type.json": `{"version": 1, "kind": "object_type", "internal_key": "person", "id": "type-person",
		  "type_internal_keys": {"wiki_page": "` + customTypeKey + `"},
		  "type_settings": {"property_definitions": [{"property": "assignee", "format": "objects", "object_types": ["wiki_page"]}]}}`,
	})
	typeIds, err := TypeIds(files)
	require.NoError(t, err)

	bad, err := CheckTargetTypes(files, typeIds)
	require.NoError(t, err)
	assert.Empty(t, bad)
}

// Fail-OPEN: object_types spells `wikiPage`, another type's stored key, while
// the legend binds it to a type the bundle does not define. The raw lookup
// hits the other type and reports nothing; the converter resolves to the bson
// key, misses typeIDs, and emits `_ot69bbfc…` — a bundled url for a type that
// is not bundled, which matches nothing on import.
func TestCheckTargetTypes_TermCollidingWithAnotherTypesKeyIsReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/wiki-page.type.json": wikiPageType, // key "wikiPage", id "type-wiki-page"
		"types/person.type.json": `{"version": 1, "kind": "object_type", "internal_key": "person", "id": "type-person",
		  "type_internal_keys": {"wikiPage": "` + customTypeKey + `"},
		  "type_settings": {"property_definitions": [{"property": "assignee", "format": "objects", "object_types": ["wikiPage"]}]}}`,
	})
	typeIds, err := TypeIds(files)
	require.NoError(t, err)
	require.Equal(t, "type-wiki-page", typeIds["wikiPage"],
		"the fixture only bites if the colliding spelling really is another type's stored key")

	bad, err := CheckTargetTypes(files, typeIds)
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Equal(t, "wikiPage", bad[0].Target)
	assert.Equal(t, "assignee", bad[0].Property)
	assert.Contains(t, bad[0].Reason, "no such type")
	assert.Contains(t, ReportTargets(bad), customTypeKey)
}

// A target the bundle neither defines nor the bundle table knows is still
// reported — resolution must not turn every miss into a pass.
func TestCheckTargetTypes_UnknownTargetIsStillReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/person.type.json": `{"version": 1, "kind": "object_type", "internal_key": "person", "id": "type-person",
		  "type_settings": {"property_definitions": [{"property": "assignee", "format": "objects", "object_types": ["wiki_page"]}]}}`,
	})
	bad, err := CheckTargetTypes(files, map[string]string{})
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Equal(t, "wiki_page", bad[0].Target)
}

// A bundled target still passes, whichever spelling is used — the stored key
// verbatim, or a term the bundled table (with its fold) resolves — because
// resolution folds both arms into one lookup.
func TestCheckTargetTypes_BundledTargetPassesInBothSpellings(t *testing.T) {
	for _, target := range []string{"object_type", "objectType", "page", "task"} {
		files := writeDocs(t, map[string]string{
			"types/person.type.json": `{"version": 1, "kind": "object_type", "internal_key": "person", "id": "type-person",
			  "type_settings": {"property_definitions": [{"property": "assignee", "format": "objects", "object_types": ["` + target + `"]}]}}`,
		})
		bad, err := CheckTargetTypes(files, map[string]string{})
		require.NoError(t, err)
		assert.Empty(t, bad, target)
	}
}

// --- anti-drift ------------------------------------------------------------

// The lint's chain is a composition — the document's legend, then the
// package's exported bundled vocabulary — and a composition can drift from
// the codec it models. This pins it against what anyblockjson.Unmarshal
// actually STORES for the same slots, so a future change to the chain that the
// lint does not follow fails here rather than in a production bundle.
func TestLintResolvesTypeTermsLikeTheCodec(t *testing.T) {
	cases := []struct {
		name        string
		legend      string
		typeTerm    string
		templateFor string
	}{
		{"verbatim custom key", ``, "template", "wikiPage"},
		{"bundled stored key spelled verbatim", ``, "template", "task"},
		{"bundled stored key that is nobody's spelling", ``, "template", "objectType"},
		{"legend-backed spelling", `"type_internal_keys": {"wiki_page": "` + customTypeKey + `"},`, "template", "wiki_page"},
		{"legend outranks the bundled table", `"type_internal_keys": {"task": "` + customTypeKey + `"},`, "template", "task"},
		{"legend on the type slot itself", `"type_internal_keys": {"wiki_page": "template"},`, "wiki_page", "task"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := `{"version": 1, "kind": "template", ` + c.legend +
				`"type": "` + c.typeTerm + `", "template_for": "` + c.templateFor + `"}`
			require.NoError(t, anyblockjson.Validate([]byte(doc)))

			_, snap, err := anyblockjson.Unmarshal([]byte(doc), anyblockjson.Options{})
			require.NoError(t, err)
			require.Len(t, snap.ObjectTypes, 2, "both type slots must reach the snapshot, or the case proves nothing")

			var probe struct {
				TypeKeys typeLegend `json:"type_internal_keys"`
			}
			require.NoError(t, json.Unmarshal([]byte(doc), &probe))

			assert.Equal(t, strings.TrimPrefix(snap.ObjectTypes[0], "ot-"),
				resolveTypeTerm(probe.TypeKeys, c.typeTerm), "type")
			assert.Equal(t, strings.TrimPrefix(snap.ObjectTypes[1], "ot-"),
				resolveTypeTerm(probe.TypeKeys, c.templateFor), "template_for")
		})
	}
}
