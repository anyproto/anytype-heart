package anyblockjson

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestViewVocabularyMatchesEnumTables pins each exported §6.2 vocabulary list
// to the codec's own enum table: every exported name must be readable by the
// importer, and every name the exporter can emit must be exported. A new enum
// value added to a table without updating its list (or vice versa) fails here
// instead of silently splitting the vocabulary between the codec and the API
// surfaces that validate against it.
func TestViewVocabularyMatchesEnumTables(t *testing.T) {
	pin := func(t *testing.T, exported []string, has func(string) bool, tableSize int) {
		t.Helper()
		assert.Len(t, exported, tableSize, "exported list and enum table must be the same size")
		for _, name := range exported {
			assert.True(t, has(name), "exported name %q must be in the enum table", name)
		}
	}

	t.Run("view types", func(t *testing.T) {
		pin(t, ViewTypeNames(), viewTypeNames.has, len(viewTypeNames.toName))
	})
	t.Run("card sizes", func(t *testing.T) {
		pin(t, ViewCardSizeNames(), cardSizeNames.has, len(cardSizeNames.toName))
	})
	t.Run("list sizes", func(t *testing.T) {
		pin(t, ViewListSizeNames(), listSizeNames.has, len(listSizeNames.toName))
	})
	t.Run("column align", func(t *testing.T) {
		pin(t, ColumnAlignNames(), alignNames.has, len(alignNames.toName))
	})
	t.Run("column aggregation", func(t *testing.T) {
		pin(t, ColumnAggregationNames(), aggregationNames.has, len(aggregationNames.toName))
	})
}
