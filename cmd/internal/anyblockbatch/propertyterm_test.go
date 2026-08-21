package anyblockbatch

// The scans read TRANSLATED property slots — every key of `properties`, and
// every `type_properties[].key` — and hand the result to the converter, which
// asks for it by the STORED key anyblockjson resolved that term to (§3). Every
// test below is a bundle where those two spellings differ, which is the only
// way the defect can show.
//
// Each test states which way it fails without the fix — fail-open (a silent
// miss the scan waves through, the case the scan exists to catch) or
// fail-closed (a valid bundle rejected, which anyblockconvert turns into a
// hard error unless -lenient).

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// customPropertyKey is a space-minted (bson) property key, the shape a real
// space gives a user-created relation — the stored key a slug legend binds to.
// `priority` is deliberately chosen as its slug: it is ALSO a bundled key, so
// a scan that reads the spelling raw does not merely miss, it confidently
// resolves to the wrong relation.
const customPropertyKey = "6a32d4856761631534b22f85"

// --- ScanFormats -----------------------------------------------------------

// Fail-OPEN, the one that costs real data: the table is keyed by the spelling
// while the converter reads it by the stored key, so the format is never
// found. The value passes through as raw JSON and no Relation object is minted
// for the property at all — and nothing anywhere says so.
func TestScanFormats_LegendBackedKeyIsStoredResolved(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/task.type.json": `{"version": 1, "kind": "object_type", "key": "task", "id": "type-task",
		  "property_keys": {"priority": "` + customPropertyKey + `"},
		  "type_properties": [{"key": "priority", "name": "Priority", "format": "select",
		    "options": ["High", "Low"]}]}`,
	})
	formats, err := ScanFormats(files)
	require.NoError(t, err)

	require.Contains(t, formats, customPropertyKey,
		"the converter looks the format up by the stored key the legend names")
	assert.NotContains(t, formats, "priority",
		"and never by the spelling, which here is a DIFFERENT (bundled) relation")
	assert.Equal(t, model.RelationFormat_status, formats[customPropertyKey].Format)
	assert.Len(t, formats[customPropertyKey].Options, 2,
		"the declared vocabulary travels with the stored key, or newBatch pre-mints it on a relation nothing uses")
}

// The fallback display name stays the SPELLING: a legend exists because the
// stored key is a bson nobody wants to read, and this name is what
// mintRelation writes when the entry declares none.
func TestScanFormats_FallbackNameIsTheSpelling(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/task.type.json": `{"version": 1, "kind": "object_type", "key": "task", "id": "type-task",
		  "property_keys": {"priority": "` + customPropertyKey + `"},
		  "type_properties": [{"key": "priority", "format": "text"}]}`,
	})
	formats, err := ScanFormats(files)
	require.NoError(t, err)
	assert.Equal(t, "priority", formats[customPropertyKey].Name)
}

// A key nobody translates is its own address (chain step 4), so resolution
// must leave the ordinary case exactly as it was.
func TestScanFormats_UntranslatedKeyIsUnchanged(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/task.type.json": `{"version": 1, "kind": "object_type", "key": "task", "id": "type-task",
		  "type_properties": [{"key": "wikiStage", "format": "select", "options": ["A"]}]}`,
	})
	formats, err := ScanFormats(files)
	require.NoError(t, err)
	assert.Contains(t, formats, "wikiStage")
}

// A bundled property declared by its api slug — the canonical spelling (§3) —
// has to land on the bundled STORED key, or the batch mints a second relation
// beside the bundled one for the same property.
func TestScanFormats_BundledSlugResolvesToTheBundledKey(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/task.type.json": `{"version": 1, "kind": "object_type", "key": "task", "id": "type-task",
		  "type_properties": [{"key": "due_date", "format": "date"}]}`,
	})
	formats, err := ScanFormats(files)
	require.NoError(t, err)
	assert.Contains(t, formats, "dueDate")
	assert.NotContains(t, formats, "due_date")
}

// The strongest statement of the contract, and the one that cannot go vacuous:
// it asks the CODEC which keys it wants formats for, through the real
// Options.ResolveFormat seam, and requires the table to answer every one.
// Without resolution the codec asks for the bson and the table holds
// "priority", so the resolver is called and misses.
func TestScanFormats_AnswersEveryKeyTheCodecAsksFor(t *testing.T) {
	const object = `{"version": 1, "type": "task", "id": "obj-1",
	  "property_keys": {"priority": "` + customPropertyKey + `"},
	  "properties": {"priority": "High", "wikiStage": "Draft"}}`
	files := writeDocs(t, map[string]string{
		"types/task.type.json": `{"version": 1, "kind": "object_type", "key": "task", "id": "type-task",
		  "property_keys": {"priority": "` + customPropertyKey + `"},
		  "type_properties": [{"key": "priority", "format": "select", "options": ["High"]},
		    {"key": "wikiStage", "format": "select", "options": ["Draft"]}]}`,
		"objects/one.json": object,
	})
	formats, err := ScanFormats(files)
	require.NoError(t, err)

	var asked []string
	var missed []string
	_, _, err = anyblockjson.Unmarshal([]byte(object), anyblockjson.Options{
		ResolveFormat: func(key domain.RelationKey) (model.RelationFormat, bool) {
			asked = append(asked, string(key))
			fi, ok := formats[string(key)]
			if !ok {
				missed = append(missed, string(key))
				return 0, false
			}
			return fi.Format, true
		},
	})
	require.NoError(t, err)

	sort.Strings(asked)
	require.Equal(t, []string{customPropertyKey, "wikiStage"}, asked,
		"the codec asks by the resolved stored key — if this list changes, the table's key must follow")
	assert.Empty(t, missed, "every key the codec asks for must be in the table ScanFormats built")
}

// --- CheckPropertyFormats --------------------------------------------------

// Fail-OPEN: the document's own legend says `priority` here is a space-minted
// relation, not the bundled one. Reading the spelling raw hit
// bundle.GetRelationFormat("priority"), which answers `number` for a relation
// this value has nothing to do with, and the check reported clean — while the
// converter resolved to the bson, found no format, and passed the value
// through raw with no Relation minted.
func TestCheckPropertyFormats_LegendBackedMissIsReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"objects/one.json": `{"version": 1, "type": "page", "id": "obj-1",
		  "property_keys": {"priority": "` + customPropertyKey + `"},
		  "properties": {"priority": "High"}}`,
	})
	_, bundled := bundle.GetRelationFormat(domain.RelationKey("priority"))
	require.NoError(t, bundled,
		"the fixture only bites while `priority` really is a bundled key the raw check would accept")

	formats, err := ScanFormats(files)
	require.NoError(t, err)
	undeclared, err := CheckPropertyFormats(files, formats)
	require.NoError(t, err)

	require.Len(t, undeclared, 1)
	assert.Equal(t, "priority", undeclared[0].Key, "the spelling, so the author can find it")
	assert.Equal(t, customPropertyKey, undeclared[0].Resolved)
	assert.Contains(t, Report(undeclared), customPropertyKey,
		"the report must name the key a type_properties entry has to end up on")
}

// Fail-CLOSED: `properties` spells bundled keys as their api slugs (§3), and
// `bundle` is keyed by stored keys — it has never heard of `due_date`. So the
// canonical spelling was reported as having no declared format, which
// anyblockconvert turns into a hard error unless -lenient.
func TestCheckPropertyFormats_BundledSlugIsDeclared(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"objects/one.json": `{"version": 1, "type": "page", "id": "obj-1",
		  "properties": {"due_date": "2026-01-01T00:00:00Z", "icon_emoji": "x", "description": "hi"}}`,
	})
	formats, err := ScanFormats(files)
	require.NoError(t, err)
	undeclared, err := CheckPropertyFormats(files, formats)
	require.NoError(t, err)
	assert.Empty(t, undeclared, "%s", Report(undeclared))
}

// Resolution must not turn every miss into a pass: a key that is neither
// bundled, nor legend-backed, nor declared is still the thing this check is
// for.
func TestCheckPropertyFormats_UnknownKeyIsStillReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"objects/one.json": `{"version": 1, "type": "page", "id": "obj-1",
		  "properties": {"wikiStage": "Draft"}}`,
	})
	undeclared, err := CheckPropertyFormats(files, map[string]FormatInfo{})
	require.NoError(t, err)
	require.Len(t, undeclared, 1)
	assert.Equal(t, "wikiStage", undeclared[0].Key)
	assert.Equal(t, "wikiStage", undeclared[0].Resolved)
	assert.NotContains(t, Report(undeclared), "stored key",
		"a term that is its own address needs no annotation")
}

// The whole point of resolving on both sides: a legend-backed value IS
// declared once some type declares the same stored key, however either spells
// it. Here the type spells the bson outright and the object spells the slug.
func TestCheckPropertyFormats_DeclarationAndUseMayDisagreeOnSpelling(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/task.type.json": `{"version": 1, "kind": "object_type", "key": "task", "id": "type-task",
		  "type_properties": [{"key": "` + customPropertyKey + `", "format": "select", "options": ["High"]}]}`,
		"objects/one.json": `{"version": 1, "type": "task", "id": "obj-1",
		  "property_keys": {"priority": "` + customPropertyKey + `"},
		  "properties": {"priority": "High"}}`,
	})
	formats, err := ScanFormats(files)
	require.NoError(t, err)
	undeclared, err := CheckPropertyFormats(files, formats)
	require.NoError(t, err)
	assert.Empty(t, undeclared, "%s", Report(undeclared))
}

// `id` and `type` are lifted into the envelope, and the codec skips them on
// the SPELLING before any resolution — so this must too, whatever a legend
// says about those terms.
func TestCheckPropertyFormats_EnvelopeLiftedKeysAreSkipped(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"objects/one.json": `{"version": 1, "id": "obj-1", "properties": {"id": "x", "type": "y"}}`,
	})
	undeclared, err := CheckPropertyFormats(files, map[string]FormatInfo{})
	require.NoError(t, err)
	assert.Empty(t, undeclared)
}

// --- CheckSharedSelects ----------------------------------------------------

// Fail-OPEN: two types declare ONE stored key, one through its legend and one
// verbatim. Their option pools merge in the space — each type's board grows
// the other's empty columns — and grouping by spelling reported nothing. This
// is the collision an author cannot see by reading the two files side by side,
// which is exactly why the check exists.
func TestCheckSharedSelects_MergesAcrossSpellings(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/a.type.json": `{"version": 1, "kind": "object_type", "key": "typeA", "id": "type-a",
		  "property_keys": {"stage": "` + customPropertyKey + `"},
		  "type_properties": [{"key": "stage", "format": "select", "options": ["One"]}]}`,
		"types/b.type.json": `{"version": 1, "kind": "object_type", "key": "typeB", "id": "type-b",
		  "type_properties": [{"key": "` + customPropertyKey + `", "format": "select", "options": ["Two"]}]}`,
	})
	shared, err := CheckSharedSelects(files)
	require.NoError(t, err)

	require.Len(t, shared, 1)
	assert.Equal(t, customPropertyKey, shared[0].Key, "the stored key is what the space shares")
	assert.ElementsMatch(t, []string{"typeA", "typeB"}, shared[0].Types)
}

// The mirror, and the guard against over-merging: one SPELLING, two stored
// keys, because each document's legend binds it elsewhere. Those are two
// properties with two option pools, and reporting them as shared would be a
// false alarm about a merge that does not happen.
func TestCheckSharedSelects_OneSpellingTwoKeysIsNotShared(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/a.type.json": `{"version": 1, "kind": "object_type", "key": "typeA", "id": "type-a",
		  "property_keys": {"stage": "` + customPropertyKey + `"},
		  "type_properties": [{"key": "stage", "format": "select", "options": ["One"]}]}`,
		"types/b.type.json": `{"version": 1, "kind": "object_type", "key": "typeB", "id": "type-b",
		  "property_keys": {"stage": "69bbfc78877a91b1d12d1a7c"},
		  "type_properties": [{"key": "stage", "format": "select", "options": ["Two"]}]}`,
	})
	shared, err := CheckSharedSelects(files)
	require.NoError(t, err)
	assert.Empty(t, shared, "%s", ReportSharedSelects(shared))
}

// The ordinary case still works: one spelling, no legend anywhere.
func TestCheckSharedSelects_PlainSharedKeyIsStillReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/a.type.json": `{"version": 1, "kind": "object_type", "key": "typeA", "id": "type-a",
		  "type_properties": [{"key": "wikiStage", "format": "select", "options": ["One"]}]}`,
		"types/b.type.json": `{"version": 1, "kind": "object_type", "key": "typeB", "id": "type-b",
		  "type_properties": [{"key": "wikiStage", "format": "select", "options": ["Two"]}]}`,
	})
	shared, err := CheckSharedSelects(files)
	require.NoError(t, err)
	require.Len(t, shared, 1)
	assert.Equal(t, "wikiStage", shared[0].Key)
}

// --- the authored targetObjectType probe -----------------------------------

// Fail-CLOSED: `target_object_type` is the canonical api-slug spelling of the
// detail (§3) and reaches the snapshot, so patchTemplateTarget keeps it — but
// the check probed the map for the STORED key alone, missed it, and rejected a
// bundle the converter wires perfectly.
func TestCheckTemplateTargets_AuthoredTargetInSlugSpellingPasses(t *testing.T) {
	const doc = `{"version": 1, "kind": "template", "type": "template", "template_for": "page",
	  "properties": {"target_object_type": "type-page"}}`
	requireCodecStoresTargetObjectType(t, doc, true)

	files := writeDocs(t, map[string]string{"templates/article.json": doc})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	assert.Empty(t, bad, "%s", ReportTemplateTargets(bad))
}

// Fail-OPEN, the mirror: the legend rebinds the `targetObjectType` spelling
// onto another relation, so the detail is NOT written and the template needs
// template_for wired after all. Probing the raw spelling took this as authored
// and skipped the document whole — the template then imports belonging to no
// type, unreported.
func TestCheckTemplateTargets_LegendMovesTheAuthoredTargetAway(t *testing.T) {
	const doc = `{"version": 1, "kind": "template", "type": "template", "template_for": "page",
	  "property_keys": {"targetObjectType": "` + customPropertyKey + `"},
	  "properties": {"targetObjectType": "type-page"}}`
	requireCodecStoresTargetObjectType(t, doc, false)

	files := writeDocs(t, map[string]string{"templates/article.json": doc})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Equal(t, "page", bad[0].Target)
	assert.Contains(t, bad[0].Reason, "bundled")
}

// requireCodecStoresTargetObjectType pins the fixture to the codec's own
// verdict, so a test claiming "the converter has an authored target here"
// cannot quietly stop being true.
func requireCodecStoresTargetObjectType(t *testing.T, doc string, want bool) {
	t.Helper()
	require.NoError(t, anyblockjson.Validate([]byte(doc)),
		"the fixture must be a document the codec accepts, or the check would never see it")
	_, snap, err := anyblockjson.Unmarshal([]byte(doc), anyblockjson.Options{})
	require.NoError(t, err)
	_, stored := snap.Details.GetFields()[string(bundle.RelationKeyTargetObjectType)]
	require.Equal(t, want, stored)
}

// --- anti-drift ------------------------------------------------------------

// The scan's chain is a composition — the document's legend, then the
// package's exported bundled vocabulary — and a composition can drift from the
// codec it models. This pins it against the key anyblockjson.Unmarshal
// actually STORES the value under, so a future change to the chain that the
// scans do not follow fails here rather than in a production bundle.
func TestLintResolvesPropertyTermsLikeTheCodec(t *testing.T) {
	cases := []struct {
		name   string
		legend string
		term   string
		value  string
	}{
		{"verbatim custom key", ``, "wikiStage", `"Draft"`},
		{"bundled slug", ``, "due_date", `"2026-01-01T00:00:00Z"`},
		{"bundled stored key that is nobody's slug", ``, "dueDate", `"2026-01-01T00:00:00Z"`},
		{"legend-backed slug", `"property_keys": {"priority": "` + customPropertyKey + `"},`, "priority", `"High"`},
		{"legend outranks the bundled table", `"property_keys": {"due_date": "` + customPropertyKey + `"},`, "due_date", `"High"`},
		{"identity entry for a shadow stored key", `"property_keys": {"due_date": "due_date"},`, "due_date", `"High"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := `{"version": 1, "id": "obj-1", ` + c.legend +
				`"properties": {"` + c.term + `": ` + c.value + `}}`
			require.NoError(t, anyblockjson.Validate([]byte(doc)))

			_, snap, err := anyblockjson.Unmarshal([]byte(doc), anyblockjson.Options{})
			require.NoError(t, err)

			var probe struct {
				PropertyKeys propertyLegend `json:"property_keys"`
			}
			require.NoError(t, json.Unmarshal([]byte(doc), &probe))

			stored := make([]string, 0, 1)
			for k := range snap.Details.GetFields() {
				if k != "id" {
					stored = append(stored, k)
				}
			}
			require.Len(t, stored, 1, "the value must reach the snapshot, or the case proves nothing")
			assert.Equal(t, stored[0], resolvePropertyTerm(probe.PropertyKeys, c.term))
		})
	}
}
