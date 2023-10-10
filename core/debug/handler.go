//go:build !anydebug

package debug

import "github.com/anyproto/any-sync/app"

func (d *debug) initHandlers(a *app.App) {
	// no-op
}
