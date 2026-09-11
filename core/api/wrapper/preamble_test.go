package wrapper

// preamble_test.go — the run context (the app's space, time zone), space
// names as space arguments, the session's recents, and the preamble that
// renders them. The device run behind these: asked which space, the user
// answered "Weekend trip", and the model handed that on — with no space
// default and no name resolution, the call was refused twice.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

func TestResultBudget(t *testing.T) {
	ctx := context.Background()

	t.Run("a document past the budget drops whole blocks and says so", func(t *testing.T) {
		// given: a document whose blocks do not fit
		fx := newFixture(t)
		fx.SetContext(RunContext{MaxResultChars: 260})
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, longDoc(12))

		// when
		result, err := fx.Run(ctx, "read", map[string]any{"object": "1"})

		// then: still a document, and honest about what it holds
		require.NoError(t, err)
		var doc struct {
			Blocks []struct {
				Id string `json:"id"`
			} `json:"blocks"`
			Truncated string `json:"truncated"`
		}
		require.NoError(t, json.Unmarshal([]byte(result.Text), &doc), "the model must be handed parseable JSON, never a cut-off half")
		assert.Greater(t, len(doc.Blocks), 0, "at least the head of the document survives")
		assert.Less(t, len(doc.Blocks), 12)
		assert.Contains(t, doc.Truncated, fmt.Sprintf("showing %d of 12 blocks", len(doc.Blocks)))
		assert.Contains(t, doc.Truncated, "read with mode=outline")
		assert.NotContains(t, result.Text, "the result was cut to fit", "read cut itself, so the backstop had nothing to do")

		// and the app still gets the whole thing
		whole, err := json.Marshal(result.JSON)
		require.NoError(t, err)
		assert.Contains(t, string(whole), `"e0011"`, "the JSON channel carries the document the budget kept from the model")
	})

	t.Run("an outline read reports the count with no false repair", func(t *testing.T) {
		fx := newFixture(t)
		fx.SetContext(RunContext{MaxResultChars: 200})
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200,
			`{"outline":[{"indent":0,"id":"e0001","type":"paragraph","text":"one"},{"indent":0,"id":"e0002","type":"paragraph","text":"two"},{"indent":0,"id":"e0003","type":"paragraph","text":"three"},{"indent":0,"id":"e0004","type":"paragraph","text":"four"}]}`)

		result, err := fx.Run(ctx, "read", map[string]any{"object": "1", "mode": "outline"})

		require.NoError(t, err)
		assert.Contains(t, result.Text, "these are the first; the object has more")
		assert.NotContains(t, result.Text, "mode=outline to survey", "the repair must not name the mode the caller is already in")
	})

	t.Run("a document within the budget passes byte for byte", func(t *testing.T) {
		fx := newFixture(t)
		fx.SetContext(RunContext{MaxResultChars: 100000})
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, testFullDoc)

		result, err := fx.Run(ctx, "read", map[string]any{"object": "1"})

		require.NoError(t, err)
		assert.Equal(t, testFullDoc, result.Text)
	})

	t.Run("describe drops settable rows, never its guidance", func(t *testing.T) {
		// given: a space with far more properties than the budget fits
		fx := newFixture(t)
		fx.SetContext(RunContext{MaxResultChars: 700})
		rows := make([]v2model.PropertyRow, 0, 60)
		for i := 0; i < 60; i++ {
			rows = append(rows, v2model.PropertyRow{Key: fmt.Sprintf("prop_%02d", i), Name: fmt.Sprintf("Property number %02d", i), Format: "text"})
		}
		fx.stub("GET /v2/spaces/space1/types/Page", 200, pageTypeDoc)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(rows...))

		// when
		result, err := fx.Run(ctx, "describe", map[string]any{"space": "space1", "type": "Page"})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, "use these exact property names", "the guidance is what makes the rows usable — it outranks three more rows")
		assert.Contains(t, result.Text, "more in this space")
		assert.NotContains(t, result.Text, "the result was cut to fit", "describe cut itself")
		assert.LessOrEqual(t, len([]rune(result.Text)), 700)
	})

	t.Run("a select's options are trimmed under a budget and the note says where the rest are", func(t *testing.T) {
		options := make([]string, 0, 20)
		for i := 0; i < 20; i++ {
			options = append(options, fmt.Sprintf("Option%02d", i))
		}
		result := describeResult{
			Name: "Task",
			Properties: []describeProperty{{
				Key: "Status", Name: "Status", Format: "select", Options: options, OnType: true,
			}},
		}

		budgeted := describeText(result, 4000)
		unbudgeted := describeText(result, 0)

		assert.Contains(t, budgeted, "Option07")
		assert.NotContains(t, budgeted, "Option08", "a budgeted row names the first few options, not two dozen")
		assert.Contains(t, budgeted, `describe with options "Status"`)
		assert.Contains(t, unbudgeted, "Option19", "an unbudgeted delivery prints them all, as before")
	})

	t.Run("the backstop cuts anything else and says it cut", func(t *testing.T) {
		// given: a find whose rows run past the budget
		fx := newFixture(t)
		fx.SetContext(RunContext{MaxResultChars: 120})
		rows := make([]v2model.ObjectRow, 0, 20)
		for i := 0; i < 20; i++ {
			rows = append(rows, v2model.ObjectRow{Id: fmt.Sprintf("bafyobj%02d", i), Name: fmt.Sprintf("Result number %02d", i), Type: "page"})
		}
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(20, false, rows...))
		fx.stub("GET /v2/spaces/space1/types", 200, `{"data":[{"key":"page","name":"Page"}],"total":1,"offset":0,"limit":500,"has_more":false}`)

		// when
		result, err := fx.Run(ctx, "find", map[string]any{"space": "space1", "query": "result"})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, "the result was cut to fit")
		assert.LessOrEqual(t, len([]rune(result.Text)), 120+len([]rune("\n… the result was cut to fit this conversation — ask for less: a smaller limit, or read mode=outline")))
		session, _ := fx.store.Load()
		assert.Len(t, session.Handles, 20, "the cut is what the model READS; every row it could address is still addressable")
	})

	t.Run("a long name is shortened in a listing, only under a budget", func(t *testing.T) {
		long := strings.Repeat("a", 120)
		assert.Equal(t, long, clampName(long, 0))
		assert.Equal(t, strings.Repeat("a", 60)+"…", clampName(long, 2000))
	})

	t.Run("no budget anywhere leaves every result whole", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, longDoc(40))

		result, err := fx.Run(ctx, "read", map[string]any{"object": "1"})

		require.NoError(t, err)
		assert.Equal(t, longDoc(40), result.Text, "the CLI and MCP deliveries are unbudgeted")
	})

	t.Run("the in-process host carries a ceiling the client can lower", func(t *testing.T) {
		_, host := newHostFixture(t)
		assert.Equal(t, 0, host.runner.DefaultResultChars, "the fixture's runner is the bare one")

		client := NewClient("http://127.0.0.1:1", "")
		real := NewHost(client)
		assert.Equal(t, defaultHostResultChars, real.runner.DefaultResultChars,
			"a mobile host without a client-set budget still cannot be ended by one huge read")
		real.SetContext(RunContext{MaxResultChars: 2000})
		assert.Equal(t, 2000, real.runner.resultBudget(), "the client's window wins")
	})
}

