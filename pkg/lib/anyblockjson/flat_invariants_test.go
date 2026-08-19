package anyblockjson

// Two invariants hold the format together, and the pre-freeze review found six
// violations of the first and four of the second — every one of them in a place
// the rule had never been applied, though it was written down. Instances get
// their own regression tests in prefreeze_review_test.go; these are the
// invariants themselves, so the next instance fails here without anyone having
// thought of it:
//
//	I1. Marshal never emits a document its own Validate rejects (§11).
//	I2. Validate and Unmarshal agree on every input (§12): if Validate accepts
//	    a document, Unmarshal must not fail to decode it, and vice versa.
//
// Both are driven by hostile inputs on purpose. A corpus generated from
// Marshal's own output cannot catch what Marshal gets wrong — it would agree
// with itself — and the goldens are exactly that corpus. So I1 runs over
// snapshots built from the id shapes real data and real generators produce
// (dots, slashes, non-ASCII, over-long, derived-cell-shaped, suffix-colliding),
// and I2 over hand-written documents.

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// hostileIds are the id shapes that broke the id domain, plus the ones that
// make two surfaces compete: `a.b` and `dir/file` miss the schema's charset,
// `c-1` sanitizes onto `c_1`, `block_12345` suffixes onto `12345` under
// CompactIds, `r1-c1` is what a table derives for its own cell, `c_1-c1` is
// what one derives after a dashed row id has been sanitized, `dataview` is
// the id the importer pins, and the long one exceeds the 64-character bound.
var hostileIds = []string{
	"", "a.b", "a-b", "a_b", "dir/file", "блок", "c-1", "c_1", "c1", "r1",
	"r1-c1", "r1-c2", "c_1-c1", "c_1-c_1", "r1-c_1", "12345", "block_12345",
	"dataview", "-", "_",
	strings.Repeat("x", 70), "R1-C1", "a b", "id\n2", "obj1",
}

