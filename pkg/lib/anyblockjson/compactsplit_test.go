package anyblockjson

// compactsplit_test.go covers what id compaction is after v0.20: ONE half.
// Object-ref compaction and its `refs` legend are deleted (§9a), so the only
// compaction left is CompactBlockLabels — doc-local relabeling that carries
// no legend — and CompactIds is its alias.

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func compactSplitSnapshot() *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Details: fields(map[string]*types.Value{
			"id":   str("bafyreiselfobjectidxxxxxxx"),
			"name": str("Doc"),
		}),
		Blocks: []*model.Block{
			{Id: "bafyreiselfobjectidxxxxxxx", ChildrenIds: []string{"64b2c1d2e3f4a5b6c7d8e9f0", "featuredRelations"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			textBlock("64b2c1d2e3f4a5b6c7d8e9f0", model.BlockContentText_Paragraph, "ping Roman",
				mark(mMention, 5, 10, "bafyreimentiontargetidxxx")),
			// a real editor id that is NOT minted-shaped: the old charset rule
			// relabeled it ("tions"), the minted rule serves it verbatim — the
			// discriminating id the labels-only test pins the rule with
			textBlock("featuredRelations", model.BlockContentText_Paragraph, "meaningful id"),
		},
	}
}

type parsedCompactDoc struct {
	Blocks []struct {
		Id string `json:"id"`
	} `json:"blocks"`
}

func marshalCompactSplit(t *testing.T, opts Options) (parsedCompactDoc, string) {
	t.Helper()
	data, err := Marshal(model.SmartBlockType_Page, compactSplitSnapshot(), opts)
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	var doc parsedCompactDoc
	require.NoError(t, json.Unmarshal(data, &doc))
	return doc, string(data)
}

// Every shape leaves object references full and inline. Stated over the
// compaction flag AND over the default, because "object ids compact" was the
// documented behaviour of exactly one of those and this is what replaced it.
func TestExport_ObjectRefsAreNeverCompacted(t *testing.T) {
	for name, opts := range map[string]Options{
		"default":            {},
		"CompactBlockLabels": {CompactBlockLabels: true},
		"CompactIds":         {CompactIds: true},
		"OmitIds":            {OmitIds: true},
	} {
		t.Run(name, func(t *testing.T) {
			// given / when
			_, s := marshalCompactSplit(t, opts)

			// then: the mention target is spelled in full where it is used,
			// and there is no legend anywhere to look it up in
			assert.Contains(t, s, `object_id=\"bafyreimentiontargetidxxx\"`,
				"the mention target must be written in full")
			assert.NotContains(t, s, `"idxxx"`, "no short label for it may appear")
		})
	}
}

func TestExport_CompactBlockLabelsOnly(t *testing.T) {
	// given / when
	doc, s := marshalCompactSplit(t, Options{CompactBlockLabels: true})

	// then: ONLY the minted id relabels — a meaningful editor id serves
	// verbatim (the old charset rule relabeled "featuredRelations" to
	// "tions"; without this id the assertion could not fail against either
	// rule, both relabel a 24-hex id)
	require.Len(t, doc.Blocks, 2)
	assert.Equal(t, "8e9f0", doc.Blocks[0].Id)
	assert.Equal(t, "featuredRelations", doc.Blocks[1].Id)
	assert.Contains(t, s, "bafyreimentiontargetidxxx",
		"the mention target must survive uncompacted in the document body")
}

func TestExport_CompactIdsIsAnAliasForBlockLabels(t *testing.T) {
	// given / when
	viaAlias, aliasBytes := marshalCompactSplit(t, Options{CompactIds: true})
	viaFlag, flagBytes := marshalCompactSplit(t, Options{CompactBlockLabels: true})

	// then — byte-identical, which is the whole content of "alias"
	assert.Equal(t, flagBytes, aliasBytes)
	assert.Equal(t, viaFlag, viaAlias)
	require.Len(t, viaAlias.Blocks, 2)
	assert.Equal(t, "8e9f0", viaAlias.Blocks[0].Id, "minted-only relabeling holds under CompactIds too")
}

