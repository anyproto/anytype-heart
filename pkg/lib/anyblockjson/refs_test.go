package anyblockjson

// refs_test.go — the informative `#name` reference suffix (§9): written on
// export behind RefNames, trimmed unread on import, and never required.

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

// testObjectNames answers from a fixed table — the ObjectNameResolver shape.
type testObjectNames map[string]string

func (m testObjectNames) ObjectName(id string) (string, bool) {
	n, ok := m[id]
	return n, ok
}

// refSnapshot exercises every slot the suffix rides: an object-format
// property, collection items, link/file/bookmark blocks, and a dataview with
// an object-valued filter, a custom order, and a kanban object order.
func refSnapshot() *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{
				Id:          "bafyreirefroot",
				ChildrenIds: []string{"lnk", "fil", "bmk", "dv1"},
				Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
			},
			{Id: "lnk", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
				TargetBlockId: "bafyreilinked",
			}}},
			{Id: "fil", Content: &model.BlockContentOfFile{File: &model.BlockContentFile{
				Type: model.BlockContentFile_Image, TargetObjectId: "bafyreipicture",
			}}},
			{Id: "bmk", Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{
				Url: "https://anytype.io", TargetObjectId: "bafyreibookmarked",
			}}},
			{Id: "dv1", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				TargetObjectId: "bafyreitargeted",
				Views: []*model.BlockContentDataviewView{{
					Id:   "view1",
					Name: "All",
					Filters: []*model.BlockContentDataviewFilter{{
						Id:          "f1",
						RelationKey: "assignee",
						Condition:   model.BlockContentDataviewFilter_In,
						Value:       strList("bafyreifiltered"),
					}},
					Sorts: []*model.BlockContentDataviewSort{{
						Id:          "s1",
						RelationKey: "assignee",
						CustomOrder: []*types.Value{str("bafyreiordered")},
					}},
				}},
				ObjectOrders: []*model.BlockContentDataviewObjectOrder{{
					ViewId:    "view1",
					ObjectIds: []string{"bafyreikanban"},
				}},
			}}},
		},
		Details: fields(map[string]*types.Value{
			"id":       str("bafyreirefroot"),
			"name":     str("Ref host"),
			"related":  strList("bafyreitopic"),
			"assignee": strList("bafyreiassigned"),
		}),
		Collections: fields(map[string]*types.Value{
			storeKeyItems: strList("bafyreicollected"),
		}),
	}
}

// refNames names every referenced object in refSnapshot.
var refNames = testObjectNames{
	"bafyreitopic":      "Local-first UX",
	"bafyreiassigned":   "Roma Kha",
	"bafyreicollected":  "Collected Page",
	"bafyreilinked":     "Linked Page",
	"bafyreipicture":    "Cat Photo",
	"bafyreibookmarked": "Bookmarked Page",
	"bafyreitargeted":   "Task Tracker",
	"bafyreifiltered":   "Filter Target",
	"bafyreiordered":    "Order Target",
	"bafyreikanban":     "Kanban Card",
}

func refOptions() Options {
	o := testOptions()
	o.ResolveFormat = func(key domain.RelationKey) (model.RelationFormat, bool) {
		if key == "related" {
			return model.RelationFormat_object, true
		}
		return testFormatResolver(key)
	}
	return o
}

// Every slot §9 lists gains the suffix when the shape asks for it, and the
// output stays a document this package's own Validate accepts (I1).
//
// How this can fail: unhook exporter.objectRef from any one slot and that
// slot's assertion finds the bare id; break the normalizer and the expected
// spellings differ; emit a suffix Validate rejects and the I1 check fails.
// Nothing here re-implements the suffix — the expectations are literal
// strings.
func TestRefNames_SuffixOnEverySlot(t *testing.T) {
	// given
	opts := refOptions()
	opts.RefNames = true
	opts.ResolveObjectNames = refNames

	// when
	data, err := Marshal(model.SmartBlockType_Page, refSnapshot(), opts)
	require.NoError(t, err)
	require.NoError(t, Validate(data), "Marshal never emits what Validate rejects (§11 I1)")
	doc := string(data)

	// then — one literal expectation per slot
	for slot, want := range map[string]string{
		"property value (custom objects format)":  `"bafyreitopic#local_first_ux"`,
		"property value (bundled objects format)": `"bafyreiassigned#roma_kha"`,
		"items":                    `"bafyreicollected#collected_page"`,
		"link block":               `"object_id": "bafyreilinked#linked_page"`,
		"file block":               `"object_id": "bafyreipicture#cat_photo"`,
		"bookmark block":           `"object_id": "bafyreibookmarked#bookmarked_page"`,
		"dataview target":          `"object_id": "bafyreitargeted#task_tracker"`,
		"filter value":             `"bafyreifiltered#filter_target"`,
		"sort custom order":        `"bafyreiordered#order_target"`,
		"object_orders object ids": `"bafyreikanban#kanban_card"`,
	} {
		assert.Contains(t, doc, want, slot)
	}
}

