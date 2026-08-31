package anyblockjson

// optionrefs_test.go — the `option_ids` legend (optionrefs.go, §3, §9a).
//
// Every test here is written against a resolver that behaves the way the real
// one does: `spaceOptions` scans a per-relation list and answers with the
// FIRST match by name, which is exactly `storeresolver.OptionId`. That is not
// incidental — the two defects this legend closes are both consequences of
// that scan, so a resolver that answered by a map would make the tests pass
// for the wrong reason.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// spaceOption is one relation option as a space holds it: an id and a name,
// with nothing enforcing that names are unique.
type spaceOption struct {
	id, name string
}

// spaceOptions is a stand-in for a space's option pool, scanned exactly as
// storeresolver.Resolvers does — first match wins, both directions, and an id
// that is not in the pool has no name (which is how import asks whether an id
// is live here).
type spaceOptions map[domain.RelationKey][]spaceOption

func (s spaceOptions) OptionName(key domain.RelationKey, id string) (string, bool) {
	for _, o := range s[key] {
		if o.id == id {
			return o.name, true
		}
	}
	return "", false
}

func (s spaceOptions) OptionId(key domain.RelationKey, name string) (string, bool) {
	for _, o := range s[key] {
		if o.name == name {
			return o.id, true
		}
	}
	return "", false
}

// selectFormats answers `tag` for every key it is given, so a test can use a
// custom property key and still exercise the select path (§3 format
// resolution). Bundled keys never reach it — the bundle answers first.
func selectFormats(domain.RelationKey) (model.RelationFormat, bool) {
	return model.RelationFormat_tag, true
}

// optionSnapshot is a one-object snapshot carrying select values by id.
func optionSnapshot(props map[string]*types.Value) *model.SmartBlockSnapshotBase {
	// an explicit envelope id, so a second generation is comparable byte for
	// byte: import mints one for a snapshot that has none (§9, §11.2)
	details := map[string]*types.Value{"id": str("bafyreiticket"), "name": str("Ticket")}
	for k, v := range props {
		details[k] = v
	}
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{Id: "obj1",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		Details: fields(details),
	}
}

// docOptionIds reads the `option_ids` legend out of an exported document.
func docOptionIds(t *testing.T, data []byte) map[string]map[string]string {
	t.Helper()
	var got struct {
		OptionIds map[string]map[string]string `json:"option_ids"`
	}
	require.NoError(t, json.Unmarshal(data, &got))
	return got.OptionIds
}

// legend is the nested literal these tests assert against, spelled once.
func legend(slug string, entries map[string]string) map[string]map[string]string {
	return map[string]map[string]string{slug: entries}
}

