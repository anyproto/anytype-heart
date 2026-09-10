package server

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

// TestWrapperRoutesRegistered walks the task-tool wrapper's hard-coded
// route templates against the gin router's registered routes: the wrapper
// suite stubs its own paths, so a renamed /v2 route would otherwise stay
// green there while every tool 404s in production.
func TestWrapperRoutesRegistered(t *testing.T) {
	fx := newV2ServerFixture(t)
	engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})

	registered := map[string]bool{}
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = true
	}

	for _, rt := range wrapper.RouteTemplates() {
		assert.True(t, registered[rt.Method+" "+rt.Path],
			fmt.Sprintf("the wrapper calls %s %s but the router does not register it", rt.Method, rt.Path))
	}
}