// The suffix is opt-in per shape: RefNames off (the default, the
// export/backup shape) writes every reference bare even with a resolver
// wired, and RefNames on with no resolver writes them bare too — never a
// partial or invented suffix.
//
// How this can fail: make the suffix unconditional and the first case finds
// a `#`; invent a suffix from the id itself and the second does.
func TestRefNames_OffByDefaultAndBareWithoutResolver(t *testing.T) {
	t.Run("resolver wired, flag off", func(t *testing.T) {
		// given
		opts := refOptions()
		opts.ResolveObjectNames = refNames

		// when
		data, err := Marshal(model.SmartBlockType_Page, refSnapshot(), opts)

		// then
		require.NoError(t, err)
		assert.NotContains(t, string(data), "#",
			"the export/backup shape writes no suffix: minimal, and stable under renames")
	})

	t.Run("flag on, no resolver", func(t *testing.T) {
		// given
		opts := refOptions()
		opts.RefNames = true

		// when
		data, err := Marshal(model.SmartBlockType_Page, refSnapshot(), opts)

		// then
		require.NoError(t, err)
		assert.NotContains(t, string(data), "#", "with no resolver, the bare id — nothing invented")
	})
}

// suffixedRefDoc spells a suffixed reference in every §9 slot; bareRefDoc is
// the same document with every suffix removed.
const suffixedRefDoc = `{
  "version": 1,
  "id": "bafyreirefroot",
  "properties": {
    "assignee": ["bafyreiassigned#roma_kha"],
    "name": "Ref host"
  },
  "blocks": [
    {"type": "link", "object_id": "bafyreilinked#linked_page"},
    {"type": "image", "object_id": "bafyreipicture#cat_photo"},
    {"type": "bookmark", "url": "https://anytype.io", "object_id": "bafyreibookmarked#bookmarked_page"},
    {"type": "dataview", "object_id": "bafyreitargeted#task_tracker", "views": [
      {"id": "view1", "name": "All",
       "filters": [{"property": "assignee", "condition": "in", "value": ["bafyreifiltered#filter_target"]}],
       "sorts": [{"property": "assignee", "custom_order": ["bafyreiordered#order_target"]}],
       "object_orders": [{"object_ids": ["bafyreikanban#kanban_card"]}]}
    ]}
  ],
  "items": ["bafyreicollected#collected_page"]
}`

