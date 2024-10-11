package table

import "strings"

const TableCellSeparator = "-"

func IsTableCell(blockId string) bool {
	index := strings.Index(blockId, TableCellSeparator)
	lastIndex := strings.LastIndex(blockId, TableCellSeparator)

	if index != lastIndex || index == -1 || index == 0 || index == len(blockId)-1 {
		return false
	}
	return true
}