// hostileSnapshot builds a deterministic snapshot for seed n: a root, a handful
// of text blocks, and optionally a table and a dataview, with every id drawn
// from hostileIds — including duplicates, which the snapshot graph is allowed
// to contain because it is untrusted (§11).
func hostileSnapshot(n int) (model.SmartBlockType, *model.SmartBlockSnapshotBase) {
	rnd := rand.New(rand.NewSource(int64(n)))
	pick := func() string { return hostileIds[rnd.Intn(len(hostileIds))] }

	root := &model.Block{Id: "obj1",
		Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}
	blocks := []*model.Block{root}
	add := func(b *model.Block) {
		root.ChildrenIds = append(root.ChildrenIds, b.Id)
		blocks = append(blocks, b)
	}

	for i := 0; i < 1+rnd.Intn(4); i++ {
		add(&model.Block{Id: pick(), Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{Text: fmt.Sprintf("text %d <sub>x</sub>", i)}}})
	}
	if rnd.Intn(2) == 0 {
		colIds := []string{pick(), pick()}
		rowIds := []string{pick(), pick()}
		table := &model.Block{Id: pick(),
			Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}}
		cols := &model.Block{Id: "cols" + fmt.Sprint(n),
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_TableColumns}}}
		rows := &model.Block{Id: "rows" + fmt.Sprint(n),
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_TableRows}}}
		table.ChildrenIds = []string{cols.Id, rows.Id}
		add(table)
		blocks = append(blocks, cols, rows)
		for _, id := range colIds {
			cols.ChildrenIds = append(cols.ChildrenIds, id)
			blocks = append(blocks, &model.Block{Id: id,
				Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}})
		}
		var unwritten []string
		for _, rowId := range rowIds {
			rows.ChildrenIds = append(rows.ChildrenIds, rowId)
			row := &model.Block{Id: rowId,
				Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}}
			blocks = append(blocks, row)
			for _, colId := range colIds {
				cellId := rowId + "-" + colId
				// a grid cell nobody has filled: no block exists for it, but
				// the id is the table's all the same (§6.1) — the editor
				// materializes the cell at exactly that id the first time it
				// is filled, and validation claims the whole grid
				if rnd.Intn(3) == 0 {
					unwritten = append(unwritten, cellId)
					continue
				}
				row.ChildrenIds = append(row.ChildrenIds, cellId)
				blocks = append(blocks, &model.Block{Id: cellId,
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{Text: "cell " + cellId}}})
			}
		}
		// a plain block sitting on a derived cell id. While every cell was
		// materialized the id was reserved by the cell block itself, so this
		// slot — the one collision the emit side never made — had no coverage.
		if len(unwritten) > 0 {
			add(&model.Block{Id: unwritten[rnd.Intn(len(unwritten))],
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{Text: "sits on a cell id"}}})
		}
	}
	if rnd.Intn(3) == 0 {
		// a date filter carrying a preset but no value: the value-bearing
		// conditions and the presence-only ones make export write different
		// things, and only one of those had coverage
		cond := []model.BlockContentDataviewFilterCondition{
			model.BlockContentDataviewFilter_Empty,
			model.BlockContentDataviewFilter_NotEmpty,
			model.BlockContentDataviewFilter_Exists,
			model.BlockContentDataviewFilter_Greater,
			model.BlockContentDataviewFilter_Equal,
			model.BlockContentDataviewFilter_NotEqual,
		}[rnd.Intn(6)]
		preset := []model.BlockContentDataviewFilterQuickOption{
			model.BlockContentDataviewFilter_NumberOfDaysAgo,
			model.BlockContentDataviewFilter_NumberOfDaysNow,
			model.BlockContentDataviewFilter_Today,
		}[rnd.Intn(3)]
		add(&model.Block{Id: pick(), Content: &model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{{Id: pick(), Name: "All",
					Filters: []*model.BlockContentDataviewFilter{{
						RelationKey: "dueDate", Condition: cond, QuickOption: preset,
						Format: model.RelationFormat_date,
					}}}},
			}}})
	}
	// details carry the keys the import surface must refuse: if export ever
	// emitted one, Marshal's output would fail its own Validate, which is how
	// this invariant proves the two surfaces are still each other's mirror.
	// The two custom keys at the end exist to give a vocabulary something to
	// slug: the hostileVocab variant maps them onto the slug shapes a real
	// space can mint (over-long, empty, shadowing a bundled spelling).
	details := map[string]*types.Value{
		"id":                       str("obj1"),
		"name":                     str("hostile"),
		"spaceId":                  str("bafyspace"),
		"uniqueKey":                str("ot-page"),
		"oldAnytypeID":             str("legacy1"),
		"sourceFilePath":           str("/tmp/x.md"),
		"restrictions":             {Kind: &types.Value_NumberValue{NumberValue: 3}},
		"isArchived":               {Kind: &types.Value_BoolValue{BoolValue: true}},
		"":                         str("empty key"),
		"a\nb":                     str("newline key"),
		"dueDate":                  str("next Friday"),
		"6a32d4856761631534b22f85": str("space-slugged"),
		"artist":                   str("verbatim custom key"),
		// the two shadow shapes of the verbatim-first family (§3): a custom
		// stored key spelling an INTERNAL bundled key's slug, and one
		// spelling a WRITABLE bundled key's slug beside that bundled key
		// itself ("dueDate" above). Both are slug-shaped stored keys the
		// details carried none of, which is exactly how "export writes it
		// verbatim, the reader resolves it elsewhere" stayed invisible: the
		// first made seed 0 fail I1's Validate leg outright, the second made
		// every seed a valid-but-unimportable archive until the identity
		// entry existed.
		"unique_key": str("custom, not the resolution vector"),
		"due_date":   str("custom, beside dueDate"),
	}
	// the envelope key is a STORED identity key written verbatim (§2), and a
	// closed charset over it was falsified by a 36 808-object sweep: relation
	// options carry their option *name* in the key, spaces and all. I1 never
	// covered this slot, which is exactly why the bad rule shipped.
	storedKeys := []string{
		"", "page", "task", "completion_status_Not Started",
		"69bbfc78877a91b1d12d1a7c_C/C++", "69a56205ccba0a47d8d8eb71_тогглы",
		"69bbfc78877a91b1d12d1a84_$addToSet", "opt-" + strings.Repeat("x", 80),
	}
	snap := &model.SmartBlockSnapshotBase{
		Blocks:  blocks,
		Details: fields(details),
		Key:     storedKeys[rnd.Intn(len(storedKeys))],
	}
	// the TYPE key slots (§3): the envelope `type`/`template_for` pair and —
	// on type-document seeds — `type_properties[].object_types`. Drawn after
	// every other pick so adding them did not reshuffle the corpus above.
	snap.ObjectTypes = hostileTypePools[rnd.Intn(len(hostileTypePools))]
	sbType := model.SmartBlockType_Page
	if rnd.Intn(4) == 0 {
		// a type document: the recommended lists resolve through
		// hostileTypePropResolver, whose definitions carry the object_types
		// shapes (custom key the vocabulary slugs onto `task`, an unwritable
		// slug, the reserved `template` spelling)
		sbType = model.SmartBlockType_STType
		snap.Details.Fields["recommendedFeaturedRelations"] = strList("hp1")
		snap.Details.Fields["recommendedRelations"] = strList("hp2")
	}
	return sbType, snap
}

