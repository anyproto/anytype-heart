package anyblockjson

// compactsplit_test.go covers the C4 split of id compaction into
// CompactObjectRefs (lossless refs legend) and CompactBlockLabels (lossy
// local relabeling) — API v2 default reads need the former without the
// latter.

import (
	"encoding/json"
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
			{Id: "bafyreiselfobjectidxxxxxxx", ChildrenIds: []string{"64b2c1d2e3f4a5b6c7d8e9f0"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			textBlock("64b2c1d2e3f4a5b6c7d8e9f0", model.BlockContentText_Paragraph, "ping Roman",
				mark(mMention, 5, 10, "bafyreimentiontargetidxxx")),
		},
	}
}

type parsedCompactDoc struct {
	Refs   map[string]string `json:"refs"`
	Blocks []struct {
		Id string `json:"id"`
	} `json:"blocks"`
}

func marshalCompactSplit(t *testing.T, opts Options) parsedCompactDoc {
	t.Helper()
	data, err := Marshal(model.SmartBlockType_Page, compactSplitSnapshot(), opts)
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	var doc parsedCompactDoc
	require.NoError(t, json.Unmarshal(data, &doc))
	return doc
}

func TestExport_CompactObjectRefsOnly(t *testing.T) {
	// given / when
	doc := marshalCompactSplit(t, Options{CompactObjectRefs: true})

	// then: object refs compacted with legend, block ids stay full
	require.Len(t, doc.Refs, 1)
	assert.Equal(t, "bafyreimentiontargetidxxx", doc.Refs["idxxx"])
	require.Len(t, doc.Blocks, 1)
	assert.Equal(t, "64b2c1d2e3f4a5b6c7d8e9f0", doc.Blocks[0].Id)
}

func TestExport_CompactBlockLabelsOnly(t *testing.T) {
	// given / when
	doc := marshalCompactSplit(t, Options{CompactBlockLabels: true})

	// then: block ids relabeled, object refs untouched (no legend)
	assert.Empty(t, doc.Refs)
	require.Len(t, doc.Blocks, 1)
	assert.Equal(t, "8e9f0", doc.Blocks[0].Id)
}

func TestExport_CompactIdsImpliesBoth(t *testing.T) {
	// given / when
	doc := marshalCompactSplit(t, Options{CompactIds: true})

	// then
	require.Len(t, doc.Refs, 1)
	require.Len(t, doc.Blocks, 1)
	assert.Equal(t, "8e9f0", doc.Blocks[0].Id)
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