// Import trims the suffix at the first `#` in every slot, and a bare
// document imports IDENTICALLY — the §11 I2 surface: a model writing a new
// reference has no name to add, and must not need one.
//
// How this can fail: skip the trim at any slot and that slot's snapshot
// value keeps the `#name`; trim at the LAST `#` instead of the first and the
// double-# case keeps half a suffix; make the suffix load-bearing and the
// bare document stops importing equal.
func TestRefs_ImportTrimsAndBareImportsIdentically(t *testing.T) {
	bare := strings.NewReplacer(
		"#roma_kha", "", "#linked_page", "", "#cat_photo", "",
		"#bookmarked_page", "", "#task_tracker", "", "#filter_target", "",
		"#order_target", "", "#kanban_card", "", "#collected_page", "",
	).Replace(suffixedRefDoc)
	require.NotContains(t, bare, "#", "the bare twin really is bare")

	// when — both forms validate and both import
	require.NoError(t, Validate([]byte(suffixedRefDoc)), "a suffixed reference is valid")
	require.NoError(t, Validate([]byte(bare)), "a bare reference is valid")
	importOpts := func() Options {
		o := testOptions()
		o.GenerateId = seqIds("gen") // deterministic, so the two snapshots can be compared whole
		return o
	}
	sbType1, suffixed, err := Unmarshal([]byte(suffixedRefDoc), importOpts())
	require.NoError(t, err)
	sbType2, bareSnap, err := Unmarshal([]byte(bare), importOpts())
	require.NoError(t, err)

	// then — the suffix reached no snapshot slot
	blob, err := json.Marshal(suffixed)
	require.NoError(t, err)
	assert.NotContains(t, string(blob), "#", "no suffix survives into the snapshot")
	for slot, want := range map[string]string{
		"assignee": "bafyreiassigned",
	} {
		assert.Equal(t, []string{want},
			valueStringList(suffixed.GetDetails().GetFields()[slot]), slot)
	}

	// and the two forms import identically
	assert.Equal(t, sbType1, sbType2)
	assert.Equal(t, bareSnap, suffixed, "a bare id and a suffixed id import identically (§11 I2)")
}

// A `#` that does not follow an id is left alone: the trim never invents an
// empty reference out of a malformed one, and an option NAME containing `#`
// (a select value like "C#") is not an object reference and keeps its
// characters.
//
// How this can fail: trim unconditionally at index 0 and the leading-#
// value comes back empty; run the trim over select values and "C#" loses
// its sharp.
func TestRefs_TrimNeverInventsEmptinessAndSkipsOptionNames(t *testing.T) {
	t.Run("a leading-# value stays whole", func(t *testing.T) {
		// given
		doc := `{"version": 1, "properties": {"assignee": ["#notanid"]}}`

		// when
		_, snap, err := Unmarshal([]byte(doc), testOptions())

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"#notanid"},
			valueStringList(snap.GetDetails().GetFields()["assignee"]))
	})

	t.Run("a double-# value trims at the FIRST separator", func(t *testing.T) {
		// given a reference whose informative half itself spells a #
		doc := `{"version": 1, "properties": {"assignee": ["bafyreiassigned#a#b"]}}`

		// when
		_, snap, err := Unmarshal([]byte(doc), testOptions())

		// then — LastIndex would hand back bafyreiassigned#a, an id that
		// addresses nothing
		require.NoError(t, err)
		assert.Equal(t, []string{"bafyreiassigned"},
			valueStringList(snap.GetDetails().GetFields()["assignee"]))
	})

	t.Run("an option name keeps its #", func(t *testing.T) {
		// given customStatus resolves to the select format (testOptions)
		doc := `{"version": 1, "properties": {"customStatus": ["C#"]}}`

		// when
		_, snap, err := Unmarshal([]byte(doc), testOptions())

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"C#"},
			valueStringList(snap.GetDetails().GetFields()["customStatus"]),
			"a select value is a name, not a reference — no trim (§9)")
	})
}

// The round trip stays byte-stable given the same resolver: import trims the
// suffix, and the second export re-derives it from the same names.
//
// How this can fail: any slot that trims without re-deriving (or derives
// without trimming) shifts bytes between generations.
func TestRefs_RoundTripByteStableWithResolver(t *testing.T) {
	// given
	opts := refOptions()
	opts.RefNames = true
	opts.ResolveObjectNames = refNames

	// when
	first, err := Marshal(model.SmartBlockType_Page, refSnapshot(), opts)
	require.NoError(t, err)
	sbType, imported, err := Unmarshal(first, opts)
	require.NoError(t, err)
	second, err := Marshal(sbType, imported, opts)
	require.NoError(t, err)

	// then
	assert.Equal(t, string(first), string(second),
		"Export ∘ Import is byte-stable with the same resolver (§11)")
}