// docProperty reads one property value out of an exported document.
func docProperty(t *testing.T, data []byte, slug string) any {
	t.Helper()
	var got struct {
		Properties map[string]any `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(data, &got))
	return got.Properties[slug]
}

// storedList reads a property back off an imported snapshot as a string list.
func storedList(t *testing.T, snap *model.SmartBlockSnapshotBase, key string) []string {
	t.Helper()
	require.NotNil(t, snap.Details)
	return valueStringList(snap.Details.Fields[key])
}

// The motivating shape, and the reason the key is qualified rather than the
// bare name: one name, two properties, two different options. A legend keyed
// by the name alone could carry only one of them, and whichever lost would
// have its value silently re-pointed at the other property's option.
func TestOptionRefs_TwoPropertiesShareAnOptionName(t *testing.T) {
	// given
	space := spaceOptions{
		"status": {{id: "bafyopt1", name: "High"}},
		"tag":    {{id: "bafyopt2", name: "High"}},
	}
	snap := optionSnapshot(map[string]*types.Value{
		"status": strList("bafyopt1"),
		"tag":    strList("bafyopt2"),
	})
	want := map[string]map[string]string{
		"Status": {"High": "bafyopt1"},
		"Tag":    {"High": "bafyopt2"},
	}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{ResolveOptions: space})
	require.NoError(t, err)

	// then
	require.NoError(t, Validate(data))
	assert.Equal(t, want, docOptionIds(t, data))
	assert.Equal(t, []any{"High"}, docProperty(t, data, "Status"))
	assert.Equal(t, []any{"High"}, docProperty(t, data, "Tag"))

	_, back, err := Unmarshal(data, Options{ResolveOptions: space})
	require.NoError(t, err)
	assert.Equal(t, []string{"bafyopt1"}, storedList(t, back, "status"))
	assert.Equal(t, []string{"bafyopt2"}, storedList(t, back, "tag"))
}

// Defect 1 of 2, measured: a space holding two options with one name under
// one relation. Name resolution returns the first, so an object sitting on
// the SECOND came back pointing at the first — 7 objects on a 34 339-object
// account. The legend names the option the document was exported from.
func TestOptionRefs_DuplicateNameKeepsTheOptionTheObjectWasOn(t *testing.T) {
	// given — the pool lists "books" twice; the object is on the second one
	space := spaceOptions{"tag": {
		{id: "bafyfirst", name: "books"},
		{id: "bafysecond", name: "books"},
	}}
	snap := optionSnapshot(map[string]*types.Value{"tag": strList("bafysecond")})

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{ResolveOptions: space})
	require.NoError(t, err)
	_, back, err := Unmarshal(data, Options{ResolveOptions: space})
	require.NoError(t, err)

	// then
	assert.Equal(t, legend("Tag", map[string]string{"books": "bafysecond"}), docOptionIds(t, data))
	assert.Equal(t, []string{"bafysecond"}, storedList(t, back, "tag"))
	// and the name resolution the legend overrides really does answer the
	// other option, so this test cannot pass by the fallback agreeing
	id, ok := space.OptionId("tag", "books")
	require.True(t, ok)
	assert.Equal(t, "bafyfirst", id, "the fixture must reproduce the first-match scan")
}

// The one case the legend cannot rescue, stated so it is not discovered
// later: ONE object holding BOTH same-named options. The document spells
// ["books", "books"] and a JSON list of two identical strings has no way to
// say which entry means which option, so the legend holds the first and both
// values land on it — the collapse §11 already documents for name resolution,
// no worse than today and now deterministic, which is what keeps a second
// export byte-identical to the first.
func TestOptionRefs_SameNameTwiceInOneValueCollapses(t *testing.T) {
	// given
	space := spaceOptions{"tag": {
		{id: "bafyfirst", name: "books"},
		{id: "bafysecond", name: "books"},
	}}
	snap := optionSnapshot(map[string]*types.Value{"tag": strList("bafysecond", "bafyfirst")})

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{ResolveOptions: space})
	require.NoError(t, err)
	_, back, err := Unmarshal(data, Options{ResolveOptions: space})
	require.NoError(t, err)

	// then — the value keeps its arity, the identities collapse onto the
	// first one written, and the legend says so out loud
	assert.Equal(t, []any{"books", "books"}, docProperty(t, data, "Tag"))
	assert.Equal(t, legend("Tag", map[string]string{"books": "bafysecond"}), docOptionIds(t, data))
	assert.Equal(t, []string{"bafysecond", "bafysecond"}, storedList(t, back, "tag"))

	// and the collapse is a FIXPOINT: exporting what came back reproduces the
	// document (§11.3). Dropping the entry instead would hand the choice to
	// the resolver's list order and make this second generation differ.
	again, err := Marshal(model.SmartBlockType_Page, back, Options{ResolveOptions: space})
	require.NoError(t, err)
	assert.Equal(t, string(data), string(again))
}

// Defect 2 of 2: the option is renamed in the target space before the
// document is read back. Name resolution finds nothing — or, worse, finds a
// DIFFERENT option that has since taken the old name — and the wiring mints a
// third option carrying the stale name. The id wins.
func TestOptionRefs_RenamedOptionResolvesById(t *testing.T) {
	// given
	source := spaceOptions{"tag": {{id: "bafyorig", name: "High"}}}
	target := spaceOptions{"tag": {
		{id: "bafyorig", name: "Urgent"}, // renamed since the export
		{id: "bafyother", name: "High"},  // and a different option took the name
	}}
	snap := optionSnapshot(map[string]*types.Value{"tag": strList("bafyorig")})

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{ResolveOptions: source})
	require.NoError(t, err)
	_, back, err := Unmarshal(data, Options{ResolveOptions: target})
	require.NoError(t, err)

	// then
	assert.Equal(t, legend("Tag", map[string]string{"High": "bafyorig"}), docOptionIds(t, data))
	assert.Equal(t, []string{"bafyorig"}, storedList(t, back, "tag"),
		"the id names the option the document came from; the name now names another")
	id, ok := target.OptionId("tag", "High")
	require.True(t, ok)
	assert.Equal(t, "bafyother", id, "the fixture must make name resolution answer differently")
}

// The fallback chain is the point of the whole design: a bundle carried to a
// space that never saw those option ids has to keep working exactly as it
// does without the legend. The id is checked for liveness against the target
// relation, and an id that is not an option there is simply not an answer.
func TestOptionRefs_UnknownIdFallsBackToTheName(t *testing.T) {
	// given
	source := spaceOptions{"tag": {{id: "bafysource", name: "High"}}}
	target := spaceOptions{"tag": {{id: "bafytarget", name: "High"}}}
	snap := optionSnapshot(map[string]*types.Value{"tag": strList("bafysource")})

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{ResolveOptions: source})
	require.NoError(t, err)
	_, back, err := Unmarshal(data, Options{ResolveOptions: target})
	require.NoError(t, err)

	// then
	assert.Equal(t, legend("Tag", map[string]string{"High": "bafysource"}), docOptionIds(t, data))
	assert.Equal(t, []string{"bafytarget"}, storedList(t, back, "tag"))
}

// An id that is live under some OTHER relation is not an answer either: the
// legend qualifies the name with a property, and the liveness question is
// asked of that property's pool.
func TestOptionRefs_IdLiveUnderAnotherRelationIsNotAnAnswer(t *testing.T) {
	// given — the target holds bafysource, but as an option of `status`
	source := spaceOptions{"tag": {{id: "bafysource", name: "High"}}}
	target := spaceOptions{
		"status": {{id: "bafysource", name: "High"}},
		"tag":    {{id: "bafytarget", name: "High"}},
	}
	snap := optionSnapshot(map[string]*types.Value{"tag": strList("bafysource")})

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{ResolveOptions: source})
	require.NoError(t, err)
	_, back, err := Unmarshal(data, Options{ResolveOptions: target})
	require.NoError(t, err)

	// then
	assert.Equal(t, []string{"bafytarget"}, storedList(t, back, "tag"))
}

// A name carrying what used to be the separator — `C#`, `#1 priority` — is
// nothing special now: there is no separator to collide with, so the name is
// the inner key character for character. The case is kept because it is the
// one the flat spelling had to reason about (split at the LAST `#`), and the
// hazard is worth a standing regression rather than an argument.
func TestOptionRefs_NameCarryingTheOldSeparator(t *testing.T) {
	for _, name := range []string{"C#", "#1 priority", "a#b#c", "#"} {
		t.Run(name, func(t *testing.T) {
			// given
			space := spaceOptions{"language": {{id: "bafyopt", name: name}}}
			snap := optionSnapshot(map[string]*types.Value{"language": strList("bafyopt")})
			opts := Options{ResolveOptions: space, ResolveFormat: selectFormats}

			// when
			data, err := Marshal(model.SmartBlockType_Page, snap, opts)
			require.NoError(t, err)
			_, back, err := Unmarshal(data, opts)
			require.NoError(t, err)

			// then
			require.NoError(t, Validate(data))
			assert.Equal(t, legend("language", map[string]string{name: "bafyopt"}), docOptionIds(t, data))
			assert.Equal(t, []string{"bafyopt"}, storedList(t, back, "language"))
		})
	}
}

// An ordinary tag name with a space in it — `import issue` is a real one from
// the account this was measured on — which the deleted plain-label charset
// ([A-Za-z0-9_-]) rejected outright. The inner key carries no charset rule at
// all, which is what makes the legend usable on real option names.
func TestOptionRefs_NameCarryingASpace(t *testing.T) {
	// given
	space := spaceOptions{"tag": {{id: "bafyopt", name: "import issue"}}}
	snap := optionSnapshot(map[string]*types.Value{"tag": strList("bafyopt")})

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{ResolveOptions: space})
	require.NoError(t, err)

	// then
	require.NoError(t, Validate(data), "a name with a space must be a legal legend key:\n%s", data)
	assert.Equal(t, legend("Tag", map[string]string{"import issue": "bafyopt"}), docOptionIds(t, data))

	_, back, err := Unmarshal(data, Options{ResolveOptions: space})
	require.NoError(t, err)
	assert.Equal(t, []string{"bafyopt"}, storedList(t, back, "tag"))
}

// A value that is neither a known name nor a legend key travels unchanged, as
// it does today: creating a missing option is the wiring's job (§3).
func TestOptionRefs_UnknownValuePassesThrough(t *testing.T) {
	// given
	space := spaceOptions{"tag": {{id: "bafyopt", name: "High"}}}
	doc := `{"version": 2, "id": "obj1", "properties": {"tag": ["Brand new"]},
		"option_ids": {"tag": {"High": "bafyopt"}}}`

	// when
	_, back, err := Unmarshal([]byte(doc), Options{ResolveOptions: space, GenerateId: seqIds("g")})

	// then
	require.NoError(t, err)
	assert.Equal(t, []string{"Brand new"}, storedList(t, back, "tag"))
}

// The legend is identity, not compaction, so it is not behind the compaction
// flag — and it is not pruned, because there is nothing unused to prune: the
// only place an entry is recorded is the substitution itself.
//
// OmitIds is the ONE shape that drops it, and that is not an exception to the
// rule but the same rule read the other way: the legend is nothing but ids, so
// a shape that declares itself id-less and then ships a map of them is not one
// (§9). This used to write them, which is what made `rich_omit_ids.json` an
// id-less golden carrying two option ids.
func TestOptionRefs_WrittenWithoutCompactionAndOnlyForWhatIsWritten(t *testing.T) {
	// given — one option the resolver knows and one raw id it does not
	space := spaceOptions{"tag": {{id: "bafyknown", name: "High"}}}
	snap := optionSnapshot(map[string]*types.Value{"tag": strList("bafyknown", "bafyunknown")})

	for name, tc := range map[string]struct {
		opts       Options
		wantLegend bool
	}{
		"plain":   {Options{ResolveOptions: space}, true},
		"compact": {Options{ResolveOptions: space, CompactBlockLabels: true}, true},
		"omitIds": {Options{ResolveOptions: space, OmitIds: true}, false},
	} {
		t.Run(name, func(t *testing.T) {
			// when
			data, err := Marshal(model.SmartBlockType_Page, snap, tc.opts)
			require.NoError(t, err)

			// then
			if tc.wantLegend {
				assert.Equal(t, legend("Tag", map[string]string{"High": "bafyknown"}), docOptionIds(t, data),
					"the unresolved id is written verbatim and owes no entry")
			} else {
				assert.Nil(t, docOptionIds(t, data),
					"an id-less shape ships no legend of ids (§9)")
				assert.NotContains(t, string(data), "bafyknown",
					"and the id it would have carried appears nowhere else either")
			}
			assert.Equal(t, []any{"High", "bafyunknown"}, docProperty(t, data, "Tag"))
		})
	}
}

// Filter values and sort custom orders are option slots too (§3, §6.2), and
// they take the same legend — the rule §3 states is "everywhere", and a slot
// that wrote the name without recording the entry would be a slot whose
// options silently keep the old behavior.
func TestOptionRefs_FilterValuesAndCustomOrders(t *testing.T) {
	// given
	space := spaceOptions{"tag": {
		{id: "bafyfilter", name: "Filtered"},
		{id: "bafyorder", name: "Ordered"},
	}}
	dv := &model.BlockContentDataview{
		RelationLinks: []*model.RelationLink{{Key: "tag", Format: model.RelationFormat_tag}},
		Views: []*model.BlockContentDataviewView{{Id: "v1", Name: "All",
			Filters: []*model.BlockContentDataviewFilter{{
				Id: "f1", RelationKey: "tag", Format: model.RelationFormat_tag,
				Condition: model.BlockContentDataviewFilter_In,
				Value:     strList("bafyfilter"),
			}},
			Sorts: []*model.BlockContentDataviewSort{{
				Id: "s1", RelationKey: "tag", Format: model.RelationFormat_tag,
				Type:        model.BlockContentDataviewSort_Custom,
				CustomOrder: []*types.Value{str("bafyorder")},
			}},
		}},
	}
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "obj1", ChildrenIds: []string{"dv1"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "dv1", Content: &model.BlockContentOfDataview{Dataview: dv}},
		},
		Details: fields(map[string]*types.Value{"name": str("Board")}),
	}
	opts := Options{ResolveOptions: space}
	want := legend("Tag", map[string]string{"Filtered": "bafyfilter", "Ordered": "bafyorder"})

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	_, back, err := Unmarshal(data, opts)
	require.NoError(t, err)

	// then
	require.NoError(t, Validate(data))
	assert.Equal(t, want, docOptionIds(t, data))
	view := backView(t, back)
	assert.Equal(t, []string{"bafyfilter"}, valueStringList(view.Filters[0].Value))
	require.Len(t, view.Sorts[0].CustomOrder, 1)
	assert.Equal(t, "bafyorder", view.Sorts[0].CustomOrder[0].GetStringValue())
}