// hostileTypePools are the ObjectTypes shapes that make the type namespace
// compete with its readers: a custom key a hostile vocabulary slugs onto the
// bundled `task` key, a stored key spelling the bundled objectType's slug
// (the §3 shadow shape — it owes an identity entry), template pairs whose
// second entry only survives if the `template` spelling stays put, keys whose
// vocabulary slug is unwritable or reserved, a prefix-less entry, and an
// entry with no key at all, and — the collateral-damage shapes — a keyless
// entry STANDING BESIDE a good one. Stored `ot-` has no spelling, so a
// positional write emitted no `type`, which made `template_for` inexpressible
// too and took the good sibling down with it; a store that ran an older build
// holds exactly these.
var hostileTypePools = [][]string{
	nil,
	{"ot-page"},
	{"ot-69bbfc78877a91b1d12d1a7c"},
	{"ot-object_type"},
	{"ot-template", "ot-69bbfc78877a91b1d12d1a7c"},
	{"ot-template", "ot-task"},
	{"ot-" + strings.Repeat("y", 140)},
	{"ot-longslugged"},
	{"ot-blankslugged"},
	{"ot-squatter"},
	{"page"},
	{"ot-"},
	{"ot-", "ot-task"},
	{"", "ot-69bbfc78877a91b1d12d1a7c"},
	{"ot-template", "ot-"},
	{"ot-template", "ot-", "ot-task"},
	{"ot-", "ot-template", "ot-task"},
}

// hostileTypePropResolver serves the two property definitions the type-seed
// recommended lists name. PropertyId answers false so import-side wiring is
// exercised without it.
type hostileTypePropResolver struct{}

func (hostileTypePropResolver) PropertyById(id string) (PropertyDefinition, bool) {
	switch id {
	case "hp1":
		return PropertyDefinition{Key: "owner", Name: "Owner", Format: model.RelationFormat_object,
			ObjectTypes: []string{"69bbfc78877a91b1d12d1a7c", "task", "squatter"}}, true
	case "hp2":
		return PropertyDefinition{Key: "genre", Format: model.RelationFormat_object,
			ObjectTypes: []string{"longslugged", strings.Repeat("y", 140)}}, true
	}
	return PropertyDefinition{}, false
}

func (hostileTypePropResolver) PropertyId(PropertyDefinition) (string, bool) { return "", false }

