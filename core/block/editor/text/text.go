package text

import "github.com/anytypeio/go-anytype-middleware/core/block/simple/text"

type Text interface {
	UpdateTextBlocks(ids []string, showEvent bool, apply func(t text.Block) error) error
	Split(id string, pos int32) (blockId string, err error)
	Merge(firstId, secondId string) (err error)
}
