package csv

import (
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/process"
)

type Strategy interface {
	CreateObjects(path string, csvTable [][]string, useFirstRowForRelations bool, progress process.Progress) (string, []*converter.Snapshot, error)
}