// invertedTypes is what a faithful reader owes back for a snapshot's
// ObjectTypes: the format writes object_types[0] as `type` — and [1] as
// `template_for` when [0] is the template key — each spelled through the
// vocabulary in force and inverted through the document's own legend, so the
// stored KEYS must survive whatever the vocabulary did to their spellings.
// Entries normalize to the `ot-` URL form; a non-template's types past the
// first are not modeled by the format (§2).
//
// An entry with no key ("ot-", "") has no spelling and is dropped — and the
// survivors CLOSE RANKS, which is the load-bearing half. Dropping in place
// made a keyless entry contagious: it silenced the slot it sat in, and a
// silent `type` slot makes `template_for` inexpressible, so a template stored
// as ["ot-", "ot-task"] came back as no types at all rather than as ot-task.
func invertedTypes(objectTypes []string) []string {
	keys := make([]string, 0, len(objectTypes))
	for _, t := range objectTypes {
		if key := strings.TrimPrefix(t, "ot-"); key != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	out := []string{"ot-" + keys[0]}
	if keys[0] == "template" && len(keys) > 1 {
		out = append(out, "ot-"+keys[1])
	}
	return out
}

// hostileVocab deliberately breaks the KeyVocabulary contract the way a real
// space can: a slug comes from apiObjectKey, which is user-supplied or
// strcase-derived from the property NAME (objectcreator/util.go) with no
// length bound and no charset audit — so nothing upstream guarantees a slug
// is a writable spelling, and a space may mint a slug that shadows a bundled
// one. This slot had no invariant coverage, which is exactly how "check the
// stored key, emit the slug" shipped.
type hostileVocab struct{ BundledKeyVocabulary }

func (hostileVocab) PropertySlug(key string) string {
	switch key {
	case "name":
		return strings.Repeat("s", maxPropertyKeyLen+64) // apiObjectKey has no length bound
	case "dueDate":
		return "due\ndate" // a control character is not a spelling
	case "artist":
		return "" // a vocabulary with no answer at all
	case "6a32d4856761631534b22f85":
		// a space-minted slug shadowing a bundled internal key's spelling —
		// which is ALSO a stored key on the hostile details, so the term
		// ledger must refuse the claim outright (§3: a stored key always
		// keeps its own term, and this one owes an identity entry). The
		// corpus's legend-rebind documents cover the honored-entry case.
		return "unique_key"
	}
	return BundledKeyVocabulary{}.PropertySlug(key)
}

// TypeSlug breaks the contract the same ways the property half does, plus
// the two shapes only the type namespace has: a slug that collides with a
// bundled TYPE key (the confirmed defect — `69bbfc…` spelled `task` read
// back as the bundled Task type by a package-only reader), and answers that
// move the reserved `template` spelling in either direction, which silently
// drops a template's target type.
func (hostileVocab) TypeSlug(key string) string {
	switch key {
	case "69bbfc78877a91b1d12d1a7c":
		return "task"
	case "longslugged":
		return strings.Repeat("t", maxPropertyKeyLen+64)
	case "blankslugged":
		return ""
	case "squatter":
		return "template"
	case "template":
		return "tmpl"
	}
	return BundledKeyVocabulary{}.TypeSlug(key)
}

// I1: Marshal either fails loudly — §11 allows that for an over-deep tree or a
// table inside a cell — or produces a document its own Validate accepts AND
// its own Unmarshal imports. What it may never do is succeed and hand back an
// unimportable archive, which is a failure nobody sees until the archive is
// needed. The Unmarshal leg runs as a package-only reader, because that is
// what an archive's consumer is: it checks that the document plus its legend
// resolve without the vocabulary that wrote them — this leg, gated on
// Validate alone for a while, is where a valid-but-unimportable document (a
// shadow twin pair; a backed-off slug a block slot recorded anyway) hid.
func TestInvariant_MarshalOutputValidates(t *testing.T) {
	variants := map[string]Options{
		"plain":        {},
		"compact":      {CompactIds: true},
		"omitIds":      {OmitIds: true},
		"hostileVocab": {Keys: hostileVocab{}},
	}
	for name, opts := range variants {
		t.Run(name, func(t *testing.T) {
			for n := 0; n < 300; n++ {
				sbType, snap := hostileSnapshot(n)
				o := opts
				if sbType == model.SmartBlockType_STType {
					o.ResolveProperties = hostileTypePropResolver{}
				}
				data, err := Marshal(sbType, snap, o)
				if err != nil {
					continue // a loud failure is allowed; silent invalidity is not
				}
				require.NoError(t, Validate(data), "seed %d produced:\n%s", n, data)
				_, back, err := Unmarshal(data, Options{GenerateId: seqIds(fmt.Sprintf("g%d_", n))})
				require.NoError(t, err,
					"seed %d produced a valid document its own Unmarshal refuses:\n%s", n, data)
				// the type slots must INVERT, not merely import: binding the
				// spelling to a different stored type is exactly the silent
				// failure the type_keys legend exists to close (§3), and no
				// error marks it
				assert.Equal(t, invertedTypes(snap.ObjectTypes), back.ObjectTypes,
					"seed %d: the archive must bind back to the types it came from:\n%s", n, data)
			}
		})
	}
}

// The same invariant on the goldens' own fixture, which is the case the
// existing corpus covers — kept so a regression there is not mistaken for a
// hostile-input-only problem.
func TestInvariant_MarshalOutputValidates_RichFixture(t *testing.T) {
	for name, opts := range map[string]Options{
		"plain":   testOptions(),
		"compact": {CompactIds: true, ResolveFormat: testFormatResolver, ResolveOptions: testResolver},
		"omitIds": {OmitIds: true, ResolveFormat: testFormatResolver, ResolveOptions: testResolver},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := Marshal(model.SmartBlockType_Page, richSnapshot(), opts)
			require.NoError(t, err)
			require.NoError(t, Validate(data))
		})
	}
}

