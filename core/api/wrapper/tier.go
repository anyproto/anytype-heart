package wrapper

// tier.go — the model-tier split over the ONE tool table (APIV2.md §8.20).
// A tier is a served subset of Tools(), selected at serve time (MCP flag,
// manifest flag); it is a field on the existing definitions, never a second
// hand-maintained list — the §8.6 one-definition invariant extends to
// tiers. Two tiers exist:
//
//   - small (~8B on-device models, Gemma-class): the minimal task set.
//     Every omission is deliberate — for a small model a missing capability
//     is better than a surface it misuses (§8.20 records each omission).
//   - large (~20B models, Qwen-class): the full task-tool set — still
//     task-shaped, still under the 15-tool cliff.
//
// The CLI verb set is NOT tiered: coding-agent harnesses drive large
// models, so the verbs stay the full table.

import (
	"fmt"
	"strings"
)

// Tier names a served tool subset. On a Tool the field means "the smallest
// tier that serves this tool": small-tier tools are served to both tiers;
// large-tier tools only to large.
type Tier string

const (
	// TierSmall is the ~8B tier: the minimal task set.
	TierSmall Tier = "small"
	// TierLarge is the ~20B tier: the full task-tool set.
	TierLarge Tier = "large"
)

// ParseTier maps a flag value to a Tier with steering.
func ParseTier(s string) (Tier, error) {
	switch Tier(s) {
	case TierSmall, TierLarge:
		return Tier(s), nil
	default:
		return "", fmt.Errorf("unknown tier %q — tiers: %s, %s", s, TierSmall, TierLarge)
	}
}

// ToolsForTier returns the tier's served subset of the one tool table, in
// documentation order. The large tier serves every tool.
func ToolsForTier(tier Tier) []Tool {
	all := Tools()
	if tier == TierLarge {
		return all
	}
	out := make([]Tool, 0, len(all))
	for _, t := range all {
		if t.Tier == TierSmall {
			out = append(out, t)
		}
	}
	return out
}

// ToolNamesForTier returns the tier's tool names in documentation order.
func ToolNamesForTier(tier Tier) []string {
	tools := ToolsForTier(tier)
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

// toolListForError renders a tier's tool names for steering text.
func toolListForError(tier Tier) string {
	return strings.Join(ToolNamesForTier(tier), ", ")
}
