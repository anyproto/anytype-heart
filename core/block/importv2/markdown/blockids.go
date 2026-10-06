package markdown

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/simple/table"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// stabilizeBlockIds replaces anymark's random block ids (bson.NewObjectId)
// with ids derived from the page and the block's position, so converting the
// same file twice yields the same blocks (converter contract rule 5).
// Other objects point at these ids — a file's or a subpage's
// createdInContextRef names the block linking it — and a resumed run
// re-converts pages a previous incarnation already persisted: random ids
// would leave those refs naming blocks that do not exist.
//
// Table cells are the one id with structure (`rowId-colId`, parsed by the
// table editor), so they are rebuilt from their row's and column's new ids.
func stabilizeBlockIds(pageName string, blocks []*model.Block) {
	renamed := make(map[string]string, len(blocks))
	for i, b := range blocks {
		if !strings.Contains(b.Id, table.TableCellSeparator) {
			renamed[b.Id] = positionalBlockId(pageName, i)
		}
	}
	for i, b := range blocks {
		if _, done := renamed[b.Id]; done {
			continue
		}
		rowId, colId, _ := strings.Cut(b.Id, table.TableCellSeparator)
		newRow, rowOk := renamed[rowId]
		newCol, colOk := renamed[colId]
		if rowOk && colOk {
			renamed[b.Id] = newRow + table.TableCellSeparator + newCol
		} else {
			renamed[b.Id] = positionalBlockId(pageName, i)
		}
	}
	for _, b := range blocks {
		b.Id = renamed[b.Id]
		// A child id naming no block is dropped: anymark leaves one in the
		// last table cell when a list follows a table, and kept as-is it
		// would be the one random id left in the page.
		children := b.ChildrenIds[:0]
		for _, child := range b.ChildrenIds {
			if newId, ok := renamed[child]; ok {
				children = append(children, newId)
			}
		}
		b.ChildrenIds = children
	}
}

// positionalBlockId is bson-id shaped (24 hex chars, no cell separator).
func positionalBlockId(pageName string, position int) string {
	sum := sha256.Sum256([]byte(pageName + "\x00" + strconv.Itoa(position)))
	return hex.EncodeToString(sum[:12])
}
