package anyblockjson

// viewvocab.go exports the §6.2 dataview view vocabulary at fragment
// granularity — the enum name lists a surface editing views needs for
// validation and error text (API v2's updateView op, APIV2.md §8.17). The
// lists are the single source the API layer consumes, so the op's allowed
// values cannot drift from what the codec actually reads and writes; a test
// pins each list against its enum table.

// ViewTypeNames lists the §6.2 view types in canonical order. `table` is the
// default (export omits it).
func ViewTypeNames() []string {
	return []string{"table", "list", "gallery", "kanban", "calendar", "graph"}
}

// ViewCardSizeNames lists the §6.2 cardSize values. `small` is the default.
func ViewCardSizeNames() []string {
	return []string{"small", "medium", "large"}
}

// ViewListSizeNames lists the §6.2 listSize values. `compact` is the default.
func ViewListSizeNames() []string {
	return []string{"compact", "regular"}
}

// ColumnAlignNames lists the §6.2 column align values.
func ColumnAlignNames() []string {
	return []string{"left", "center", "right", "justify"}
}

// ColumnAggregationNames lists the §6.2 column aggregation values. Absent
// means none.
func ColumnAggregationNames() []string {
	return []string{
		"count", "countValue", "countDistinct", "countEmpty", "countNotEmpty",
		"percentEmpty", "percentNotEmpty", "sum", "average", "median", "min",
		"max", "range",
	}
}
