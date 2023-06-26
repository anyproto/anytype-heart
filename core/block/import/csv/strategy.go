package csv

import (
	"github.com/anyproto/anytype-heart/core/block/import/converter"
)

type Strategy interface {
	CreateObjects(path string, fileName string, csvTable [][]string) ([]string, []*converter.Snapshot, error)
}