// hostileDocs are hand-written documents aimed at the seam between the schema's
// idea of a value and Go's: the schema says "integer", the decoder says int64,
// and JSON Schema counts 2048.0 and 1e1 as integers. Every one of these is a
// document a generator can plausibly emit.
var hostileDocs = []string{
	`{"version": 1}`,
	`{"version": 1.0}`,
	`{"version": 1e0}`,
	`{"version": 1.5}`,
	`{"version": 2}`,
	`{"version": 0}`,
	`{"version": 1, "blocks": [{"type": "file", "size": 2048}]}`,
	`{"version": 1, "blocks": [{"type": "file", "size": 2048.0}]}`,
	`{"version": 1, "blocks": [{"type": "file", "size": 1e3}]}`,
	`{"version": 1, "blocks": [{"type": "file", "size": 1e30}]}`,
	`{"version": 1, "blocks": [{"type": "file", "size": -1}]}`,
	`{"version": 1, "blocks": [{"type": "file", "size": 2048.5}]}`,
	`{"version": 1, "blocks": [{"type": "widget", "limit": 10}]}`,
	`{"version": 1, "blocks": [{"type": "widget", "limit": 1e1}]}`,
	`{"version": 1, "blocks": [{"type": "widget", "limit": 1e20}]}`,
	`{"version": 1, "blocks": [{"type": "widget", "limit": -3}]}`,
	`{"version": 1, "blocks": [{"type": "dataview", "views": [{"id": "v1", "page_size": 50.0}]}]}`,
	`{"version": 1, "blocks": [{"type": "dataview", "views": [{"id": "v1", "page_size": 1e19}]}]}`,
	`{"version": 1, "blocks": [{"type": "dataview", "views": [{"id": "v1", "page_size": 0}]}]}`,
	`{"version": 1, "blocks": [{"type": "table", "columns": [{"id": "c1", "width": 120.7}], "rows": []}]}`,
	`{"version": 1, "blocks": [{"type": "table", "columns": [{"id": "c1", "width": 1e30}], "rows": []}]}`,
	`{"version": 1, "blocks": [{"type": "table", "columns": [{"id": "c1", "width": -5}], "rows": []}]}`,
	`{"version": 1, "blocks": [{"indent": 0.0, "type": "paragraph", "text": "x"}]}`,
	`{"version": 1, "blocks": [{"indent": 1e1, "type": "paragraph", "text": "x"}]}`,
	`{"version": 1, "properties": {"name": "x", "size": 9007199254740993}}`,
	`{"version": 1, "blocks": [{"type": "paragraph", "text": "<sub>x</sub>"}]}`,
	// a JSON number larger than float64 can hold. The loose surfaces have no
	// schema bound to catch it by construction (§3 accepts any number), and the
	// snapshot they decode into is a proto Struct, whose numbers are float64 —
	// so there is nowhere to put such a value, and the answer has to be a
	// path-addressed rejection rather than a decode error
	`{"version": 1, "properties": {"num": 1e400}}`,
	`{"version": 1, "properties": {"num": 1e309}}`,
	`{"version": 1, "properties": {"num": 1e308}}`,
	`{"version": 1, "store": {"k": 1e400}}`,
	`{"version": 1, "blocks": [{"type": "paragraph", "text": "x", "fields": {"w": 1e400}}]}`,
	`{"version": 1, "blocks": [{"type": "table", "columns": [{"id": "c1", "width": 1e400}], "rows": []}]}`,
	`{"version": 1, "blocks": [{"type": "dataview", "views": [{"id": "v1",
		"filters": [{"property": "p", "condition": "equal", "value": 1e400}]}]}]}`,
	`{"version": 1, "blocks": [{"type": "table", "columns": [{"id": "c1"}],
		"rows": [{"id": "r1", "cells": [["nested", {"indent": 1, "type": "paragraph", "text": "y"}]]}]}]}`,
	// admission runs on the RESOLVED stored key (§3): the canonical slug
	// spelling of a denied key, a property_keys legend rebinding a harmless
	// spelling onto one, and the layout-name check behind the same resolution.
	// These pin WHERE the rule lives as much as that it exists — a "fix" that
	// moves the deny rule into import alone makes Validate accept what
	// Unmarshal rejects, and this corpus is what catches that.
	`{"version": 1, "properties": {"unique_key": "ot-page"}}`,
	`{"version": 1, "properties": {"space_id": "other"}}`,
	`{"version": 1, "properties": {"old_anytype_id": "legacy-1"}}`,
	`{"version": 1, "properties": {"source_file_path": "/x/y"}}`,
	`{"version": 1, "properties": {"resolved_layout": "nonsense"}}`,
	`{"version": 1, "properties": {"resolved_layout": "todo"}}`,
	`{"version": 1, "property_keys": {"prio": "uniqueKey"}, "properties": {"prio": "ot-page"}}`,
	`{"version": 1, "property_keys": {"myid": "id"}, "properties": {"myid": "boom"}}`,
	`{"version": 1, "property_keys": {"s": "spaceId"}, "properties": {"s": "other"}}`,
	// a benign rebind is the legend working as specified, and flows through
	`{"version": 1, "property_keys": {"prio": "6a32d4856761631534b22f85"}, "properties": {"prio": "high"}}`,
	// a counting date preset with no count: an error where the preset's day
	// range is applied, and nothing at all where it is inert (§6.2)
	`{"version": 1, "blocks": [{"type": "dataview", "views": [{"id": "v1",
		"filters": [{"property": "due_date", "condition": "empty", "date_preset": "number_of_days_ago"}]}]}]}`,
	`{"version": 1, "blocks": [{"type": "dataview", "views": [{"id": "v1",
		"filters": [{"property": "due_date", "condition": "greater", "date_preset": "number_of_days_ago"}]}]}]}`,
	// a legend value is a stored key and obeys the writable-key rule (§3)
	`{"version": 1, "property_keys": {"p": ""}}`,
	`{"version": 1, "property_keys": {"p": "a\nb"}}`,
	`{"version": 1, "property_keys": {"p": "` + strings.Repeat("k", 129) + `"}}`,
	// the verbatim-first family (§3): twin spellings binding one stored key
	// are refused by BOTH halves with default Options; an identity entry
	// makes a shadow spelling a stored key in every reader; a legend VALUE is
	// admitted like the stored key it is, member or no member spelling it
	`{"version": 1, "properties": {"iconEmoji": "a", "icon_emoji": "b"}}`,
	`{"version": 1, "properties": {"dueDate": "2025-01-01T00:00:00Z", "due_date": "x"}}`,
	`{"version": 1, "property_keys": {"unique_key": "unique_key"}, "properties": {"unique_key": "custom"}}`,
	`{"version": 1, "property_keys": {"unique_key": "6a32d4856761631534b22f85"}, "properties": {"unique_key": "high"}}`,
	`{"version": 1, "property_keys": {"sneaky": "uniqueKey"}}`,
	`{"version": 1, "property_keys": {"p": "oldAnytypeID"}}`,
	// two spellings the document's own chain accepts that a WIDER vocabulary
	// resolves onto a denied / an unwritable key — the i2Vocabularies entries
	// that widen resolution exercise the §3 seam through these
	`{"version": 1, "properties": {"prio": "bare"}}`,
	`{"version": 1, "properties": {"blank": "x"}}`,
	// the TYPE namespace mirrors the legend rules (§3): a benign rebind and
	// an identity entry flow through, a legend value obeys the writable-key
	// rule, the template gate runs on the RESOLVED type key, and two
	// spellings a wider vocabulary resolves further than the document's own
	// chain (the type-axis i2Vocabularies entries widen through these)
	`{"version": 1, "type": "task"}`,
	`{"version": 1, "type": "tsk"}`,
	`{"version": 1, "type": "blanktype"}`,
	`{"version": 1, "type": "template", "template_for": "blanktype"}`,
	`{"version": 1, "type_keys": {"task": "69bbfc78877a91b1d12d1a7c"}, "type": "task"}`,
	`{"version": 1, "type_keys": {"object_type": "object_type"}, "type": "object_type"}`,
	`{"version": 1, "type_keys": {"t": ""}}`,
	`{"version": 1, "type_keys": {"t": "a\nb"}}`,
	`{"version": 1, "type_keys": {"t": "` + strings.Repeat("k", 129) + `"}}`,
	`{"version": 1, "type_keys": {"template": "custom1"}, "type": "template", "template_for": "page"}`,
	`{"version": 1, "type_keys": {"tpl": "template"}, "type": "tpl", "template_for": "page"}`,
	`{"version": 1, "kind": "object_type", "id": "t1", "key": "k",
		"type_keys": {"task": "69bbfc78877a91b1d12d1a7c"},
		"type_properties": [{"key": "owner", "format": "objects", "object_types": ["task", "blanktype"]}]}`,
}

