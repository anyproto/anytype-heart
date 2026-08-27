package anyblockjson

// viewvocab.go exports the §6.2 dataview view vocabulary at fragment
// granularity — the enum name lists a surface editing views needs for
// validation and error text (the API's updateView op). The
// lists are the single source the API layer consumes, so the op's allowed
// values cannot drift from what the codec actually reads and writes; a test
// pins each list against its enum table.

// ViewTypeNames lists the §6.2 view types in canonical order. `table` is the
// default (export omits it).
func ViewTypeNames() []string {
	return []string{"table", "list", "gallery", "kanban", "calendar", "graph"}
}

// ViewCardSizeNames lists the §6.2 card_size values. `small` is the default.
func ViewCardSizeNames() []string {
	return []string{"small", "medium", "large"}
}

// ViewListSizeNames lists the §6.2 list_size values. `compact` is the default.
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
		"count", "count_value", "count_distinct", "count_empty", "count_not_empty",
		"percent_empty", "percent_not_empty", "sum", "average", "median", "min",
		"max", "range",
	}
}
