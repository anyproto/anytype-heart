package history

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/session"
)

func ResetToVersion(ctx session.Context, sb smartblock.SmartBlock, st *state.State) error {
	if err := sb.ResetToVersion(st); err != nil {
		return fmt.Errorf("resetting smartblock to version: %w", err)
	}
	migration.RunMigrations(sb, &smartblock.InitContext{
		Ctx:   ctx,
		State: sb.NewState(),
	})
	return nil
}
