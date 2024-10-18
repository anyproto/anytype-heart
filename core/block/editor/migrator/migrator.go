package migrator

import (
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("migrator")

type Migrator interface {
	Migrate(state *state.State)
}
