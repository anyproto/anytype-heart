package snapshotdiff

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// A type's four role lists are usually ABSENT in the store when nothing
// occupies the role, and import rebuilds all four, so the round trip adds an
// empty one. That step is normalization (see recommendedListKeys) — but only
// that step: a list arriving with members, or an empty list on a key that is
// not one of the four, is still a real difference. These pin both directions,
// so the suppression cannot quietly grow.
func snapWith(details map[string]*types.Value) *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Details:     &types.Struct{Fields: details},
		ObjectTypes: []string{"ot-objectType"},
	}
}

func list(vals ...string) *types.Value {
	out := make([]*types.Value, 0, len(vals))
	for _, v := range vals {
		out = append(out, &types.Value{Kind: &types.Value_StringValue{StringValue: v}})
	}
	return &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: out}}}
}

func TestCompare_EmptyRecommendedListIsNormalization(t *testing.T) {
	fileKey := bundle.RelationKeyRecommendedFileRelations.String()
	featuredKey := bundle.RelationKeyRecommendedFeaturedRelations.String()

	t.Run("an added empty role list is not reported", func(t *testing.T) {
		orig := snapWith(map[string]*types.Value{"name": {Kind: &types.Value_StringValue{StringValue: "Task"}}})
		got := snapWith(map[string]*types.Value{
			"name":  {Kind: &types.Value_StringValue{StringValue: "Task"}},
			fileKey: list(),
		})
		assert.Empty(t, Compare(orig, got, model.SmartBlockType_Page, anyblockjson.Options{}))
	})

	t.Run("an added role list WITH members is still reported", func(t *testing.T) {
		orig := snapWith(map[string]*types.Value{"name": {Kind: &types.Value_StringValue{StringValue: "Task"}}})
		got := snapWith(map[string]*types.Value{
			"name":  {Kind: &types.Value_StringValue{StringValue: "Task"}},
			fileKey: list("rel-cover"),
		})
		found := Compare(orig, got, model.SmartBlockType_Page, anyblockjson.Options{})
		require.Len(t, found, 1, "a role list that gained content is real drift, not normalization")
		assert.Contains(t, found[0], fileKey)
	})

	t.Run("a role list that LOST its members is still reported", func(t *testing.T) {
		orig := snapWith(map[string]*types.Value{featuredKey: list("rel-name")})
		got := snapWith(map[string]*types.Value{featuredKey: list()})
		found := Compare(orig, got, model.SmartBlockType_Page, anyblockjson.Options{})
		require.Len(t, found, 1, "emptying an existing list is loss; only absent->empty is normalization")
	})

	t.Run("an empty list on an unrelated key is still reported", func(t *testing.T) {
		orig := snapWith(map[string]*types.Value{"name": {Kind: &types.Value_StringValue{StringValue: "Task"}}})
		got := snapWith(map[string]*types.Value{
			"name": {Kind: &types.Value_StringValue{StringValue: "Task"}},
			"tag":  list(),
		})
		found := Compare(orig, got, model.SmartBlockType_Page, anyblockjson.Options{})
		require.Len(t, found, 1, "the rule is scoped to the four role lists")
		assert.Contains(t, found[0], "tag")
	})
}
