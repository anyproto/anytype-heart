package wrapper

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTierSets pins BOTH served tool sets as golden lists (§8.20): the
// small tier is the deliberate ~8B curation and every omission is a
// decision — a tool silently moving between tiers, or a new tool landing
// without a tier decision, must fail here.
func TestTierSets(t *testing.T) {
	assert.Equal(t, []string{
		"spaces", "find", "read", "describe", "create", "set_properties",
		"add_blocks", "edit_text",
	}, ToolNamesForTier(TierSmall), "the small tier is a recorded curation — changing it is a §8.20 decision")

	assert.Equal(t, []string{
		"spaces", "find", "read", "describe", "create", "set_properties",
		"check_item", "add_blocks", "edit_text", "set_cell", "move_block",
		"delete_block",
	}, ToolNamesForTier(TierLarge), "the large tier serves the whole table")
}

// TestEveryToolDeclaresTier keeps the tier field mandatory: a Tool with the
// zero tier would silently vanish from the small tier.
func TestEveryToolDeclaresTier(t *testing.T) {
	for _, tool := range Tools() {
		assert.Contains(t, []Tier{TierSmall, TierLarge}, tool.Tier,
			"tool %s must declare a tier — the zero value is not a decision", tool.Name)
	}
}

// TestTierSubset pins small ⊂ large: the tiers are filters over ONE table,
// so the small tier can never serve a tool the large tier lacks.
func TestTierSubset(t *testing.T) {
	large := map[string]bool{}
	for _, name := range ToolNamesForTier(TierLarge) {
		large[name] = true
	}
	for _, name := range ToolNamesForTier(TierSmall) {
		assert.True(t, large[name], "small-tier tool %s missing from the large tier", name)
	}
	assert.Equal(t, len(Tools()), len(ToolsForTier(TierLarge)), "the large tier is the whole table")
}

// TestReadOnlyMarks pins which tools carry the read-only annotation — a
// write tool wrongly marked read-only would skip host write confirmation.
func TestReadOnlyMarks(t *testing.T) {
	want := map[string]bool{"spaces": true, "find": true, "read": true, "describe": true}
	for _, tool := range Tools() {
		assert.Equal(t, want[tool.Name], tool.ReadOnly, "tool %s ReadOnly mark", tool.Name)
	}
}

// TestParseTier pins the flag vocabulary and its steering.
func TestParseTier(t *testing.T) {
	for _, s := range []string{"small", "large"} {
		tier, err := ParseTier(s)
		require.NoError(t, err)
		assert.Equal(t, Tier(s), tier)
	}
	_, err := ParseTier("medium")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown tier "medium" — tiers: small, large`)
}

// TestTierManifest asserts the tier filter reaches the manifest delivery.
func TestTierManifest(t *testing.T) {
	small, err := BuildManifestForTier(TierSmall)
	require.NoError(t, err)
	require.Len(t, small.Tools, len(ToolsForTier(TierSmall)))
	for i, tool := range ToolsForTier(TierSmall) {
		assert.Equal(t, tool.Name, small.Tools[i].Name)
	}

	full, err := BuildManifest()
	require.NoError(t, err)
	assert.Len(t, full.Tools, len(Tools()), "BuildManifest stays the full table")
}