// backView digs the single dataview view out of an imported snapshot.
func backView(t *testing.T, snap *model.SmartBlockSnapshotBase) *model.BlockContentDataviewView {
	t.Helper()
	for _, b := range snap.Blocks {
		if c, ok := b.Content.(*model.BlockContentOfDataview); ok {
			require.Len(t, c.Dataview.Views, 1)
			return c.Dataview.Views[0]
		}
	}
	t.Fatal("no dataview in the imported snapshot")
	return nil
}

// The legend's OUTER key carries the SPELLING the document writes, not the
// stored key — the reader that resolves it is reading the document and has no
// store to translate with.
func TestOptionRefs_KeyIsTheSpellingNotTheStoredKey(t *testing.T) {
	// given — a same-named twin ahead of it in the pool, so name resolution
	// answers a DIFFERENT option and only the legend can be right. Without
	// it the fallback rescues a key built from the wrong half and the test
	// passes while looking up nothing.
	space := spaceOptions{"6a32d4856761631534b22f85": {
		{id: "bafydecoy", name: "High"},
		{id: "bafyopt", name: "High"},
	}}
	snap := optionSnapshot(map[string]*types.Value{"6a32d4856761631534b22f85": strList("bafyopt")})
	opts := Options{ResolveOptions: space, ResolveFormat: selectFormats, Keys: slugVocab{
		slugs: map[string]string{"6a32d4856761631534b22f85": "priority"},
	}}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	_, back, err := Unmarshal(data, opts)
	require.NoError(t, err)

	// then
	assert.Equal(t, legend("priority", map[string]string{"High": "bafyopt"}), docOptionIds(t, data))
	assert.Equal(t, []string{"bafyopt"}, storedList(t, back, "6a32d4856761631534b22f85"))
	id, ok := space.OptionId("6a32d4856761631534b22f85", "High")
	require.True(t, ok)
	assert.Equal(t, "bafydecoy", id, "the fixture must make name resolution answer differently")
}

