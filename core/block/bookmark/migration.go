package bookmark

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
)

type BlockMigrator interface {
	MigrateBlock(bm bookmark.Block) (err error)
}

func WithFixedBookmarks(bm BlockMigrator) func(st *state.State) {
	return func(st *state.State) {
		if err := migrateBlocks(bm, st); err != nil {
			log.Errorf("migrate bookmark blocks: %s", err)
		}
	}
}

func migrateBlocks(bm BlockMigrator, st *state.State) error {
	var migrateErr error
	err := st.Iterate(func(b simple.Block) bool {
		block, ok := b.(bookmark.Block)
		if !ok {
			return true
		}

		if migrateErr = bm.MigrateBlock(block); migrateErr != nil {
			return false
		}
		return true
	})
	if migrateErr != nil {
		return migrateErr
	}
	if err != nil {
		return fmt.Errorf("iterate: %w", err)
	}
	return nil
}