// TestExport_MintedShapeRelabeling pins the relabel rule: only machine-
// minted opaque ids (24-hex bson/API mints, RFC-4122 view UUIDs) relabel;
// anything that could carry meaning — structural constants like "dataview",
// readable seeded/imported ids, short hand-authored ids — keeps its full
// spelling and is reserved so no label can alias it. The fixture carries
// the three id populations the review asked for (minted, hyphenated-tail,
// 5-char) plus the aliasing pair that used to serve two blocks under one id.
func TestExport_MintedShapeRelabeling(t *testing.T) {
	// given — id populations:
	//   minted        24-hex, relabels to its last 5 chars
	//   readable      hyphenated tail, stays full
	//   constants     "dataview", "featuredRelations": dash-free tails that the
	//                 old charset rule relabeled — must stay full now
	//   short-hex     "abcde": 5 lowercase hex chars, a label look-alike
	//   alias-minted  a real minted id ENDING in "abcde" — must not take the
	//                 short block's id as its label
	const (
		minted     = "64b2c1d2e3f4a5b6c7d8e9f0"
		readable   = "pages-roadmap-home-1"
		constant1  = "dataview"
		constant2  = "featuredRelations"
		shortHex   = "abcde"
		aliasMint  = "fffffffffffffffffffabcde"
		viewUuid   = "32726bf3-cd8b-4099-aafb-688e9525ed67"
		mintedWant = "8e9f0"
	)
	children := []string{minted, readable, constant1, constant2, shortHex, aliasMint}
	blocks := []*model.Block{
		{Id: "rootselfid", ChildrenIds: children,
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
	}
	for _, id := range children {
		blocks = append(blocks, textBlock(id, model.BlockContentText_Paragraph, "text of "+id))
	}
	blocks[3] = &model.Block{Id: constant1, Content: &model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{Views: []*model.BlockContentDataviewView{{
			Id:   viewUuid,
			Type: model.BlockContentDataviewView_Table,
			Name: "All",
		}}},
	}}
	snap := &model.SmartBlockSnapshotBase{
		Details: fields(map[string]*types.Value{"id": str("rootselfid"), "name": str("Doc")}),
		Blocks:  blocks,
	}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{CompactBlockLabels: true})
	require.NoError(t, err)
	// the served document must stay schema-valid
	require.NoError(t, Validate(data))

	var doc struct {
		Blocks []struct {
			Id    string `json:"id"`
			Views []struct {
				Id string `json:"id"`
			} `json:"views"`
		} `json:"blocks"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	served := make([]string, 0, len(doc.Blocks))
	seen := map[string]int{}
	var viewIds []string
	for _, b := range doc.Blocks {
		served = append(served, b.Id)
		seen[b.Id]++
		for _, v := range b.Views {
			viewIds = append(viewIds, v.Id)
			seen[v.Id]++
		}
	}

	// then — which ids come back relabeled
	assert.Contains(t, served, mintedWant, "a minted 24-hex id relabels")
	assert.Contains(t, served, readable, "a hyphenated readable id stays full")
	assert.Contains(t, served, constant1, "the dataview constant stays full")
	assert.Contains(t, served, constant2, "featuredRelations stays full — the old charset rule relabeled it to \"tions\"")
	assert.Contains(t, served, shortHex, "a short id serves as itself")
	assert.Contains(t, served, aliasMint,
		"a minted id whose suffix spells another block's id must stay full, not alias it")
	assert.Equal(t, []string{"5ed67"}, viewIds, "a UUID view id relabels")

	// the invariant behind finding 1, pinned independently of the rule that
	// produces it: no two blocks/views ever share a served id
	for id, n := range seen {
		assert.Equalf(t, 1, n, "served id %q appears %d times", id, n)
	}
}

// The `refs` legend is not merely unwritten — a document that CARRIES one is
// refused, and refused at an address the writer can act on.
//
// This is the headline deletion of v0.20, and until now nothing stood on it.
// The exporter no longer emits `refs`, so every export-side assertion passes
// whether or not a reader would accept one; what makes the deletion real is
// the read side refusing the member, and that rests on a single token in the
// schema — the envelope's `additionalProperties: false`. Re-admitting `refs`
// there (as a permissive object, which is how it was spelled) leaves every
// other test in this package green, and the format quietly grows back a
// legend whose values nothing resolves: the labels inside the document would
// then be read as literal object ids, silently re-pointing every reference.
//
// The refusal has to say WHICH member to drop (§12) — an agent regenerating a
// document from a pre-v0.20 memory needs the name, not "the document is bad
// somewhere" — and at the envelope root that name arrives in the MESSAGE, not
// in the path. The root closes with `additionalProperties: false`, which the
// validator reports once for the object and lists the offending names in its
// text; inside a block the same refusal comes from `unevaluatedProperties`,
// which is reported per member and so carries `/blocks/0/bogus`
// (TestValidate_ErrorsDoNotCascade). The assertions below pin what the
// refusal actually says, message included, so a member-addressed root path
// would be a deliberate change and not a silent one.
func TestValidate_TheRefsLegendIsRefused(t *testing.T) {
	refused := func(t *testing.T, doc string) []Issue {
		t.Helper()
		err := Validate([]byte(doc))
		require.Error(t, err, "a document carrying a `refs` legend must not validate")
		var ve *ValidationError
		require.True(t, errors.As(err, &ve), "got %v", err)
		return ve.Issues
	}

	t.Run("the legend a pre-v0.20 exporter wrote", func(t *testing.T) {
		// the exact shape: short labels in the body, the legend to invert them
		got := refused(t, `{"version": 1, "id": "bafyreiselfobjectidxxxxxxx",
			"refs": {"idxxx": "bafyreimentiontargetidxxx"},
			"blocks": [{"id": "b1", "type": "paragraph",
				"text": "ping <mention object_id=\"idxxx\">Roman</mention>"}]}`)
		require.Len(t, got, 1, "got: %v", got)
		// the refusal addresses the MEMBER, not the envelope holding it: the
		// schema's own closed-set verdict carries the object's location and
		// named `refs` only inside its text, so a reader was handed an empty
		// path for a fault it could point at (§12)
		assert.Equal(t, "/refs", got[0].Path)
		// and it says what happened. `version` is still 1 across the grammar
		// change, so this message is the only place a pre-v0.20 document is
		// told why it stopped validating — and the only place the reader is
		// warned off the repair the bare verdict suggests
		assert.Contains(t, got[0].Message, "§9a")
		assert.Contains(t, got[0].Message, "written in full",
			"the message states the rule that replaced the legend")
		assert.Contains(t, got[0].Message, "address nothing",
			"and warns that deleting the legend alone strands the labels it inverted")
	})

	t.Run("an empty legend is refused as well", func(t *testing.T) {
		// nothing about the refusal may depend on the legend's CONTENTS: a
		// schema node admitting `refs` and constraining it would still let
		// this through
		got := refused(t, `{"version": 1, "refs": {}}`)
		require.Len(t, got, 1, "got: %v", got)
		assert.Equal(t, "/refs", got[0].Path)
		assert.Contains(t, got[0].Message, "§9a")
	})

	t.Run("import refuses it too, so no reader takes the labels literally", func(t *testing.T) {
		_, _, err := Unmarshal([]byte(`{"version": 1, "refs": {"idxxx": "bafyreitarget"},
			"blocks": [{"type": "paragraph", "text": "x"}]}`), Options{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refs")
	})
}