// THE CAPABILITY HOLE, CLOSED. A property whose SPELLING carries a `#` used
// to get no entry at all: `strcase.ToSnake("C#")` is `c#`, a legal api slug,
// and a flat key `<name>#<slug>` with a separator on both sides of the split
// was not invertible — so the escape hatch was unreachable exactly where a
// user's own naming needed it, and the value fell back to name resolution in
// silence. Nesting has no separator, so the entry is simply written.
//
// The pool lists a same-named decoy FIRST, so name resolution answers a
// different id: without it the fallback would rescue the value and this test
// would pass whether or not the entry was written.
func TestOptionRefs_SeparatorInThePropertySpellingStillGetsAnEntry(t *testing.T) {
	// given
	space := spaceOptions{"csharpTag": {
		{id: "bafydecoy", name: "High"},
		{id: "bafyopt", name: "High"},
	}}
	snap := optionSnapshot(map[string]*types.Value{"csharpTag": strList("bafyopt")})
	opts := Options{ResolveOptions: space, ResolveFormat: selectFormats, Keys: slugVocab{
		slugs: map[string]string{"csharpTag": "c#_lang"},
	}}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	_, back, err := Unmarshal(data, opts)
	require.NoError(t, err)

	// then
	require.NoError(t, Validate(data))
	assert.Equal(t, legend("c#_lang", map[string]string{"High": "bafyopt"}), docOptionIds(t, data))
	assert.Equal(t, []any{"High"}, docProperty(t, data, "c#_lang"))
	assert.Equal(t, []string{"bafyopt"}, storedList(t, back, "csharpTag"),
		"the legend, not name resolution, is what carries the identity here")
	id, ok := space.OptionId("csharpTag", "High")
	require.True(t, ok)
	assert.Equal(t, "bafydecoy", id, "the fixture must make name resolution answer differently")
}

