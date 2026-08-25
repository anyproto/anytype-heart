package anyblockjson

// fragmentlegend_test.go — a fragment carries what its blocks mean.
//
// The fragment surface (fragment.go, filters.go, BuildRecommendedLists) is
// the seam that edits live objects op-by-op, and it ran the §3 resolution
// chain from step 2 in both directions. Export computed all three legends and
// discarded them at the return; import built `&jsonDoc{}`, an empty legend, so
// step 1 was unconditionally silent. A block cut out of a document that said
// `{"priority": "6a32d485…"}` resolved `priority` through the READER's table
// — the exact misresolution the legend exists to prevent, reintroduced at the
// one seam where it lands on a live object.
//
// Every test here uses a DECOY: the reader's vocabulary binds the spelling to
// a different stored key, and the option pool holds a same-named option under
// a different id. Without a decoy the legend and the fallback agree and the
// test asks nothing.

import (
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const fragKey = "6a32d4856761631534b22f85"

// fragSubtree is a property block and a dataview naming one custom key, with
// a filter carrying a select value — the three fragment slots that owe a
// legend between them.
func fragSubtree() []*model.Block {
	return []*model.Block{
		{Id: "root1", ChildrenIds: []string{"rel1", "dv1"},
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "holder"}}},
		{Id: "rel1", Content: &model.BlockContentOfRelation{
			Relation: &model.BlockContentRelation{Key: fragKey}}},
		{Id: "dv1", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
			RelationLinks: []*model.RelationLink{{Key: fragKey, Format: model.RelationFormat_tag}},
			Views: []*model.BlockContentDataviewView{{Id: "v1", Name: "All",
				Type: model.BlockContentDataviewView_Table,
				Filters: []*model.BlockContentDataviewFilter{{
					Id: "f1", RelationKey: fragKey, Format: model.RelationFormat_tag,
					Condition: model.BlockContentDataviewFilter_In,
					Value:     strList("bafylive"),
				}}}},
		}}},
	}
}

// fragSpace is the writer's space: it spells the custom key `priority` and
// serves one option called High.
func fragSpace() Options {
	return Options{
		Keys:           slugVocab{slugs: map[string]string{fragKey: "priority"}},
		ResolveFormat:  selectFormats,
		ResolveOptions: spaceOptions{fragKey: {{id: "bafylive", name: "High"}}},
	}
}

// The export half: the legends the blocks owe come back with them.
func TestMarshalBlockSubtree_CarriesTheLegendsItsBlocksOwe(t *testing.T) {
	// when
	out, err := MarshalBlockSubtree(fragSubtree(), fragSpace())

	// then — the run spells the space's slug, and the envelope says what it
	// means; without the legend `priority` is just a word
	require.NoError(t, err)
	blocks := fragmentBlocks(t, out)
	require.Len(t, blocks, 3)
	assert.Equal(t, "priority", blocks[1]["key"], "the property block spells the slug")

	legend := fragmentLegend(t, out)
	assert.Equal(t, map[string]string{"priority": fragKey}, legend.PropertyKeys)
	assert.Equal(t, map[string]map[string]string{"priority": {"High": "bafylive"}}, legend.OptionIds)

	// and the member order is the envelope's (§2, §4): the legend that
	// inverts a spelling precedes the legend keyed BY one
	s := string(out)
	assert.Less(t, indexOf(s, `"property_internal_keys"`), indexOf(s, `"option_ids"`))
	assert.Less(t, indexOf(s, `"option_ids"`), indexOf(s, `"blocks"`))
}

// The import half, with a decoy on both axes: the reader binds `priority` to
// another relation, and its option pool holds a SECOND option also called
// High. The legend has to beat both.
func TestUnmarshalBlocks_HonoursTheLegendItIsHandedOverItsOwnVocabulary(t *testing.T) {
	// given — the fragment the writer produced, and a reader that disagrees
	out, err := MarshalBlockSubtree(fragSubtree(), fragSpace())
	require.NoError(t, err)
	var env struct {
		Blocks []json.RawMessage `json:"blocks"`
	}
	require.NoError(t, json.Unmarshal(out, &env))
	legend := fragmentLegend(t, out)

	reader := Options{
		GenerateId:    seqIds("g"),
		Keys:          slugVocab{slugs: map[string]string{"decoyKey": "priority"}},
		ResolveFormat: selectFormats,
		ResolveOptions: spaceOptions{
			fragKey: {
				{id: "bafydecoy", name: "High"}, // what a NAME resolves to
				{id: "bafylive", name: "High"},  // what the LEGEND names
			},
			"decoyKey": {{id: "bafydecoy", name: "High"}},
		},
	}

	t.Run("without the legend the reader's own answers win", func(t *testing.T) {
		blocks, _, err := UnmarshalBlocks(env.Blocks, reader)
		require.NoError(t, err)
		assert.Equal(t, "decoyKey", fragBlockKey(t, blocks),
			"`priority` is this reader's spelling for a different relation")
	})

	t.Run("with it the document's own statement is chain step 1", func(t *testing.T) {
		withLegend := reader
		withLegend.Legend = legend

		blocks, _, err := UnmarshalBlocks(env.Blocks, withLegend)
		require.NoError(t, err)
		assert.Equal(t, fragKey, fragBlockKey(t, blocks),
			"the legend names the relation the fragment was cut from")
		assert.Equal(t, []string{"bafylive"}, valueStringList(fragFilterValue(t, blocks)),
			"and the option id beats the same-named decoy the pool answers first")
	})
}

