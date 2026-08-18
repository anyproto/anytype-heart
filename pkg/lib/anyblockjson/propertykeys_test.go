package anyblockjson

// The slug layer is a compaction of key spelling, and like every compaction in
// this format it has to be invertible from the document alone (§9a's rule for
// object ids). It was not: a node-backed vocabulary slugs a custom key
// `6a32d485…` to `priority`, and a reader without that space reads `priority`
// back as the key `priority` — a different relation. A 36 808-object sweep
// caught it as 12 objects whose dataview silently changed which relation it
// pointed at.

import (
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// spaceVocabulary is a node-backed vocabulary: it knows the space's stored
// slugs, which the bundled table cannot.
type spaceVocabulary struct{ slugOf map[string]string }

func (v spaceVocabulary) PropertySlug(key string) string {
	if slug, ok := v.slugOf[key]; ok {
		return slug
	}
	return BundledKeyVocabulary{}.PropertySlug(key)
}

func (v spaceVocabulary) PropertyKey(slug string) (string, bool) {
	for key, s := range v.slugOf {
		if s == slug {
			return key, true
		}
	}
	return BundledKeyVocabulary{}.PropertyKey(slug)
}

func (v spaceVocabulary) TypeSlug(key string) string { return BundledKeyVocabulary{}.TypeSlug(key) }
func (v spaceVocabulary) TypeKey(slug string) (string, bool) {
	return BundledKeyVocabulary{}.TypeKey(slug)
}

func customKeySnapshot(details map[string]*types.Value) *model.SmartBlockSnapshotBase {
	details["id"] = str("o1")
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{Id: "o1",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		Details: fields(details),
	}
}

// The legend carries exactly what the bundled table cannot invert — no more:
// a bundled key needs no entry, because every reader ships the table.
func TestExport_PropertyKeysLegendCarriesWhatTheTableCannot(t *testing.T) {
	vocab := spaceVocabulary{slugOf: map[string]string{"6a32d4856761631534b22f85": "priority"}}
	snap := customKeySnapshot(map[string]*types.Value{
		"6a32d4856761631534b22f85": {Kind: &types.Value_NumberValue{NumberValue: 3}},
		"dueDate":                  str("2026-07-06T08:44:05Z"),
	})

	data, err := Marshal(model.SmartBlockType_Page, snap, Options{Keys: vocab})
	require.NoError(t, err)

	var doc struct {
		Properties   map[string]any    `json:"properties"`
		PropertyKeys map[string]string `json:"property_keys"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Contains(t, doc.Properties, "priority", "the custom key is spelled as its slug")
	assert.Contains(t, doc.Properties, "due_date", "a bundled key is spelled as its slug")
	assert.Equal(t, map[string]string{"priority": "6a32d4856761631534b22f85"}, doc.PropertyKeys,
		"only the entry a package-only reader could not invert")
}

// The point of the legend: a reader with no space gets the stored keys back.
func TestImport_PropertyKeysLegendInvertsWithoutTheSpace(t *testing.T) {
	doc := `{"version": 1, "id": "o1",
		"property_keys": {"priority": "6a32d4856761631534b22f85"},
		"properties": {"priority": 3, "due_date": "2026-07-06T08:44:05Z"}}`

	// a package-only reader — no vocabulary at all
	_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Contains(t, snap.Details.Fields, "6a32d4856761631534b22f85",
		"the legend is what makes the custom slug invertible offline")
	assert.NotContains(t, snap.Details.Fields, "priority")
	assert.Contains(t, snap.Details.Fields, "dueDate", "the bundled table still applies")
}

// The collision the sweep found: a custom key slugs onto a term that is
// another property's stored key. A stored key is always its own address
// (§3 chain step 1), so it keeps the term and the custom key stays verbatim —
// the document can name both, and neither moves.
func TestExport_StoredKeyKeepsItsOwnTerm(t *testing.T) {
	vocab := spaceVocabulary{slugOf: map[string]string{"6a32d4856761631534b22f85": "priority"}}
	snap := customKeySnapshot(map[string]*types.Value{
		"6a32d4856761631534b22f85": {Kind: &types.Value_NumberValue{NumberValue: 3}},
		"priority":                 {Kind: &types.Value_NumberValue{NumberValue: 7}},
	})

	data, err := Marshal(model.SmartBlockType_Page, snap, Options{Keys: vocab})
	require.NoError(t, err)
	require.NoError(t, Validate(data))

	_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, float64(7), back.Details.Fields["priority"].GetNumberValue(),
		"the relation actually keyed priority keeps its value")
	assert.Equal(t, float64(3), back.Details.Fields["6a32d4856761631534b22f85"].GetNumberValue(),
		"and the custom one is still itself")
}
