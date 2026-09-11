package wrapper

// preamble_test.go — the run context (the app's space, time zone), space
// names as space arguments, the session's recents, and the preamble that
// renders them. The device run behind these: asked which space, the user
// answered "Weekend trip", and the model handed that on — with no space
// default and no name resolution, the call was refused twice.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

func TestRunContextSpaceDefault(t *testing.T) {
	ctx := context.Background()

	t.Run("the app's space is the default when no find has set one", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.SetContext(RunContext{Space: "hpujze"})
		fx.stub("POST /v2/spaces/hpujze/objects", 200, `{"id":"bafynew","type":"page"}`)

		// when: the device run's call — no space at all
		result, err := fx.Run(ctx, "create", map[string]any{"name": "Trip to Copenhagen", "type": "page", "properties": map[string]any{}})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, "created bafynew")
		require.Len(t, fx.sent("POST /v2/spaces/hpujze/objects"), 1)
	})

	t.Run("an empty space string is no space", func(t *testing.T) {
		fx := newFixture(t)
		fx.SetContext(RunContext{Space: "hpujze"})
		fx.stub("POST /v2/spaces/hpujze/objects", 200, `{"id":"bafynew","type":"page"}`)

		_, err := fx.Run(ctx, "create", map[string]any{"space": "", "name": "X", "type": "page"})

		require.NoError(t, err)
		require.Len(t, fx.sent("POST /v2/spaces/hpujze/objects"), 1)
	})

	t.Run("a find's working space wins over the app's", func(t *testing.T) {
		fx := newFixture(t)
		fx.SetContext(RunContext{Space: "hpujze"})
		fx.seedSession("xjwg44", Handle{N: 1, Id: "obj1"})
		fx.stub("POST /v2/spaces/xjwg44/objects", 200, `{"id":"bafynew","type":"page"}`)

		_, err := fx.Run(ctx, "create", map[string]any{"name": "X", "type": "page"})

		require.NoError(t, err)
		require.Len(t, fx.sent("POST /v2/spaces/xjwg44/objects"), 1)
	})

	t.Run("the context survives a session reset", func(t *testing.T) {
		fx, host := newHostFixture(t)
		host.SetContext(RunContext{Space: "hpujze", TimeZone: "Europe/Berlin"})
		require.NoError(t, host.ResetSession())
		assert.Equal(t, RunContext{Space: "hpujze", TimeZone: "Europe/Berlin"}, host.Context())
		_ = fx
	})

	t.Run("relative dates follow the context time zone", func(t *testing.T) {
		// given: 15:00 UTC on 6 August is already 7 August in Auckland
		fx := newFixture(t)
		fx.SetContext(RunContext{TimeZone: "Pacific/Auckland"})

		// when
		got, ok := resolveRelativeDate("today", fx.nowLocal())

		// then
		require.True(t, ok)
		assert.Equal(t, "2026-08-07T00:00:00+12:00", got)
		fx.SetContext(RunContext{TimeZone: "Not/AZone"})
		assert.Equal(t, fx.now, fx.nowLocal(), "an unknown zone falls back to the process clock")
	})
}

func TestSpaceNameResolves(t *testing.T) {
	ctx := context.Background()

	t.Run("an exact name, a partial name, and a spaces row all reach the id", func(t *testing.T) {
		for _, spelling := range []string{"Weekend Trips", "weekend trips", "Weekend trip", "Trips", "Weekend Trips — hpujze", "hpujze"} {
			fx := newFixture(t)
			fx.stub("GET /v2/spaces", 200, twoSpacesBody)
			fx.stub("POST /v2/spaces/hpujze/objects", 200, `{"id":"bafynew","type":"page"}`)

			_, err := fx.Run(ctx, "create", map[string]any{"space": spelling, "name": "X", "type": "page"})

			require.NoError(t, err, spelling)
			require.Len(t, fx.sent("POST /v2/spaces/hpujze/objects"), 1, spelling)
		}
	})

	t.Run("find takes a space name too", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("GET /v2/spaces", 200, twoSpacesBody)
		fx.stub("POST /v2/spaces/xjwg44/search", 200, searchResponse(0, false))

		_, err := fx.Run(ctx, "find", map[string]any{"space": "soft motion", "query": "x"})

		require.NoError(t, err)
		require.Len(t, fx.sent("POST /v2/spaces/xjwg44/search"), 1)
	})

	t.Run("several matches refuse and name them", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("GET /v2/spaces", 200, twoSpacesBody)

		_, err := fx.Run(ctx, "create", map[string]any{"space": "t", "name": "X", "type": "page"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `space "t" matches several spaces (Soft motion — xjwg44; Weekend Trips — hpujze)`)
		assert.Empty(t, fx.sent("POST /v2/spaces/hpujze/objects"))
	})

	t.Run("a name matching nothing passes through to the server's refusal", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("GET /v2/spaces", 200, twoSpacesBody)
		fx.stub("POST /v2/spaces/Nowhere/objects", 404, `{"status":404,"code":"not_found","message":"space \"Nowhere\" not found","issues":[{"path":"space_id","message":"no space with this id is open on this account","hint":"list them with GET /v2/spaces","see_also":[{"op":"list_spaces"}]}]}`)

		_, err := fx.Run(ctx, "create", map[string]any{"space": "Nowhere", "name": "X", "type": "page"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "list them with the `spaces` tool")
	})
}