// The ids that already say what they mean take no suffix: a date reference,
// the missing-object sentinel, a dynamic filter placeholder.
//
// How this can fail: drop the suffixableRef guard and each of the three
// gains a suffix the moment a resolver claims to name it.
func TestRefNames_SelfDescribingIdsTakeNoSuffix(t *testing.T) {
	// given a resolver that (wrongly) has names for all three
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{
			Id:      "bafyreirefroot",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}},
		Details: fields(map[string]*types.Value{
			"id":       str("bafyreirefroot"),
			"related":  strList("_date_2026-08-17", "_missing_object", "_filter_template_2_"),
			"assignee": strList("bafyreiassigned"),
		}),
	}
	opts := refOptions()
	opts.RefNames = true
	opts.ResolveObjectNames = testObjectNames{
		"_date_2026-08-17":    "17 Aug 2026",
		"_missing_object":     "Missing",
		"_filter_template_2_": "Current user",
		"bafyreiassigned":     "Roma Kha",
	}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	doc := string(data)

	// then
	assert.Contains(t, doc, `"_date_2026-08-17"`)
	assert.Contains(t, doc, `"_missing_object"`)
	assert.Contains(t, doc, `"_filter_template_2_"`)
	assert.Contains(t, doc, `"bafyreiassigned#roma_kha"`, "the control: an ordinary ref still gains one")
}

// The name half of the split guarantee: whatever a display name holds, the
// suffix that reaches the document contains no `#` and survives in the
// identifier grammar.
//
// How this can fail: write the raw display name after the `#` and the
// adversarial cases split wrong on read; drop the truncation and the long
// name blows the bound.
func TestRefNameLabel(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"spaces snake":    {"Local-first UX", "local_first_ux"},
		"plain name":      {"Roma Kha", "roma_kha"},
		"hash inside":     {"a#b", "a_b"},
		"only a hash":     {"#", ""},
		"whitespace only": {" \t ", ""},
		"non-latin kept":  {"Тоггл", "тоггл"},
		"empty":           {"", ""},
	} {
		t.Run(name, func(t *testing.T) {
			got := refNameLabel(tc.in)
			assert.Equal(t, tc.want, got)
			assert.NotContains(t, got, "#", "the grammar admits no #")
		})
	}

	t.Run("a long name truncates at the bound", func(t *testing.T) {
		long := strings.Repeat("word ", 40) // normalizes to 199 chars of word_word_…
		got := refNameLabel(long)
		assert.LessOrEqual(t, len([]rune(got)), maxRefNameLen)
		assert.NotEmpty(t, got)
		assert.False(t, strings.HasSuffix(got, "_"), "no dangling separator after the cut")
	})
}

// An id that already carries a `#` takes no suffix, however confidently a
// resolver names it: `x#y` + `#name` reads back as `x`, so the caption would
// be paid for with the id itself (§9).
//
// How this can fail: drop the refNameSep arm of suffixableRef and both ids
// below gain a caption the reader cannot undo.
func TestRefNames_AnIdCarryingAHashTakesNoSuffix(t *testing.T) {
	// given a resolver that has a name for both hostile ids
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{
			Id:      "bafyreirefroot",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}},
		Details: fields(map[string]*types.Value{
			"id":       str("bafyreirefroot"),
			"assignee": strList("bafyreiassigned#stale_name"),
			"related":  strList("#notanid"),
		}),
	}
	opts := refOptions()
	opts.RefNames = true
	opts.ResolveObjectNames = testObjectNames{
		"bafyreiassigned#stale_name": "Roma Kha",
		"#notanid":                   "Roma Kha",
	}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	require.NoError(t, Validate(data), "Marshal never emits what Validate rejects (§11 I1)")
	doc := string(data)

	// then — both stand exactly as stored, with nothing appended
	assert.Contains(t, doc, `"bafyreiassigned#stale_name"`)
	assert.Contains(t, doc, `"#notanid"`)
	assert.NotContains(t, doc, "roma_kha", "no caption on an unsplittable id")
}