// i2Vocabularies is the Options axis I2 runs over. A vocabulary can resolve
// spellings the document's own chain (legend → bundled table → verbatim)
// cannot, and §3 licenses import to refuse MORE than Validate then: admission
// re-runs at the details seam on the wider resolved key, which Validate —
// deliberately vocabulary-less (§13) — never sees. For those configurations
// the invariant is containment plus path-addressed refusals; where nothing
// widens resolution, it is exact agreement.
var i2Vocabularies = map[string]struct {
	keys   KeyVocabulary
	widens bool
}{
	"default": {nil, false},
	"bundled": {BundledKeyVocabulary{}, false},
	// a symmetric node-backed vocabulary: both directions agree
	"space": {spaceVocabulary{slugOf: map[string]string{"6a32d4856761631534b22f85": "priority"}}, true},
	// PropertySlug and PropertyKey are NOT inverses — the accept side answers
	// for a slug the emit side never writes, the way a stale or hand-rolled
	// vocabulary really breaks; the target is an ordinary custom key, so only
	// the binding moves, never admission
	"asymmetric": {asymmetricVocab{}, true},
	// the two resolutions the seam exists to refuse
	"resolves-denied":     {rebindingVocabulary{}, true},
	"resolves-unwritable": {blankKeyVocab{}, true},
	// the TYPE axis of the same matrix: a symmetric node-backed vocabulary,
	// an asymmetric one whose accept side answers for a slug the emit side
	// never writes, and the unwritable resolution the type seam refuses
	"type-space":               {typedSpaceVocabulary{typeSlugOf: map[string]string{customTypeKey: "task2"}}, true},
	"type-asymmetric":          {asymmetricTypeVocab{}, true},
	"type-resolves-unwritable": {blankTypeVocab{}, true},
}

