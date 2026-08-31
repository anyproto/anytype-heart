package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/internal/anyblockbatch"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The `option_ids` legend (SPEC §3, §9a) is resolved inside
// pkg/lib/anyblockjson, but whether it does anything at all is decided HERE:
// the package honours a legend id only where the wired OptionResolver
// confirms it is a live option of that relation, and the confirming call is
// OptionName. This tool implements that resolver, so the legend is live for
// the flagship import path only as long as the two sides agree about what
// OptionName means — and they are declared in different packages, one as an
// interface method with two duties and one as a method on `batch`.
//
// They did not agree. OptionName was an export-only call when this tool was
// written ("what is this option id called?"), so `batch` stubbed it to false;
// the legend then made it the import-side liveness check, and every legend
// entry silently failed liveness — anyblockconvert ignored `option_ids`
// wholesale while nothing failed anywhere. Nothing structural stops that
// happening again, so the guard has to be a test that crosses the boundary.
// It asserts the two properties that matter across the seam and nothing about
// how either side is built:
//
//   - an id this resolver SERVES is honoured, in preference to the name;
//   - an id it does NOT serve is not, and never reaches the snapshot — the
//     archive must not reference an option object it does not carry.
func TestBatch_OptionIdsLegendCrossesIntoThePackage(t *testing.T) {
	newStageBatch := func() *batch {
		return newBatch(map[string]anyblockbatch.FormatInfo{
			"stage": {
				Format:  model.RelationFormat_status,
				Options: vocab("Blocked", "Done"),
			},
		}, nil)
	}

	t.Run("an id this batch serves wins over the name", func(t *testing.T) {
		b := newStageBatch()
		blocked, ok := b.OptionId(domain.RelationKey("stage"), "Blocked")
		require.True(t, ok)
		done, ok := b.OptionId(domain.RelationKey("stage"), "Done")
		require.True(t, ok)
		require.NotEqual(t, blocked, done, "the decoy only works if the two options are distinct objects")

		// the rename case the legend exists for: the value still spells the
		// old name, the legend carries the id the name stood for, and the
		// option now goes by something else. Name resolution would answer
		// `done` — a DIFFERENT live option of the same relation, so the
		// assertion below cannot pass by agreeing with the fallback.
		_, snap := convertDoc(t, b, "renamed.json", stageDoc(t, "Done", map[string]string{"Done": blocked}))

		got := optionValues(t, snap)
		assert.Equal(t, []string{blocked}, got,
			"the legend id must win: %q is what resolving the name alone answers", done)
		assertEveryOptionMinted(t, b, got)
	})

	t.Run("an id from another space is not honoured", func(t *testing.T) {
		b := newStageBatch()
		done, ok := b.OptionId(domain.RelationKey("stage"), "Done")
		require.True(t, ok)

		// what every real bundle carries: ids minted by the space that
		// exported it, naming nothing in the archive being built
		_, snap := convertDoc(t, b, "foreign.json", stageDoc(t, "Done", map[string]string{"Done": "bafydecoy"}))

		got := optionValues(t, snap)
		assert.Equal(t, []string{done}, got,
			"a foreign id fails liveness, so the value resolves by name as if the legend were absent")
		assertEveryOptionMinted(t, b, got)
	})
}

// stageDoc is a one-property document: a `stage` value, and optionally the
// `option_ids` entry for it.
func stageDoc(t *testing.T, value string, legend map[string]string) string {
	t.Helper()
	doc := map[string]any{
		"version":    2,
		"id":         "obj-1",
		"properties": map[string]any{"stage": value},
	}
	if legend != nil {
		doc["option_ids"] = map[string]any{"stage": legend}
	}
	raw, err := json.Marshal(doc)
	require.NoError(t, err)
	return string(raw)
}

// optionValues reads the resolved `stage` value off the snapshot.
func optionValues(t *testing.T, snap *model.SmartBlockSnapshotBase) []string {
	t.Helper()
	v := snap.Details.GetFields()["stage"]
	require.NotNil(t, v, "the value must reach the detail")
	var out []string
	for _, item := range v.GetListValue().GetValues() {
		out = append(out, item.GetStringValue())
	}
	require.NotEmpty(t, out, "a resolved select value is an option id list, not the raw string %q", v.GetStringValue())
	return out
}

// assertEveryOptionMinted is the safety half: an option id the converter
// writes into a snapshot must be an object the same batch also carries, or
// the archive imports with a dangling reference.
func assertEveryOptionMinted(t *testing.T, b *batch, ids []string) {
	t.Helper()
	minted := map[string]bool{}
	for _, p := range b.pending {
		if p.sbType == model.SmartBlockType_STRelationOption {
			minted[p.id] = true
		}
	}
	for _, id := range ids {
		assert.True(t, minted[id], "option id %q reaches the snapshot but no RelationOption object is minted for it", id)
	}
}
