package csv

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
)

type Strategy interface {
	CreateObjects(path string, csvTable [][]string) ([]string, []*converter.Snapshot, map[string][]*converter.Relation, error)
}
