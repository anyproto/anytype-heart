package wrapper

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newHostFixture builds a Host over the stub-server fixture's runner, so
// the host tests reuse the fixture's stubs and recorded requests.
func newHostFixture(t *testing.T) (*fixture, *Host) {
	fx := newFixture(t)
	return fx, &Host{runner: fx.Runner, store: fx.store}
}

func TestHostCall(t *testing.T) {
	t.Run("success carries the text and the machine shape", func(t *testing.T) {
		// given
		fx, host := newHostFixture(t)
		fx.stub("GET /v2/spaces", 200, `{"data":[{"id":"space1","name":"Work"}],"total":1,"has_more":false}`)

		// when
		got := host.Call(context.Background(), "spaces", nil)

		// then
		assert.False(t, got.IsError, got.Text)
		assert.Empty(t, got.Code)
		assert.Contains(t, got.Text, "Work — space1")
		require.IsType(t, spacesResult{}, got.JSON)
		assert.Equal(t, 1, got.JSON.(spacesResult).Total)
	})

	t.Run("a wrapper-side validation error is in-band", func(t *testing.T) {
		// given: find without its required space
		fx, host := newHostFixture(t)

		// when
		got := host.Call(context.Background(), "find", map[string]any{"query": "x"})

		// then
		assert.True(t, got.IsError)
		assert.Equal(t, CallCodeToolError, got.Code)
		assert.Contains(t, got.Text, `find needs "space"`)
		assert.Empty(t, fx.requests, "nothing reached the server")
	})

	t.Run("an unknown tool lists the tools", func(t *testing.T) {
		// given
		_, host := newHostFixture(t)

		// when
		got := host.Call(context.Background(), "nope", map[string]any{})

		// then
		assert.True(t, got.IsError)
		assert.Contains(t, got.Text, `unknown tool "nope"`)
		assert.Contains(t, got.Text, "spaces, find")
	})

	t.Run("a server refusal arrives as its C6 text", func(t *testing.T) {
		// given
		fx, host := newHostFixture(t)
		fx.stub("GET /v2/spaces", 500, `{"status":500,"code":"internal","message":"index unavailable","issues":[]}`)

		// when
		got := host.Call(context.Background(), "spaces", nil)

		// then
		assert.True(t, got.IsError)
		assert.Equal(t, "index unavailable", got.Text)
	})

	t.Run("an unreachable API says the account is not running", func(t *testing.T) {
		// given: a client pointed at a port nothing listens on
		client := NewClient("http://127.0.0.1:1", "")
		client.Backoff = func(int) time.Duration { return 0 }
		host := NewHost(client)

		// when
		got := host.Call(context.Background(), "spaces", nil)

		// then
		assert.True(t, got.IsError)
		assert.Contains(t, got.Text, "account is not running")
		assert.Contains(t, got.Text, "no change to the call will help")
	})

	t.Run("ResetSession forgets the handles and the working space", func(t *testing.T) {
		// given: a session with a handle
		fx, host := newHostFixture(t)
		require.NoError(t, fx.store.Save(&Session{Space: "space1", Handles: []Handle{{N: 1, Id: "obj1", Name: "Note"}}}))

		// when
		require.NoError(t, host.ResetSession())

		// then
		session, err := fx.store.Load()
		require.NoError(t, err)
		assert.Empty(t, session.Space)
		assert.Empty(t, session.Handles)
		got := host.Call(context.Background(), "read", map[string]any{"object": "1"})
		assert.True(t, got.IsError)
		assert.Contains(t, got.Text, "run find first")
	})
}
