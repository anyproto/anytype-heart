package anyblockjson

// typeProperties[].name is used only when the property has to be created.
// For a key that already exists — every bundled one — the existing
// name wins, so a document asking for a different label reads as working and
// silently does nothing.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func typeDoc(tp string) string {
	return `{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
		"typeProperties": [` + tp + `]}`
}

func TestValidate_BundledPropertyRenameWarns(t *testing.T) {
	for _, tc := range []struct{ key, want, bundled string }{
		{"description", "Summary", "Description"},
		{"createdInContext", "Parent page", "Created in context"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			var got []Issue
			require.NoError(t, ValidateWarn(
				[]byte(typeDoc(`{"key": "`+tc.key+`", "name": "`+tc.want+`"}`)),
				func(i Issue) { got = append(got, i) }))
			require.Len(t, got, 1)
			assert.Contains(t, got[0].Message, tc.bundled)
			assert.Contains(t, got[0].Path, "/typeProperties/0/name")
		})
	}
}

func TestValidate_PropertyNameNonTriggers(t *testing.T) {
	noWarn := func(t *testing.T, doc string) {
		t.Helper()
		var got []Issue
		require.NoError(t, ValidateWarn([]byte(doc), func(i Issue) { got = append(got, i) }))
		assert.Empty(t, got)
	}
	t.Run("custom key keeps its name", func(t *testing.T) {
		noWarn(t, typeDoc(`{"key": "verifiedUntil", "name": "Verified until", "format": "date"}`))
	})
	t.Run("bundled key with the bundled name", func(t *testing.T) {
		noWarn(t, typeDoc(`{"key": "description", "name": "Description"}`))
	})
	t.Run("bundled key with no name at all", func(t *testing.T) {
		noWarn(t, typeDoc(`{"key": "description"}`))
	})
}
