package snapshotdiff

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// A type document does not carry its own install provenance (§2a, v0.32):
// eight stored keys come back absent from the round trip, and the five
// lifted settings come back absent when their stored value was empty. Both
// are the format's documented normalizations, so the comparator must accept
// exactly them — through the format's OWN predicates, never a copy — and
// still report everything else. When the exporter and this comparator last
// drifted on a rule of this shape, 1,344 of 1,351 differing objects in one
// sweep were false failures.
//
// How this can fail: teach export a drop without wiring the predicate here
// (the first case reports every provenance key as a change), or suppress
// more than the rule says (the second and third cases go quiet on real
// loss).
func TestTypeProvenance_DropIsNormalizationOnTypeDocuments(t *testing.T) {
	orig := snapWith(map[string]*types.Value{
		"name":       str2("Task"),
		"origin":     num2(7),
		"revision":   num2(3),
		"setOf":      list("bafyreinothing"),
		"pluralName": str2(""), // present-and-empty: the omit-empty canon
	})

	t.Run("the documented drops are silent", func(t *testing.T) {
		// given the real round trip, not a hand-built got
		data, err := anyblockjson.Marshal(model.SmartBlockType_STType, orig, anyblockjson.Options{})
		require.NoError(t, err)
		_, got, err := anyblockjson.Unmarshal(data, anyblockjson.Options{})
		require.NoError(t, err)

		// when
		found := Compare(orig, got, model.SmartBlockType_STType, anyblockjson.Options{})

		// then
		assert.Empty(t, found, "the drops are the format's decision, not loss")
	})

	t.Run("the same keys still report on a page", func(t *testing.T) {
		got := snapWith(map[string]*types.Value{"name": str2("Task")})
		found := Compare(orig, got, model.SmartBlockType_Page, anyblockjson.Options{})
		assert.NotEmpty(t, found, "off a type document, origin and setOf are real data")
	})

	t.Run("a non-empty lifted setting that vanishes still reports", func(t *testing.T) {
		withName := snapWith(map[string]*types.Value{"pluralName": str2("Tasks")})
		got := snapWith(map[string]*types.Value{})
		found := Compare(withName, got, model.SmartBlockType_STType, anyblockjson.Options{})
		assert.NotEmpty(t, found, "only the EMPTY setting's omission is documented")
	})
}

func str2(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}

func num2(n float64) *types.Value {
	return &types.Value{Kind: &types.Value_NumberValue{NumberValue: n}}
}