// The value-level pair. MarshalPropertyValue writes an option NAME and used
// to drop the id it stood for, so every row-level caller silently had the
// pre-§9a behaviour: a shared name lands on whichever option answers first.
func TestPropertyValue_TheOptionIdSurvivesTheValueLevelSurface(t *testing.T) {
	// given — a space with two options called High, one of them the value's
	writer := Options{
		ResolveFormat: selectFormats,
		ResolveOptions: spaceOptions{fragKey: {
			{id: "bafydecoy", name: "High"},
			{id: "bafylive", name: "High"},
		}},
	}

	// when
	out, ids := MarshalPropertyValue(fragKey, strList("bafylive"), writer)

	// then
	assert.Equal(t, []any{"High"}, out, "the value is written as the option NAME")
	require.Equal(t, map[string]string{"High": "bafylive"}, ids,
		"and the id it stood for comes back, or the name is all the reader gets")

	// the reader: name resolution alone answers the decoy, which is the loss
	back := UnmarshalPropertyValue(fragKey, out, writer)
	assert.Equal(t, []string{"bafydecoy"}, valueStringList(back),
		"first match wins, and it is the wrong option")

	// handed the id, it answers the value that was written
	withLegend := writer
	withLegend.Legend = Legend{OptionIds: map[string]map[string]string{fragKey: ids}}
	back = UnmarshalPropertyValue(fragKey, out, withLegend)
	assert.Equal(t, []string{"bafylive"}, valueStringList(back))
}

// UnmarshalFilters is the query-side twin, and it resolves both a key slot and
// an option value.
func TestUnmarshalFilters_HonoursTheLegendItIsHandedOver(t *testing.T) {
	raw := json.RawMessage(`[{"property":"priority","condition":"in","value":["High"]}]`)
	reader := Options{
		Keys:          slugVocab{slugs: map[string]string{"decoyKey": "priority"}},
		ResolveFormat: selectFormats,
		ResolveOptions: spaceOptions{
			fragKey:    {{id: "bafydecoy", name: "High"}, {id: "bafylive", name: "High"}},
			"decoyKey": {{id: "bafydecoy", name: "High"}},
		},
	}

	t.Run("without the legend", func(t *testing.T) {
		got, err := UnmarshalFilters(raw, reader)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "decoyKey", got[0].RelationKey)
	})

	t.Run("with it", func(t *testing.T) {
		withLegend := reader
		withLegend.Legend = Legend{
			PropertyKeys: map[string]string{"priority": fragKey},
			OptionIds:    map[string]map[string]string{"priority": {"High": "bafylive"}},
		}

		got, err := UnmarshalFilters(raw, withLegend)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, fragKey, got[0].RelationKey)
		assert.Equal(t, []string{"bafylive"}, valueStringList(got[0].Value),
			"the option legend is keyed by the SPELLING the slot wrote (§9a)")
	})
}

// BuildRecommendedLists is the PATCH-type door into the same array
// applyTypeProperties reads out of a document, and its own doc comment used to
// say the caller "owes them that document's legend before calling, because
// nothing downstream of this signature can see it". Now it can.
func TestBuildRecommendedLists_HonoursTheLegendItIsHandedOver(t *testing.T) {
	props := []TypeProperty{{Property: "priority", Section: "featured"}}
	reader := Options{Keys: slugVocab{slugs: map[string]string{"decoyKey": "priority"}}}

	t.Run("without the legend", func(t *testing.T) {
		lists, err := BuildRecommendedLists(props, reader)
		require.NoError(t, err)
		assert.Equal(t, []string{"decoyKey"}, listKeys(t, lists, "recommendedFeaturedRelations"))
	})

	t.Run("with it", func(t *testing.T) {
		withLegend := reader
		withLegend.Legend = Legend{PropertyKeys: map[string]string{"priority": fragKey}}

		lists, err := BuildRecommendedLists(props, withLegend)
		require.NoError(t, err)
		assert.Equal(t, []string{fragKey}, listKeys(t, lists, "recommendedFeaturedRelations"),
			"the type's recommended list names the relation the document meant")
	})
}

// fragBlockKey is the property block's resolved key in an imported run.
func fragBlockKey(t *testing.T, blocks []*model.Block) string {
	t.Helper()
	for _, b := range blocks {
		if c, ok := b.Content.(*model.BlockContentOfRelation); ok {
			return c.Relation.Key
		}
	}
	t.Fatal("no property block in the run")
	return ""
}

// fragFilterValue is the single dataview filter's value in an imported run.
func fragFilterValue(t *testing.T, blocks []*model.Block) *types.Value {
	t.Helper()
	for _, b := range blocks {
		if c, ok := b.Content.(*model.BlockContentOfDataview); ok {
			require.Len(t, c.Dataview.Views, 1)
			require.Len(t, c.Dataview.Views[0].Filters, 1)
			return c.Dataview.Views[0].Filters[0].Value
		}
	}
	t.Fatal("no dataview in the run")
	return nil
}

// listKeys is the resolved key list of one recommended section, by the detail
// key that section writes to.
func listKeys(t *testing.T, lists []RecommendedList, detailKey string) []string {
	t.Helper()
	for _, l := range lists {
		if l.DetailKey == detailKey {
			return l.Ids
		}
	}
	t.Fatalf("no %q list in %v", detailKey, lists)
	return nil
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
