package bookmark

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/session"
)

type BlockMigrator interface {
	MigrateBlock(ctx session.Context, bm bookmark.Block) (err error)
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
		_, ok := b.(bookmark.Block)
		if !ok {
			return true
		}

		block := st.Get(b.Model().Id).(bookmark.Block)
		if migrateErr = bm.MigrateBlock(st.Context(), block); migrateErr != nil {
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
