package identity

import (
	"github.com/anyproto/anytype-heart/core/block/importv2/persist"
	"github.com/anyproto/anytype-heart/core/block/importv2/resolve"
)

// The resolver drops createdInContextRef into matched parents only through
// this optional interface: a renamed method would silently disable it.
var _ resolve.MatchReporter = (*Service)(nil)

// Likewise the persister clears refs into rewritten parents only through
// persist.UpdateTargets.
var _ persist.UpdateTargets = (*Service)(nil)