type asymmetricTypeVocab struct{ BundledKeyVocabulary }

func (asymmetricTypeVocab) TypeKey(slug string) (string, bool) {
	if slug == "tsk" {
		return "69bbfc78877a91b1d12d1a7c", true
	}
	return BundledKeyVocabulary{}.TypeKey(slug)
}

type asymmetricVocab struct{ BundledKeyVocabulary }

func (asymmetricVocab) PropertyKey(slug string) (string, bool) {
	if slug == "prio" {
		return "6a32d4856761631534b22f85", true
	}
	return BundledKeyVocabulary{}.PropertyKey(slug)
}

// I2: whatever Validate accepts, Unmarshal must decode, and whatever Validate
// rejects, Unmarshal must reject too — exactly, for every Options
// configuration that does not widen resolution, and as containment (Unmarshal
// accepts a subset, refusing only through path-addressed admission) for the
// vocabularies that do. A disagreement means the guarantee Validate offers —
// "this document imports" — is not true, and the failure arrives as a bare Go
// decode error with no JSON pointer, outside the path-addressed error
// contract §13 promises.
func TestInvariant_ValidateAndUnmarshalAgree(t *testing.T) {
	for vocabName, vocab := range i2Vocabularies {
		t.Run(vocabName, func(t *testing.T) {
			for i, doc := range hostileDocs {
				t.Run(doc[:min(len(doc), 60)], func(t *testing.T) {
					valErr := Validate([]byte(doc))
					_, _, unmErr := Unmarshal([]byte(doc),
						Options{GenerateId: seqIds(fmt.Sprintf("g%d_", i)), Keys: vocab.keys})
					switch {
					case valErr != nil:
						assert.Error(t, unmErr,
							"Validate rejects this document, so Unmarshal must too: %v", valErr)
					case !vocab.widens:
						assert.NoError(t, unmErr,
							"Validate accepts and nothing widens resolution, so Unmarshal must accept")
					case unmErr != nil:
						var ve *ValidationError
						assert.ErrorAs(t, unmErr, &ve,
							"a wider vocabulary may refuse more, but only through path-addressed admission")
					}
					if unmErr != nil {
						// every rejection is path-addressed — never a raw
						// decode error escaping from the Go layer. Checked
						// whenever Unmarshal fails: the old clause ran only
						// under valErr != nil, where Unmarshal returns
						// validateToDoc's error unchanged — so it could never
						// fire — and nil-panicked when Unmarshal wrongly
						// accepted what Validate refused.
						assert.NotContains(t, unmErr.Error(), "decode document",
							"the reason must come from validation, not from json.Unmarshal")
					}
				})
			}
		})
	}
}

// Whatever Unmarshal accepts must re-export to something Validate accepts too:
// this is I1 with the input side as the generator, which is how an agent's
// document actually travels.
func TestInvariant_ImportedDocumentReExportsValid(t *testing.T) {
	for _, doc := range hostileDocs {
		if Validate([]byte(doc)) != nil {
			continue
		}
		sbType, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err, doc)
		out, err := Marshal(sbType, snap, Options{})
		require.NoError(t, err, doc)
		require.NoError(t, Validate(out), "%s re-exported as:\n%s", doc, out)

		// and the canonical form is byte-stable through another round (§11.2)
		_, snap2, err := Unmarshal(out, Options{GenerateId: seqIds("h")})
		require.NoError(t, err, doc)
		again, err := Marshal(sbType, snap2, Options{})
		require.NoError(t, err, doc)
		assert.Equal(t, string(out), string(again), "re-export must be byte-stable for %s", doc)
	}
}