// The other residue nesting removes: an option name past the bound the joined
// key carried. There is no joined key, and the inner key is bounded only by
// being non-empty, so a name of any length gets its entry — as it must, since
// the same string is already sitting in the value slot beside it.
func TestOptionRefs_OverLongNameStillGetsAnEntry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		length int
	}{
		{"at the old bound", maxPropertyKeyLen},
		{"past the old bound", maxPropertyKeyLen + 1},
		{"far past it", maxPropertyKeyLen * 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given — a same-named decoy again, so the entry is load-bearing
			optName := strings.Repeat("n", tc.length)
			space := spaceOptions{"tag": {
				{id: "bafydecoy", name: optName},
				{id: "bafyopt", name: optName},
			}}
			snap := optionSnapshot(map[string]*types.Value{"tag": strList("bafyopt")})
			opts := Options{ResolveOptions: space}

			// when
			data, err := Marshal(model.SmartBlockType_Page, snap, opts)
			require.NoError(t, err)
			_, back, err := Unmarshal(data, opts)
			require.NoError(t, err)

			// then
			require.NoError(t, Validate(data), "%s", data)
			assert.Equal(t, legend("Tag", map[string]string{optName: "bafyopt"}), docOptionIds(t, data))
			assert.Equal(t, []string{"bafyopt"}, storedList(t, back, "tag"))
		})
	}
}

// A reader with no option resolver has no space to ask whether an id is live
// there, and an id it cannot check is not an answer it can give — so the
// entries are ignored and the value passes through as the name, exactly as it
// does today. This is what keeps a package-only read unchanged by the legend.
func TestOptionRefs_ReaderWithoutAResolverIgnoresTheLegend(t *testing.T) {
	// given
	doc := `{"version": 2, "id": "obj1", "properties": {"tag": ["High"]},
		"option_ids": {"tag": {"High": "bafyopt"}}}`

	// when
	_, back, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})

	// then
	require.NoError(t, err)
	assert.Equal(t, []string{"High"}, storedList(t, back, "tag"))
}

