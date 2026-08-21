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