// A document's ids must survive a round trip unchanged when they are already
// valid: sanitizing is for ids that need it, and renaming one that does not
// would break the "provided ids are preserved so re-exports diff cleanly"
// promise (§9).
func TestExport_ValidIdsAreNeverRenamed(t *testing.T) {
	doc := `{"version": 1, "id": "obj1", "blocks": [
		{"type": "paragraph", "id": "a_b", "text": "first"},
		{"type": "paragraph", "id": "keep-me", "text": "second"},
		{"type": "table", "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": ["x"]}]}]}`
	sbType, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	out, err := Marshal(sbType, snap, Options{})
	require.NoError(t, err)

	var got struct {
		Blocks []struct {
			Id      string `json:"id"`
			Columns []struct {
				Id string `json:"id"`
			} `json:"columns"`
			Rows []struct {
				Id string `json:"id"`
			} `json:"rows"`
		} `json:"blocks"`
	}
	require.NoError(t, json.Unmarshal(out, &got))
	require.Len(t, got.Blocks, 3)
	assert.Equal(t, "a_b", got.Blocks[0].Id)
	assert.Equal(t, "keep-me", got.Blocks[1].Id)
	assert.Equal(t, "c1", got.Blocks[2].Columns[0].Id)
	assert.Equal(t, "r1", got.Blocks[2].Rows[0].Id)
}

// I3: every identifier the format defines is snake_case (§1 Naming). The rule
// is worth a test rather than a review habit — the vocabulary grows one enum
// value at a time, and a camelCase addition would be invisible until a
// generating model tripped over it, which is the failure the rename fixed.
//
// It covers the Go name tables as well as the schema: some vocabulary (the
// layout names) exists only in Go, which is exactly where a stray name hides.
func TestInvariant_VocabularyIsSnakeCase(t *testing.T) {
	// platform identifiers are quoted, not translated (§1 Naming)
	platform := map[string]bool{"allObjects": true, "recentOpen": true}
	snake := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

	check := func(t *testing.T, where, ident string) {
		t.Helper()
		if platform[ident] || strings.HasPrefix(ident, "$") {
			return
		}
		assert.Regexp(t, snake, ident, "%s: %q is not snake_case", where, ident)
	}

	for _, schema := range [][]byte{schemaJSON, indexSchemaJSON} {
		var doc any
		require.NoError(t, json.Unmarshal(schema, &doc))
		var walk func(node any)
		walk = func(node any) {
			switch n := node.(type) {
			case map[string]any:
				for key, v := range n {
					switch key {
					case "properties":
						if props, ok := v.(map[string]any); ok {
							for name, sub := range props {
								check(t, "schema property", name)
								walk(sub)
							}
							continue
						}
					case "enum":
						if list, ok := v.([]any); ok {
							for _, e := range list {
								if s, isStr := e.(string); isStr {
									check(t, "schema enum", s)
								}
							}
							continue
						}
					}
					walk(v)
				}
			case []any:
				for _, v := range n {
					walk(v)
				}
			}
		}
		walk(doc)
	}

	for name, values := range map[string][]string{
		"kind":            namesOf(kindNames.toName),
		"textStyle":       namesOf(textStyleNames.toName),
		"fileType":        namesOf(fileTypeNames.toName),
		"processor":       namesOf(processorNames.toName),
		"widgetLayout":    namesOf(widgetLayoutNames.toName),
		"viewType":        namesOf(viewTypeNames.toName),
		"condition":       namesOf(conditionNames.toName),
		"date_preset":     namesOf(datePresetNames.toName),
		"aggregation":     namesOf(aggregationNames.toName),
		"format":          namesOf(formatNames.toName),
		"layout":          namesOf(layoutNames.toName),
		"card_style":      namesOf(cardStyleNames.toName),
		"card_size":       namesOf(cardSizeNames.toName),
		"list_size":       namesOf(listSizeNames.toName),
		"empty_placement": namesOf(emptyPlacementNames.toName),
	} {
		for _, v := range values {
			check(t, name+" name table", v)
		}
	}
}

func namesOf[T comparable](m map[T]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
