package dependencies

import "github.com/anyproto/any-sync/app"

// SyncDeferrer allows per-space builders to check whether sync should be
// deferred (because a higher-priority space is syncing first) and to
// register a deferred StartSync callback.
type SyncDeferrer interface {
	app.Component
	// ShouldDeferSync returns true if the given space should defer its
	// StartSync call (i.e., it is not the currently active space).
	ShouldDeferSync(spaceId string) bool
	// DeferSync registers a StartSync callback that will be invoked later
	// when the active space is caught up or a timeout expires.
	DeferSync(spaceId string, startSync func())
}
