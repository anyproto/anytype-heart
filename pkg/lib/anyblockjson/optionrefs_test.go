package anyblockjson

// optionrefs_test.go — the qualified option legend (optionrefs.go, §3, §9a).
//
// Every test here is written against a resolver that behaves the way the real
// one does: `spaceOptions` scans a per-relation list and answers with the
// FIRST match by name, which is exactly `storeresolver.OptionId`. That is not
// incidental — the two defects this legend closes are both consequences of
// that scan, so a resolver that answered by a map would make the tests pass
// for the wrong reason.

import (
	"encoding/json"
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

// docRefs reads the `refs` legend out of an exported document.
func docRefs(t *testing.T, data []byte) map[string]string {
	t.Helper()
	var got struct {
		Refs map[string]string `json:"refs"`
	}
	require.NoError(t, json.Unmarshal(data, &got))
	return got.Refs
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
	want := map[string]string{"High#status": "bafyopt1", "High#tag": "bafyopt2"}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{ResolveOptions: space})
	require.NoError(t, err)

	// then
	require.NoError(t, Validate(data))
	assert.Equal(t, want, docRefs(t, data))
	assert.Equal(t, []any{"High"}, docProperty(t, data, "status"))
	assert.Equal(t, []any{"High"}, docProperty(t, data, "tag"))

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
	assert.Equal(t, map[string]string{"books#tag": "bafysecond"}, docRefs(t, data))
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
	assert.Equal(t, []any{"books", "books"}, docProperty(t, data, "tag"))
	assert.Equal(t, map[string]string{"books#tag": "bafysecond"}, docRefs(t, data))
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
	assert.Equal(t, map[string]string{"High#tag": "bafyorig"}, docRefs(t, data))
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
	assert.Equal(t, map[string]string{"High#tag": "bafysource"}, docRefs(t, data))
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

// A name carrying the separator itself — `C#`, `#1 priority` — still lands
// whole, because the key splits at the LAST separator and the property
// spelling on the right of it can never hold one.
func TestOptionRefs_NameCarryingTheSeparator(t *testing.T) {
	for _, name := range []string{"C#", "#1 priority", "a#b#c"} {
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
			assert.Equal(t, map[string]string{name + "#language": "bafyopt"}, docRefs(t, data))
			assert.Equal(t, []string{"bafyopt"}, storedList(t, back, "language"))
		})
	}
}

// An ordinary tag name with a space in it — `import issue` is a real one from
// the account this was measured on — which the plain-label charset
// ([A-Za-z0-9_-]) rejected outright. The relaxation is what makes the legend
// usable at all.
func TestOptionRefs_NameCarryingASpace(t *testing.T) {
	// given
	space := spaceOptions{"tag": {{id: "bafyopt", name: "import issue"}}}
	snap := optionSnapshot(map[string]*types.Value{"tag": strList("bafyopt")})

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{ResolveOptions: space})
	require.NoError(t, err)

	// then
	require.NoError(t, Validate(data), "a name with a space must be a legal legend key:\n%s", data)
	assert.Equal(t, map[string]string{"import issue#tag": "bafyopt"}, docRefs(t, data))

	_, back, err := Unmarshal(data, Options{ResolveOptions: space})
	require.NoError(t, err)
	assert.Equal(t, []string{"bafyopt"}, storedList(t, back, "tag"))
}

// A value that is neither a known name nor a legend key travels unchanged, as
// it does today: creating a missing option is the wiring's job (§3).
func TestOptionRefs_UnknownValuePassesThrough(t *testing.T) {
	// given
	space := spaceOptions{"tag": {{id: "bafyopt", name: "High"}}}
	doc := `{"version": 1, "id": "obj1", "properties": {"tag": ["Brand new"]},
		"refs": {"High#tag": "bafyopt"}}`

	// when
	_, back, err := Unmarshal([]byte(doc), Options{ResolveOptions: space, GenerateId: seqIds("g")})

	// then
	require.NoError(t, err)
	assert.Equal(t, []string{"Brand new"}, storedList(t, back, "tag"))
}