// longDoc renders a served document with n paragraph blocks.
func longDoc(n int) string {
	blocks := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		blocks = append(blocks, fmt.Sprintf(`{"id":"e%04d","type":"paragraph","text":"block number %d"}`, i, i))
	}
	return `{"formatVersion":"2.0","type":"page","properties":{"name":"Long"},"blocks":[` + strings.Join(blocks, ",") + `]}`
}

func TestRefusalsStaySmall(t *testing.T) {
	// A refusal is never clamped — cutting a repair tip destroys the repair
	// — so the one refusal that lists the workspace has to be short by
	// construction. An account with a hundred spaces used to produce
	// kilobytes of it.
	fx := newFixture(t)
	rows := make([]v2model.SpaceRow, 0, 100)
	for i := 0; i < 100; i++ {
		rows = append(rows, v2model.SpaceRow{Id: fmt.Sprintf("space%02d", i), Name: fmt.Sprintf("Space number %02d", i)})
	}
	body, err := json.Marshal(v2model.ListResponse[v2model.SpaceRow]{Data: rows, Total: len(rows), Limit: 100})
	require.NoError(t, err)
	fx.stub("GET /v2/spaces", 200, string(body))

	_, err = fx.Run(context.Background(), "create", map[string]any{"name": "X", "type": "page"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Space number 00 — space00")
	assert.Contains(t, err.Error(), "… and 92 more — run spaces to see them all")
	assert.NotContains(t, err.Error(), "Space number 99")
	assert.Less(t, len(err.Error()), 400, "a refusal an unbudgeted model reads whole")
}
