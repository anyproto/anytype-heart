package history

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/migration"
)

func ResetToVersion(sb smartblock.SmartBlock, st *state.State) error {
	if err := sb.ResetToVersion(st); err != nil {
		return fmt.Errorf("resetting smartblock to version: %w", err)
	}
	return migration.RunMigrations(sb, &smartblock.InitContext{
		State: sb.NewState(),
	})
}
