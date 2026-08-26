package anyblockjson

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A slot that restricts the icon variants must name ITS OWN set once, not the
// object's set and then its own. `plainIcon` used to be
// `allOf: [{$ref: icon}, {format: {enum: [emoji, file]}}]`, so a value outside
// BOTH enums failed both and the reader got two contradictory verdicts, the
// WIDER one first:
//
//	/blocks/0/icon/format: value must be one of 'emoji', 'file', 'icon', 'color'
//	/blocks/0/icon/format: value must be one of 'emoji', 'file'
//
// A greedy repairer reads line 1, writes `format: "icon"`, and gets a fresh
// error — two round trips where one would do. That is the disease this whole
// section exists to cure, in miniature, on the surface where the wrong guess is
// most likely: `image` is both a block type name and the cover variant name.
//
// The fix splits the variant machinery (`iconVariants`) from the variant NAMES,
// so each slot states its own enum once. These tests can only fail if a slot
// starts inheriting a second enum: each asserts the COUNT and the wording, so
// re-introducing the wider verdict fails on the count even if the narrow one is
// still present.
func TestValidate_ARestrictedIconSlotNamesOneUnion(t *testing.T) {
	firstIssues := func(t *testing.T, doc string) []Issue {
		t.Helper()
		err := Validate([]byte(doc))
		require.Error(t, err)
		var ve *ValidationError
		require.True(t, errors.As(err, &ve))
		return ve.Issues
	}

	for name, doc := range map[string]string{
		"a cover variant on a callout": `{"version": 1, "type": "page", "blocks": [
			{"type": "callout", "icon": {"format": "image", "file": "bafy1"}, "text": "x"}]}`,
		"an invented format on a callout": `{"version": 1, "type": "page", "blocks": [
			{"type": "callout", "icon": {"format": "url", "url": "http://x"}, "text": "x"}]}`,
		"an object-only variant on a callout": `{"version": 1, "type": "page", "blocks": [
			{"type": "callout", "icon": {"format": "icon", "name": "rocket"}, "text": "x"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			issues := firstIssues(t, doc)
			require.Len(t, issues, 1, "one fault, one issue (§12): %v", issues)
			assert.Contains(t, issues[0].Message, "'emoji', 'file'",
				"and it names the slot's OWN union")
			assert.NotContains(t, issues[0].Message, "'icon'",
				"never the object's wider one — that is what sent a repairer the wrong way")
		})
	}

	// the control: the OBJECT slot still names all four, so the fix cannot pass
	// by narrowing every slot to the callout's set
	t.Run("the object slot still names all four", func(t *testing.T) {
		issues := firstIssues(t, `{"version": 1, "type": "page", "icon": {"format": "url", "url": "http://x"}}`)
		require.Len(t, issues, 1)
		assert.Contains(t, issues[0].Message, "'emoji', 'file', 'icon', 'color'")
	})

	// and both slots still accept what they should
	t.Run("valid icons still validate", func(t *testing.T) {
		for _, doc := range []string{
			`{"version": 1, "type": "page", "icon": {"format": "emoji", "emoji": "📕"}}`,
			`{"version": 1, "type": "page", "icon": {"format": "icon", "name": "rocket"}}`,
			`{"version": 1, "type": "page", "blocks": [{"type": "callout", "icon": {"format": "emoji", "emoji": "📕"}, "text": "x"}]}`,
		} {
			require.NoError(t, Validate([]byte(doc)), fmt.Sprintf("doc: %s", doc))
		}
	})
}
