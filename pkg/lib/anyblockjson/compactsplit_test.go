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
			{Id: "bafyreiselfobjectidxxxxxxx", ChildrenIds: []string{"blockidlongenough1"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			textBlock("blockidlongenough1", model.BlockContentText_Paragraph, "ping Roman",
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
	assert.Equal(t, "blockidlongenough1", doc.Blocks[0].Id)
}

func TestExport_CompactBlockLabelsOnly(t *testing.T) {
	// given / when
	doc := marshalCompactSplit(t, Options{CompactBlockLabels: true})

	// then: block ids relabeled, object refs untouched (no legend)
	assert.Empty(t, doc.Refs)
	require.Len(t, doc.Blocks, 1)
	assert.Equal(t, "ough1", doc.Blocks[0].Id)
}

func TestExport_CompactIdsImpliesBoth(t *testing.T) {
	// given / when
	doc := marshalCompactSplit(t, Options{CompactIds: true})

	// then
	require.Len(t, doc.Refs, 1)
	require.Len(t, doc.Blocks, 1)
	assert.Equal(t, "ough1", doc.Blocks[0].Id)
}