func TestRecentsAndPreamble(t *testing.T) {
	ctx := context.Background()

	t.Run("find, describe and create record what they touched", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.stub("POST /v2/search", 200, searchResponse(2, false,
			v2model.ObjectRow{Id: "bafytrip", Name: "Prague trip", Type: "trip", SpaceId: "hpujze"},
			v2model.ObjectRow{Id: "bafynote", Name: "Notes", Type: "note", SpaceId: "xjwg44"},
		))
		fx.stub("GET /v2/spaces", 200, twoSpacesBody)
		fx.stub("GET /v2/spaces/hpujze/types", 200, `{"data":[{"key":"trip","name":"Trip"}],"total":1,"offset":0,"limit":500,"has_more":false}`)
		fx.stub("GET /v2/spaces/xjwg44/types", 200, `{"data":[{"key":"note","name":"Note"},{"key":"page","name":"Page"}],"total":2,"offset":0,"limit":500,"has_more":false}`)
		fx.stub("POST /v2/spaces/xjwg44/objects", 200, `{"id":"bafynew","type":"page"}`)

		// when
		_, err := fx.Run(ctx, "find", map[string]any{"query": "prague"})
		require.NoError(t, err)
		_, err = fx.Run(ctx, "create", map[string]any{"space": "xjwg44", "name": "X", "type": "page"})
		require.NoError(t, err)

		// then: most recent first, no duplicates
		session, _ := fx.store.Load()
		assert.Equal(t, []string{"xjwg44", "hpujze"}, session.RecentSpaces)
		assert.Equal(t, []string{"Page", "Note", "Trip"}, session.RecentTypes)
	})

	t.Run("the preamble names the date, the current space, the count and the recents", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.SetContext(RunContext{Space: "hpujze", TimeZone: "Europe/Berlin"})
		fx.stub("GET /v2/spaces", 200, twoSpacesBody)
		require.NoError(t, fx.store.Save(&Session{RecentSpaces: []string{"hpujze", "xjwg44"}, RecentTypes: []string{"Trip", "Page"}}))
		want := "Today is Thursday, 6 August 2026.\n" +
			"Current space: Weekend Trips (hpujze) — describe, create and create_type use it when given no space.\n" +
			"2 spaces in the account; find with no space searches all of them, spaces lists them.\n" +
			"Recently used spaces: Soft motion (xjwg44).\n" +
			"Recently used types: Trip, Page."

		// when
		got, err := fx.Preamble(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.Less(t, len(got), 400, "the preamble stays small: it is context on a 4k-token model")
	})

	t.Run("with no context and no session the preamble is the date and the count", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("GET /v2/spaces", 200, twoSpacesBody)

		got, err := fx.Preamble(ctx)

		require.NoError(t, err)
		assert.Equal(t, "Today is Thursday, 6 August 2026.\n2 spaces in the account; find with no space searches all of them, spaces lists them.", got)
	})

	t.Run("a failed space listing keeps the date", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("GET /v2/spaces", 500, `{"status":500,"code":"internal","message":"index unavailable","issues":[]}`)

		got, err := fx.Preamble(ctx)

		require.NoError(t, err)
		assert.Equal(t, "Today is Thursday, 6 August 2026.", got)
	})

	t.Run("the recents are capped and dedup at the front", func(t *testing.T) {
		list := []string{}
		for _, v := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "a"} {
			list = pushRecent(list, v)
		}
		assert.Equal(t, []string{"a", "i", "h", "g", "f", "e", "d", "c"}, list)
	})
}