// The legend is identity, not compaction, so it is not behind the compaction
// flag — and it is not pruned, because there is nothing unused to prune: the
// only place an entry is recorded is the substitution itself.
func TestOptionRefs_WrittenWithoutCompactionAndOnlyForWhatIsWritten(t *testing.T) {
	// given — one option the resolver knows and one raw id it does not
	space := spaceOptions{"tag": {{id: "bafyknown", name: "High"}}}
	snap := optionSnapshot(map[string]*types.Value{"tag": strList("bafyknown", "bafyunknown")})

	for name, opts := range map[string]Options{
		"plain":   {ResolveOptions: space},
		"compact": {ResolveOptions: space, CompactObjectRefs: true},
		"omitIds": {ResolveOptions: space, OmitIds: true},
	} {
		t.Run(name, func(t *testing.T) {
			// when
			data, err := Marshal(model.SmartBlockType_Page, snap, opts)
			require.NoError(t, err)

			// then
			assert.Equal(t, map[string]string{"High#tag": "bafyknown"}, docRefs(t, data),
				"the unresolved id is written verbatim and owes no entry")
			assert.Equal(t, []any{"High", "bafyunknown"}, docProperty(t, data, "tag"))
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
	want := map[string]string{"Filtered#tag": "bafyfilter", "Ordered#tag": "bafyorder"}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	_, back, err := Unmarshal(data, opts)
	require.NoError(t, err)

	// then
	require.NoError(t, Validate(data))
	assert.Equal(t, want, docRefs(t, data))
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

// The legend key carries the SPELLING the document writes, not the stored
// key — the reader that resolves it is reading the document and has no store
// to translate with.
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
	assert.Equal(t, map[string]string{"High#priority": "bafyopt"}, docRefs(t, data))
	assert.Equal(t, []string{"bafyopt"}, storedList(t, back, "6a32d4856761631534b22f85"))
	id, ok := space.OptionId("6a32d4856761631534b22f85", "High")
	require.True(t, ok)
	assert.Equal(t, "bafydecoy", id, "the fixture must make name resolution answer differently")
}

// A property whose SPELLING carries the separator gets no entry: `#` is legal
// in an api key (`strcase.ToSnake("C#")` is `c#`), and a key with one on both
// sides of the split would not be invertible. Those options keep today's
// name-only behavior — correct, merely less faithful — rather than getting an
// entry that could be read two ways.
func TestOptionRefs_SeparatorInThePropertySpellingSuppressesTheEntry(t *testing.T) {
	// given
	space := spaceOptions{"csharpTag": {{id: "bafyopt", name: "High"}}}
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
	assert.Nil(t, docRefs(t, data))
	assert.Equal(t, []any{"High"}, docProperty(t, data, "c#_lang"))
	assert.Equal(t, []string{"bafyopt"}, storedList(t, back, "csharpTag"),
		"name resolution still carries it")
}

// A name past the bound each half of a qualified key carries (the §3
// writable-key rule, 128 characters) gets no entry either; the value is
// still written and still resolves by name.
func TestOptionRefs_OverLongNameGetsNoEntry(t *testing.T) {
	for _, tc := range []struct {
		name    string
		length  int
		wantRef bool
	}{
		{"at the bound", maxPropertyKeyLen, true},
		{"past the bound", maxPropertyKeyLen + 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			optName := strings.Repeat("n", tc.length)
			space := spaceOptions{"tag": {{id: "bafyopt", name: optName}}}
			snap := optionSnapshot(map[string]*types.Value{"tag": strList("bafyopt")})

			// when
			data, err := Marshal(model.SmartBlockType_Page, snap, Options{ResolveOptions: space})
			require.NoError(t, err)

			// then
			require.NoError(t, Validate(data), "%s", data)
			if tc.wantRef {
				assert.Equal(t, map[string]string{optName + "#tag": "bafyopt"}, docRefs(t, data))
			} else {
				assert.Nil(t, docRefs(t, data))
			}
		})
	}
}

// A reader with no option resolver has no space to ask whether an id is live
// there, and an id it cannot check is not an answer it can give — so the
// entries are ignored and the value passes through as the name, exactly as it
// does today. This is what keeps a package-only read unchanged by the legend.
func TestOptionRefs_ReaderWithoutAResolverIgnoresTheLegend(t *testing.T) {
	// given
	doc := `{"version": 1, "id": "obj1", "properties": {"tag": ["High"]},
		"refs": {"High#tag": "bafyopt"}}`

	// when
	_, back, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})

	// then
	require.NoError(t, err)
	assert.Equal(t, []string{"High"}, storedList(t, back, "tag"))
}

