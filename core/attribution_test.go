package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
)

// newContext is where a local-API request's attribution crosses from the
// request context into the editor (docs/ApiChangeAttribution.md, carry 1):
// every ClientCommands method that edits through NewStateCtx depends on it.
func TestNewContextCarriesTheRequestAttribution(t *testing.T) {
	mw := &Middleware{}

	t.Run("a local-API request: the session context names the app", func(t *testing.T) {
		// given
		cctx := domain.CtxWithIntegrationName(context.Background(), "Raycast")
		want := "Raycast"

		// when
		got := mw.newContext(cctx).IntegrationName()

		// then: red if newContext stops copying the name — every API edit
		// made through a session context would be pushed unstamped
		assert.Equal(t, want, got)
	})

	t.Run("a gRPC call from an app: the session context names nothing", func(t *testing.T) {
		// when: a request context no API middleware touched
		got := mw.newContext(context.Background()).IntegrationName()

		// then: red if a name is ever defaulted
		assert.Equal(t, "", got)
	})

	t.Run("a child context still serves the same request", func(t *testing.T) {
		// given
		parent := mw.newContext(domain.CtxWithIntegrationName(context.Background(), "Raycast"))

		// when
		got := session.NewChildContext(parent).IntegrationName()

		// then: red if NewChildContext drops the name — the pending text
		// flush and other in-request child applies would be unstamped
		assert.Equal(t, "Raycast", got)
	})
}