// The published schema and the Go restatement have to admit the same keys, or
// an external validator (§12) and this package disagree about what a document
// is. `option_ids` carries a rule at BOTH levels and the table runs both.
//
// The outer rule is the writable-key rule every property spelling carries.
// The inner rule is only "non-empty", deliberately: an option name is the
// same string the value slot already holds, so anything stricter would refuse
// a legend entry for a value the document itself carries — the `C#` hole one
// level down.
func TestOptionRefs_LegendKeyRulesAreTheSameInBothValidators(t *testing.T) {
	for _, tc := range []struct {
		name  string
		slug  string
		optId string
		valid bool
	}{
		{"a plain spelling", "tag", "High", true},
		{"a spelling carrying the old separator", "c#_lang", "High", true},
		{"an option name with a space", "tag", "import issue", true},
		{"an option name carrying the old separator", "tag", "C#", true},
		{"an option name that is only the old separator", "tag", "#", true},
		{"an option name past the old joined bound", "tag", strings.Repeat("n", 400), true},
		{"a spelling at the bound", strings.Repeat("p", 128), "High", true},
		{"a spelling past the bound", strings.Repeat("p", 129), "High", false},
		{"an empty spelling", "", "High", false},
		{"a control character in the spelling", "ta\ng", "High", false},
		{"an empty option name", "tag", "", false},
		// the bound counts CHARACTERS in both validators — a byte count
		// would put a 65-character Cyrillic spelling past 128 and refuse a
		// document the package writes
		{"a Cyrillic spelling at the bound", strings.Repeat("\u044f", 128), "High", true},
		{"a Cyrillic spelling past it", strings.Repeat("\u044f", 129), "High", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			raw, err := json.Marshal(map[string]any{
				"version":    2,
				"option_ids": map[string]any{tc.slug: map[string]string{tc.optId: "bafyreiopt"}},
			})
			require.NoError(t, err)

			// when — ValidateWarn, because an unreachable entry is a WARNING
			// and every document here has one: the point is the key rule, not
			// the census
			schemaErr := ValidateWarn(raw, func(Issue) {})

			// then
			if tc.valid {
				assert.NoError(t, schemaErr, "%s", raw)
			} else {
				assert.Error(t, schemaErr, "%s", raw)
			}
		})
	}
}

// The three legends have a canonical order (§2, §4), and `option_ids` is last
// of them because its OUTER keys are property spellings: the legend that
// inverts a spelling has to precede the legend keyed by one, so a reader
// working through the document linearly meets `property_internal_keys` first.
//
// No golden pins this — all four carry `option_ids` and none carries
// `property_internal_keys`, so the two never appear together in a frozen document.
func TestOptionRefs_TheLegendFollowsPropertyKeys(t *testing.T) {
	// given — a stored key the bundled table cannot invert, so the document
	// owes a property_internal_keys entry, carrying a select value so it owes an
	// option_ids entry too
	space := spaceOptions{"6a32d4856761631534b22f85": {{id: "bafyopt", name: "High"}}}
	snap := optionSnapshot(map[string]*types.Value{"6a32d4856761631534b22f85": strList("bafyopt")})
	opts := Options{ResolveOptions: space, ResolveFormat: selectFormats, Keys: slugVocab{
		slugs: map[string]string{"6a32d4856761631534b22f85": "priority"},
	}}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)

	// then
	require.NoError(t, Validate(data))
	s := string(data)
	properties := strings.Index(s, `"properties"`)
	propertyKeys := strings.Index(s, `"property_internal_keys"`)
	optionIds := strings.Index(s, `"option_ids"`)
	require.NotEqual(t, -1, propertyKeys, "the fixture must produce a property_internal_keys legend:\n%s", s)
	require.NotEqual(t, -1, optionIds, "and an option_ids legend:\n%s", s)
	assert.Less(t, properties, propertyKeys, "properties precede the legends:\n%s", s)
	assert.Less(t, propertyKeys, optionIds,
		"option_ids is keyed by spellings property_internal_keys inverts, so it comes after:\n%s", s)
	// and the outer key is the SPELLING, which is what makes the order matter
	assert.Equal(t, legend("priority", map[string]string{"High": "bafyopt"}), docOptionIds(t, data))
}

// slugVocab spells the keys it is given and inverts them, and nothing else —
// a conforming vocabulary just wide enough to move a spelling.
type slugVocab struct {
	BundledKeyVocabulary
	slugs map[string]string
}

func (v slugVocab) PropertySlug(key string) string {
	if slug, ok := v.slugs[key]; ok {
		return slug
	}
	return BundledKeyVocabulary{}.PropertySlug(key)
}

func (v slugVocab) PropertyKey(slug string) (string, bool) {
	for key, s := range v.slugs {
		if s == slug {
			return key, true
		}
	}
	return BundledKeyVocabulary{}.PropertyKey(slug)
}

//
// ---- the property vocabulary (optionrefs.go) ----
//