// The two key populations are disjoint by shape, and each is reachable only
// from its own kind of slot (§9a). Both directions are pinned, because a
// legend that leaks across slots is exactly the silent re-pointing this whole
// mechanism exists to stop.
func TestOptionRefs_ThePopulationsDoNotLeakIntoEachOther(t *testing.T) {
	space := spaceOptions{"tag": {{id: "bafyopt", name: "High"}}}

	t.Run("a qualified key is not an object id", func(t *testing.T) {
		// given — an object-id slot literally spelling a qualified key
		doc := `{"version": 1, "id": "obj1", "refs": {"High#tag": "bafyopt"},
			"blocks": [{"type": "link", "object_id": "High#tag"}]}`

		// when
		_, back, err := Unmarshal([]byte(doc), Options{ResolveOptions: space, GenerateId: seqIds("g")})

		// then
		require.NoError(t, err)
		assert.Equal(t, "High#tag", linkTarget(t, back))
	})

	t.Run("a bare option name is not an object id", func(t *testing.T) {
		// given — the legend holds High#tag, so a bare "High" binds nothing
		doc := `{"version": 1, "id": "obj1", "refs": {"High#tag": "bafyopt"},
			"blocks": [{"type": "link", "object_id": "High"}]}`

		// when
		_, back, err := Unmarshal([]byte(doc), Options{ResolveOptions: space, GenerateId: seqIds("g")})

		// then
		require.NoError(t, err)
		assert.Equal(t, "High", linkTarget(t, back))
	})

	t.Run("a compaction label is not an option", func(t *testing.T) {
		// given — a plain label whose spelling a select value repeats
		doc := `{"version": 1, "id": "obj1", "refs": {"miovm": "bafyreitarget"},
			"properties": {"tag": ["miovm"]}}`

		// when
		_, back, err := Unmarshal([]byte(doc), Options{ResolveOptions: space, GenerateId: seqIds("g")})

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"miovm"}, storedList(t, back, "tag"))
	})
}

func linkTarget(t *testing.T, snap *model.SmartBlockSnapshotBase) string {
	t.Helper()
	for _, b := range snap.Blocks {
		if c, ok := b.Content.(*model.BlockContentOfLink); ok {
			return c.Link.TargetBlockId
		}
	}
	t.Fatal("no link block in the imported snapshot")
	return ""
}

// The published schema and the Go predicate have to admit the same keys, or
// an external validator (§12) and this package disagree about what a document
// is. The table runs both against the same inputs.
func TestOptionRefs_KeyShapeIsTheSameInBothValidators(t *testing.T) {
	for _, tc := range []struct {
		key   string
		valid bool
	}{
		{"miovm", true},                             // plain label
		{"a-b_C9", true},                            // plain charset, whole
		{"has space", false},                        // plain charset is unchanged
		{"High#tag", true},                          // qualified
		{"import issue#tag", true},                  // a name with a space
		{"C##language", true},                       // a name with the separator
		{"a#b#c", true},                             // splits at the last one
		{"#tag", false},                             // no name
		{"High#", false},                            // no property
		{"#", false},                                // neither
		{strings.Repeat("n", 128) + "#tag", true},   // both halves at the bound
		{strings.Repeat("n", 129) + "#tag", false},  // the name half past it
		{"High#" + strings.Repeat("p", 129), false}, // the property half past it
		{"High#" + strings.Repeat("p", 128), true},  //
		{"a\nb#tag", false},                         // a control character in the name
		{"High#ta\ng", false},                       // and one in the property
		{strings.Repeat("l", 65), false},            // a plain label past 64
		// the bound counts CHARACTERS in both validators — a byte count
		// would put a 65-character Cyrillic name past 128 and refuse a
		// document the package writes
		{strings.Repeat("\u044f", 128) + "#tag", true},
		{strings.Repeat("\u044f", 129) + "#tag", false},
	} {
		t.Run(tc.key, func(t *testing.T) {
			// given
			raw, err := json.Marshal(map[string]any{
				"version": 1,
				"refs":    map[string]string{tc.key: "bafyreitarget"},
			})
			require.NoError(t, err)

			// when
			schemaErr := Validate(raw)

			// then
			assert.Equal(t, tc.valid, isValidRefsKey(tc.key), "the Go predicate")
			if tc.valid {
				assert.NoError(t, schemaErr, "the schema")
			} else {
				assert.Error(t, schemaErr, "the schema")
			}
		})
	}
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