// A reference with no id half does not grow a name every generation (§11
// guarantee 2). splitRefName refuses to split at index 0, so `#name` imports
// whole; if export were still willing to caption it, each round trip would
// append another and the document would diverge without bound.
//
// How this can fail: let suffixableRef admit a `#`-bearing id and generation
// 2 is `#some_name#roma_kha`, generation 3 one name longer again.
func TestRefs_ALeadingHashReferenceDoesNotGrow(t *testing.T) {
	// given the mistake a writer makes copying the readable half of id#name
	doc := []byte(`{"version": 1, "kind": "page", "id": "bafyreiroot",
		"properties": {"assignee": ["#some_name"]}}`)
	require.NoError(t, Validate(doc))

	opts := refOptions()
	opts.RefNames = true
	opts.ResolveObjectNames = testObjectNames{"#some_name": "Roma Kha"}

	// when — three generations through the codec
	var gens []string
	cur := doc
	for i := 0; i < 3; i++ {
		sbType, snap, err := Unmarshal(cur, opts)
		require.NoError(t, err)
		assert.Equal(t, []string{"#some_name"},
			valueStringList(snap.GetDetails().GetFields()["assignee"]),
			"generation %d reads back the value it was given", i+1)
		cur, err = Marshal(sbType, snap, opts)
		require.NoError(t, err)
		require.NoError(t, Validate(cur))
		gens = append(gens, string(cur))
	}

	// then
	assert.Equal(t, gens[0], gens[1], "Export ∘ Import is byte-stable (§11)")
	assert.Equal(t, gens[1], gens[2])
}

// The format's one reference normalization (§11 N(S)): an id with a `#`
// INSIDE it loses its tail on read, because the split cannot tell that `#`
// from the one the suffix uses. Export no longer captions such an id, so the
// loss happens once and the value is a fixpoint from the second generation
// on — it does not shrink again, and it does not grow.
//
// No id this format writes contains a `#` and none was found in 81,696
// production documents across two corpora; this test exists to state what
// happens if one ever does, rather than to leave it to be discovered.
//
// How this can fail: caption a `#`-bearing id again and generation 2 differs
// from generation 3 as the tail is eaten one segment at a time.
func TestRefs_AHashInsideAnIdIsNormalizedOnce(t *testing.T) {
	// given
	opts := refOptions()
	opts.RefNames = true
	opts.ResolveObjectNames = testObjectNames{
		"bafyreiassigned#weird": "Roma Kha",
		"bafyreiassigned":       "Roma Kha",
	}
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{
			Id:      "bafyreirefroot",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}},
		Details: fields(map[string]*types.Value{
			"id":       str("bafyreirefroot"),
			"assignee": strList("bafyreiassigned#weird"),
		}),
	}

	// when
	gen1, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	sbType, back, err := Unmarshal(gen1, opts)
	require.NoError(t, err)
	gen2, err := Marshal(sbType, back, opts)
	require.NoError(t, err)
	_, back2, err := Unmarshal(gen2, opts)
	require.NoError(t, err)
	gen3, err := Marshal(sbType, back2, opts)
	require.NoError(t, err)

	// then — the tail goes once, and only once
	assert.Contains(t, string(gen1), `"bafyreiassigned#weird"`, "export writes the stored id whole")
	assert.Equal(t, []string{"bafyreiassigned"},
		valueStringList(back.GetDetails().GetFields()["assignee"]),
		"the reader cannot tell this # from the suffix's, so the tail goes (§11 N(S))")
	assert.Equal(t, string(gen2), string(gen3), "and the normalized value is a fixpoint")
}

// A reference with no id half is a warning on the way in, not a silent
// dangling value: the reader will not repair it, so the writer is told (§9).
// Warning-grade, because a document that carries one is still readable, and
// export must be able to pass through whatever a snapshot holds.
//
// How this can fail: drop the objects arm of wrongShapeForFormat and the
// document validates clean while the value addresses nothing.
func TestValidate_AReferenceWithNoIdHalfWarns(t *testing.T) {
	// given assignee is a bundled objects property — no store needed to know it
	doc := []byte(`{"version": 1, "properties": {"assignee": ["#roma_kha"]}}`)

	// when
	var warned []Issue
	err := ValidateWarn(doc, func(i Issue) { warned = append(warned, i) })

	// then
	require.NoError(t, err, "a document that carries one is still readable")
	require.Len(t, warned, 1)
	assert.Equal(t, "/properties/assignee", warned[0].Path)
	assert.Contains(t, warned[0].Message, "no id before its")
}