// optionIdsWarnings keeps the warnings addressed at the legend and drops the
// rest, so a corpus that legitimately warns about something else (an unguarded
// date filter, a tag-shaped literal) cannot make one of these tests pass or
// fail for a reason it is not about.
func optionIdsWarnings(issues []Issue) []Issue {
	var out []Issue
	for _, i := range issues {
		if strings.HasPrefix(i.Path, "/option_ids/") {
			out = append(out, i)
		}
	}
	return out
}

// An outer key naming no property the document uses qualifies nothing: import
// indexes the legend by the spelling the slot it is resolving wrote, so an
// entry filed under `priorty` is never asked for and the value falls back to
// name resolution — the silent degradation the warning exists to say out loud.
// It stays a WARNING: a legend is allowed to carry more than one document
// needs, and hard-rejecting one would contradict that.
//
// The pool lists a same-named DECOY first, so name resolution and the legend
// answer different ids: without it both branches would land on one id and the
// test would pass while asking nothing.
func TestOptionRefs_ALegendEntryForAPropertyTheDocumentDoesNotUse(t *testing.T) {
	space := spaceOptions{"tag": {
		{id: "bafyname", name: "High"},   // what the NAME resolves to
		{id: "bafylegend", name: "High"}, // what the LEGEND names
	}}
	for _, tc := range []struct {
		name     string
		slug     string
		wantWarn bool
		wantId   string
	}{
		{"a typo in the property spelling", "priorty", true, "bafyname"},
		{"the spelling the document uses", "tag", false, "bafylegend"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			doc := fmt.Sprintf(`{"version": 2, "id": "obj1", "properties": {"tag": ["High"]},
				"option_ids": {%q: {"High": "bafylegend"}}}`, tc.slug)

			// when
			var warned []Issue
			validateErr := ValidateWarn([]byte(doc), func(i Issue) { warned = append(warned, i) })
			_, back, err := Unmarshal([]byte(doc), Options{ResolveOptions: space, GenerateId: seqIds("g")})

			// then
			require.NoError(t, validateErr, "an unreachable entry is a warning, not an error (§9a)")
			require.NoError(t, err)
			got := optionIdsWarnings(warned)
			if tc.wantWarn {
				require.Len(t, got, 1, "warnings: %v", warned)
				assert.Equal(t, "/option_ids/"+tc.slug, got[0].Path)
				assert.Contains(t, got[0].Message, `spells "priorty"`)
			} else {
				assert.Empty(t, got, "the document spells this property; nothing to report")
			}
			assert.Equal(t, []string{tc.wantId}, storedList(t, back, "tag"))
		})
	}
}

