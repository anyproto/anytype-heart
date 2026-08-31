package anyblockbatch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeDoc(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

// A view naming a property nothing declares is the silent class: the filter
// matches nothing, so the view shows everything it was meant to narrow, and
// the document validates, imports and round-trips byte-stably. Six of nine
// agents asked to "also exclude archived items" produced exactly that.
//
// It is a BATCH check because a per-document reader cannot judge it. A custom
// property whose stored key is already a legal spelling — `aroma_notes`, and
// 112 more in a 77-space corpus — binds no legend entry, because the spelling
// IS the key; inside one document that is indistinguishable from a typo. The
// codec-level version of this check fired on exactly that case, which is how
// the distinction was found.
//
// How this can fail: judge a spelling the dataview's own properties list
// declares, or one the bundle declares elsewhere, and a correct bundle is
// refused.
func TestCheckViewProperties(t *testing.T) {
	dir := t.TempDir()
	f := writeDoc(t, dir, "board.anyblock.json", `{"version":2,"type":"page",
	  "property_internal_keys":{"assignee":"6a32d4856761631534b22f85"},
	  "blocks":[{"type":"dataview","object_id":"b1",
	    "properties":[{"property":"local_only","format":"checkbox"}],
	    "views":[{"id":"v","filters":[
	      {"property":"status","condition":"equal","value":"Done"},
	      {"property":"is_archived","condition":"not_equal","value":true},
	      {"property":"assignee","condition":"not_empty"},
	      {"property":"local_only","condition":"equal","value":true},
	      {"operator":"or","filters":[{"property":"nowhere","condition":"not_empty"}]}],
	     "sorts":[{"property":"whenever"}],
	     "columns":[{"property":"status"}]}]}]}`)

	// what the bundle declares: the dictionary's own entries, plus the stored
	// key the legend binds `assignee` to — a real bundle declares the key it
	// binds, and the case where it does not is its own finding below
	declared := map[string]bool{"status": true, "6a32d4856761631534b22f85": true}

	bad, err := CheckViewProperties([]string{f}, declared)
	require.NoError(t, err)

	got := map[string]string{}
	for _, b := range bad {
		got[b.Property] = b.Slot
	}
	assert.Equal(t, map[string]string{"nowhere": "filter", "whenever": "sort"}, got,
		"only the two that nothing declares, and the group node names no property itself")

	t.Run("what must NOT be flagged", func(t *testing.T) {
		for _, prop := range []string{
			"status",      // the property dictionary declares it
			"is_archived", // a bundled property
			"assignee",    // bound by this document's legend
			"local_only",  // the dataview's own properties list declares it
		} {
			assert.NotContainsf(t, got, prop, "%q is declared and must pass", prop)
		}
	})

	// A legend says which stored key a spelling MEANS. It does not make that
	// key exist: bind a spelling to a key nothing declares and the filter is
	// still a no-op, so the binding must not excuse it.
	t.Run("a legend binding to a key nothing declares is still a finding", func(t *testing.T) {
		bad, err := CheckViewProperties([]string{f}, map[string]bool{"status": true})
		require.NoError(t, err)
		found := false
		for _, b := range bad {
			if b.Property == "assignee" {
				found = true
			}
		}
		assert.True(t, found, "the legend points at 6a32d485…, which nothing declares")
	})

	t.Run("a verbatim custom key the bundle declares is not a typo", func(t *testing.T) {
		// the case the codec cannot tell apart: no legend entry, because the
		// spelling is the stored key
		f := writeDoc(t, dir, "note.anyblock.json", `{"version":2,"type":"page",
		  "blocks":[{"type":"dataview","object_id":"b2","views":[{"id":"v",
		    "filters":[{"property":"aroma_notes","condition":"not_empty"}]}]}]}`)
		bad, err := CheckViewProperties([]string{f}, map[string]bool{"aroma_notes": true})
		require.NoError(t, err)
		assert.Empty(t, bad)

		t.Run("and is a typo when the bundle declares nothing", func(t *testing.T) {
			bad, err := CheckViewProperties([]string{f}, map[string]bool{})
			require.NoError(t, err)
			require.Len(t, bad, 1)
			assert.Equal(t, "aroma_notes", bad[0].Property)
		})
	})

	t.Run("the report says what the slot does instead", func(t *testing.T) {
		out := ReportViewProperties(bad)
		assert.Contains(t, out, "narrows nothing")
		assert.Contains(t, out, "leaves the order untouched")
	})
}
