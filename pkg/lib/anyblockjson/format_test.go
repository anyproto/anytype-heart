package anyblockjson

// There is one text format on the wire, "text" (§3). The stored
// longtext/shorttext split survives it because import resolves "text"
// against the key's existing format.

import (
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func dataviewSnapshot(links ...*model.RelationLink) *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "root", ChildrenIds: []string{"dataview"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "dataview", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				RelationLinks: links,
				Views:         []*model.BlockContentDataviewView{{Id: "v1", Name: "All"}},
			}}},
		},
		Details: fields(map[string]*types.Value{"id": str("root")}),
	}
}

func linkFormats(t *testing.T, snap *model.SmartBlockSnapshotBase) map[string]model.RelationFormat {
	t.Helper()
	for _, b := range snap.Blocks {
		if dv := b.GetDataview(); dv != nil {
			out := map[string]model.RelationFormat{}
			for _, rl := range dv.RelationLinks {
				out[rl.Key] = rl.Format
			}
			return out
		}
	}
	t.Fatal("no dataview block")
	return nil
}

// Both stored text formats serialize to the single name "text".
func TestExport_TextFormatsCollapse(t *testing.T) {
	data, err := Marshal(model.SmartBlockType_Page, dataviewSnapshot(
		&model.RelationLink{Key: "name", Format: model.RelationFormat_shorttext},
		&model.RelationLink{Key: "description", Format: model.RelationFormat_longtext},
	), testOptions())
	require.NoError(t, err)

	assert.NotContains(t, string(data), "shortText")
	assert.Contains(t, string(data), `"property": "Name"`)
	assert.Equal(t, 2, strings.Count(string(data), `"format": "text"`))
}

// The collapse is not lossy: a key already known to be shorttext gets its
// stored format back, so proto -> json -> proto is an identity for it.
func TestRoundtrip_ShortTextSurvivesCollapse(t *testing.T) {
	t.Run("bundled key", func(t *testing.T) {
		data, err := Marshal(model.SmartBlockType_Page, dataviewSnapshot(
			&model.RelationLink{Key: "name", Format: model.RelationFormat_shorttext},
			&model.RelationLink{Key: "description", Format: model.RelationFormat_longtext},
		), testOptions())
		require.NoError(t, err)

		_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
		require.NoError(t, err)

		got := linkFormats(t, back)
		assert.Equal(t, model.RelationFormat_shorttext, got["name"], "bundled name is shorttext")
		assert.Equal(t, model.RelationFormat_longtext, got["description"])
	})

	// same rule via the wiring's resolver, for non-bundled keys
	t.Run("resolver key", func(t *testing.T) {
		doc := `{"version": 1, "id": "root", "blocks": [{"type": "dataview",
			"properties": [{"property": "legacyShort", "format": "text"},
			               {"property": "plainNote", "format": "text"}],
			"views": [{"name": "All"}]}]}`
		opts := Options{GenerateId: seqIds("g")}
		opts.ResolveFormat = func(key domain.RelationKey) (model.RelationFormat, bool) {
			if key == "legacyShort" {
				return model.RelationFormat_shorttext, true
			}
			return 0, false
		}
		_, snap, err := Unmarshal([]byte(doc), opts)
		require.NoError(t, err)

		got := linkFormats(t, snap)
		assert.Equal(t, model.RelationFormat_shorttext, got["legacyShort"])
		assert.Equal(t, model.RelationFormat_longtext, got["plainNote"], "unknown key is a new text property")
	})

	// a resolver disagreeing about a *non*-text format must not win: the
	// document stays authoritative for every name that is unambiguous.
	t.Run("only text defers to the key", func(t *testing.T) {
		doc := `{"version": 1, "id": "root", "blocks": [{"type": "dataview",
			"properties": [{"property": "customDate", "format": "number"}],
			"views": [{"name": "All"}]}]}`
		_, snap, err := Unmarshal([]byte(doc), Options{
			GenerateId:    seqIds("g"),
			ResolveFormat: testFormatResolver, // says customDate is a date
		})
		require.NoError(t, err)
		assert.Equal(t, model.RelationFormat_number, linkFormats(t, snap)["customDate"])
	})
}

// shortText is gone from the vocabulary, not merely unused.
func TestValidate_ShortTextRejected(t *testing.T) {
	doc := `{"version": 1, "id": "root", "blocks": [{"type": "dataview",
		"properties": [{"property": "name", "format": "shortText"}],
		"views": [{"name": "All"}]}]}`
	_, _, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "format")
}