// The census is a statement about the DOCUMENT, not about one slot: a
// property a document uses only in a dataview filter — or only in a sort's
// custom order — is a property it knows, and the legend under it has to be
// honored. A census that read the `properties` map alone would turn these
// entries away, and it would do it in silence, because the value still
// resolves by name afterwards. So the pool lists a same-named decoy FIRST:
// name resolution answers `bafydecoy`, and only the legend can answer
// `bafylegend`.
//
// Written as documents rather than round trips on purpose. An exported
// dataview carries a `properties` list naming its own properties, which would
// put `tag` in the census by a second route and leave the filter and sort
// positions untested.
func TestOptionRefs_PropertySpelledOnlyInsideADataview(t *testing.T) {
	space := spaceOptions{"tag": {
		{id: "bafydecoy", name: "High"},
		{id: "bafylegend", name: "High"},
	}}
	for _, tc := range []struct {
		name string
		view string
	}{
		{"only in a filter", `{"id": "v1", "filters":
			[{"property": "tag", "condition": "in", "value": ["High"]}]}`},
		{"only in a nested filter", `{"id": "v1", "filters": [{"operator": "or", "filters":
			[{"property": "tag", "condition": "in", "value": ["High"]}]}]}`},
		{"only in a sort's custom order", `{"id": "v1", "sorts":
			[{"property": "tag", "direction": "custom", "custom_order": ["High"]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			doc := fmt.Sprintf(`{"version": 2, "id": "obj1",
				"option_ids": {"tag": {"High": "bafylegend"}},
				"blocks": [{"id": "dv1", "type": "dataview", "views": [%s]}]}`, tc.view)

			// when
			var warned []Issue
			validateErr := ValidateWarn([]byte(doc), func(i Issue) { warned = append(warned, i) })
			_, back, err := Unmarshal([]byte(doc), Options{ResolveOptions: space, GenerateId: seqIds("g")})

			// then
			require.NoError(t, validateErr, doc)
			require.NoError(t, err)
			assert.Empty(t, optionIdsWarnings(warned), "the document spells `tag` in this dataview")
			assert.Equal(t, []string{"bafylegend"}, resolvedOptionIds(t, back),
				"the legend under a filter-only property must still be honored")
			// and the fallback the legend overrides really does answer the
			// other option, so this cannot pass by the two agreeing
			id, ok := space.OptionId("tag", "High")
			require.True(t, ok)
			assert.Equal(t, "bafydecoy", id, "the fixture must reproduce the first-match scan")
		})
	}
}

// resolvedOptionIds reads back whatever select values an imported dataview
// carries — the filter value or the sort's custom order, whichever the
// document had.
func resolvedOptionIds(t *testing.T, snap *model.SmartBlockSnapshotBase) []string {
	t.Helper()
	view := backView(t, snap)
	var out []string
	var walk func(fs []*model.BlockContentDataviewFilter)
	walk = func(fs []*model.BlockContentDataviewFilter) {
		for _, f := range fs {
			out = append(out, valueStringList(f.Value)...)
			walk(f.NestedFilters)
		}
	}
	walk(view.Filters)
	for _, s := range view.Sorts {
		for _, v := range s.CustomOrder {
			out = append(out, v.GetStringValue())
		}
	}
	return out
}

// The census itself, position by position (optionrefs.go). Each document
// spells one probe in exactly one place and the census has to find it.
//
// This IS the guard now. The census used to exist in two implementations —
// import's, over the decoded document, and Validate's, over the undecoded
// one — with an agreement test between them; import no longer takes a census
// at all, so the twin is gone and so is that test. What the agreement really
// stood for is this table: a census that stopped covering a position would
// make Validate warn about entries import honours, and it is this list, not
// a second implementation, that says it does not.
//
// The last two entries are the boundary: a filter's `nested_property` names a
// property of the object the filter walks TO, and an `option_ids` key is not
// a use of the property it names, or every typo would vouch for itself.
func TestOptionRefs_ThePropertyCensusCoversEveryPosition(t *testing.T) {
	const probe = "probe_property"
	for _, tc := range []struct {
		name    string
		doc     string
		counted bool
	}{
		{"a properties member", `{"properties": {"probe_property": ["High"]}}`, true},
		{"a property_internal_keys spelling", `{"property_internal_keys": {"probe_property": "storedKey"}}`, true},
		{"a type_properties key", `{"kind": "object_type",
			"type_settings": {"property_definitions": [{"property": "probe_property", "format": "select"}]}}`, true},
		{"a property block's key", `{"blocks":
			[{"type": "property", "property": "probe_property"}]}`, true},
		{"a link block's shown properties", `{"blocks":
			[{"type": "link", "object_id": "bafyreitarget", "properties": ["probe_property"]}]}`, true},
		{"a dataview's properties list", `{"blocks": [{"type": "dataview",
			"properties": [{"property": "probe_property", "format": "select"}]}]}`, true},
		{"a view's group_by", `{"blocks": [{"type": "dataview",
			"views": [{"id": "v1", "group_by": "probe_property"}]}]}`, true},
		{"a view's cover_property", `{"blocks": [{"type": "dataview",
			"views": [{"id": "v1", "cover_property": "probe_property"}]}]}`, true},
		{"a view's end_property", `{"blocks": [{"type": "dataview",
			"views": [{"id": "v1", "end_property": "probe_property"}]}]}`, true},
		{"a view column", `{"blocks": [{"type": "dataview", "views":
			[{"id": "v1", "columns": [{"property": "probe_property"}]}]}]}`, true},
		{"a sort", `{"blocks": [{"type": "dataview", "views":
			[{"id": "v1", "sorts": [{"property": "probe_property"}]}]}]}`, true},
		{"a filter", `{"blocks": [{"type": "dataview", "views":
			[{"id": "v1", "filters": [{"property": "probe_property", "condition": "in"}]}]}]}`, true},
		{"a nested filter", `{"blocks": [{"type": "dataview", "views": [{"id": "v1", "filters":
			[{"operator": "and", "filters": [{"property": "probe_property"}]}]}]}]}`, true},
		{"a property block inside a table cell", `{"blocks": [{"type": "table",
			"columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells":
			[{"type": "property", "property": "probe_property"}]}]}]}`, true},
		{"a dataview inside a table cell", `{"blocks": [{"type": "table",
			"columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": [[{"type": "dataview",
			"views": [{"id": "v1", "filters": [{"property": "probe_property"}]}]}]]}]}]}`, true},
		{"a filter's nested_property", `{"blocks": [{"type": "dataview", "views": [{"id": "v1",
			"filters": [{"property": "assignee", "nested_property": "probe_property"}]}]}]}`, false},
		{"the option_ids key itself", `{"option_ids":
			{"probe_property": {"High": "bafyreiopt"}}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			data := []byte(`{"version": 2, "id": "obj1",` + strings.TrimPrefix(tc.doc, "{"))

			// when
			var raw map[string]any
			require.NoError(t, json.Unmarshal(data, &raw))

			// then
			assert.Equal(t, tc.counted, rawPropertySpellings(raw)[probe])
		})
	}
}
