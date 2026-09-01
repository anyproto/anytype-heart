package wrapper

// tools_view_test.go — the update_view tool: the three channels alone and
// together (one atomic PATCH), the empty-call refusal, the sort grammar,
// the columns show/hide computation, and the ambiguity refusals.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testDataviewDoc is a set as the server serves it to the wrapper — under
// ?keys=name (D5), so column keys are display names: one dataview block,
// one view, three visible columns and one hidden.
const testDataviewDoc = `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"Name":"Tasks","setOf":["ot-task"]},"blocks":[` +
	`{"id":"dataview","type":"dataview",` +
	`"properties":[{"property":"Name","format":"text"},{"property":"Status","format":"select"},{"property":"Due date","format":"date"},{"property":"Priority","format":"select"}],` +
	`"views":[{"id":"viewAll1","name":"All",` +
	`"columns":[{"property":"Name"},{"property":"Status"},{"property":"Priority"},{"property":"Due date","hidden":true}]}]}]}`

// testTwoViewsDoc carries two views whose ids share a suffix, so a short
// view reference is ambiguous.
const testTwoViewsDoc = `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"Name":"Tasks","setOf":["ot-task"]},"blocks":[` +
	`{"id":"dataview","type":"dataview",` +
	`"properties":[{"property":"Name","format":"text"},{"property":"Status","format":"select"}],` +
	`"views":[` +
	`{"id":"aaaa0001","name":"All","columns":[{"property":"Name"}]},` +
	`{"id":"bbbb0001","name":"Board","type":"kanban","group_by":"Status","columns":[{"property":"Name"}]}]}]}`

func TestUpdateView(t *testing.T) {
	ctx := context.Background()

	t.Run("filter alone becomes one update_view PATCH", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1", Name: "Tasks", Type: "set"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		want := map[string]any{
			"op":  "update_view",
			"set": map[string]any{"filter": "done = false"},
		}

		// when
		result, err := fx.Run(ctx, "update_view", map[string]any{"object": "1", "filter": "done = false"})

		// then
		require.NoError(t, err)
		assert.Equal(t, `ok — "Tasks": filter set`, result.Text)
		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 1)
		assert.NotEmpty(t, sent[0].Header.Get("Idempotency-Key"), "mutations carry an Idempotency-Key")
		assert.Equal(t, want, firstOp(t, sent[0]))
		assert.Empty(t, fx.sent("GET /v2/spaces/space1/objects/bafyobj1"),
			"filter needs no read — the string is parsed server-side")
	})

	t.Run("sort alone parses the order-by grammar — multi-word names included", func(t *testing.T) {
		// given: "Due date desc" — the key carries a space, the LAST token is
		// the direction
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1", Name: "Tasks", Type: "set"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		want := map[string]any{
			"op": "update_view",
			"set": map[string]any{"sorts": []any{
				map[string]any{"property": "Due date", "direction": "desc"},
				map[string]any{"property": "Name", "direction": "asc"},
			}},
		}

		// when
		result, err := fx.Run(ctx, "update_view", map[string]any{"object": "1", "sort": "Due date desc, Name"})

		// then
		require.NoError(t, err)
		assert.Equal(t, `ok — "Tasks": sorted by Due date desc, Name asc`, result.Text,
			"the receipt echoes the normalized sort — the default direction made explicit")
		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 1)
		assert.Equal(t, want, firstOp(t, sent[0]))
	})

	t.Run("columns alone shows the listed keys and hides the rest", func(t *testing.T) {
		// given: Name/Status/Priority visible, Due date hidden — the column
		// list itself carries a multi-word name, which the comma split must
		// keep intact
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1", Name: "Tasks", Type: "set"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, testDataviewDoc)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		want := map[string]any{
			"op": "update_view",
			"columns": map[string]any{
				"Name":     map[string]any{"hidden": false},
				"Due date": map[string]any{"hidden": false},
				"Status":   map[string]any{"hidden": true},
				"Priority": map[string]any{"hidden": true},
			},
		}

		// when
		result, err := fx.Run(ctx, "update_view", map[string]any{"object": "1", "columns": "Name,Due date"})

		// then
		require.NoError(t, err)
		assert.Equal(t, `ok — "Tasks": columns: Name, Due date`, result.Text)
		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 1)
		assert.Equal(t, want, firstOp(t, sent[0]))
		reads := fx.sent("GET /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, reads, 1)
		assert.Equal(t, "name", reads[0].Query.Get("keys"),
			"the columns read asks for the name vocabulary — the spellings read here are the patches sent back")
	})

	t.Run("filter, sort and columns travel in ONE PATCH", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1", Name: "Tasks", Type: "set"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, testDataviewDoc)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		want := map[string]any{
			"op":    "update_view",
			"block": "dataview",
			"view":  "viewAll1",
			"set": map[string]any{
				"filter": "done = false",
				"sorts": []any{
					map[string]any{"property": "Due date", "direction": "desc"},
				},
			},
			"columns": map[string]any{
				"Name":     map[string]any{"hidden": false},
				"Status":   map[string]any{"hidden": true},
				"Priority": map[string]any{"hidden": true},
			},
		}

		// when
		result, err := fx.Run(ctx, "update_view", map[string]any{
			"object": "1", "block": "dataview", "view": "viewAll1",
			"filter": "done = false", "sort": "Due date desc", "columns": "Name",
		})

		// then: one atomic PATCH — the server validates every channel
		// against a private copy and applies all or nothing
		require.NoError(t, err)
		assert.Equal(t, `ok — "Tasks": filter set, sorted by Due date desc, columns: Name`, result.Text)
		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 1, "the three channels ride ONE PATCH")
		assert.Equal(t, want, firstOp(t, sent[0]))
	})

	t.Run("none of filter, sort, columns is a refusal that says so", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})

		// when
		_, err := fx.Run(ctx, "update_view", map[string]any{"object": "1"})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update_view needs filter, sort or columns")
		assert.Empty(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1"), "nothing reaches the server")
	})

	t.Run("a mistyped direction becomes part of the key — the server owns the refusal", func(t *testing.T) {
		// "due_date descending" cannot be told apart from a multi-word
		// property name ending in "descending", so the whole term travels
		// as the key and the server's did-you-mean names it — the price of
		// names with spaces (the old fixed word-count refusal is gone)
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1", Name: "Tasks", Type: "set"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 400,
			`{"status":400,"code":"validation_failed","message":"update_view rejected","issues":[{"path":"ops[0].set.sorts[0].property","message":"unknown property key \"due_date descending\""}]}`)

		_, err := fx.Run(ctx, "update_view", map[string]any{"object": "1", "sort": "due_date descending"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `unknown property key "due_date descending"`)
		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 1)
		op := firstOp(t, sent[0])
		set, _ := op["set"].(map[string]any)
		assert.Equal(t, []any{map[string]any{"property": "due_date descending", "direction": "asc"}}, set["sorts"])
	})

	t.Run("a sort key named twice is refused, not deduplicated", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})

		_, err := fx.Run(ctx, "update_view", map[string]any{"object": "1", "sort": "name asc, name desc"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `sort names "name" more than once`)
		assert.Empty(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1"))
	})

	t.Run("an invalid sort property changes nothing and answers in tool vocabulary", func(t *testing.T) {
		// given: the server's own §8.17 refusal — the whole op is validated
		// against a private copy, so a rejected key means NO channel applied
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1", Name: "Tasks", Type: "set"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 400,
			`{"status":400,"code":"validation_failed","message":"update_view rejected","issues":[{"path":"ops[0].set.sorts[0].property","message":"unknown property key \"due_dat\" — did you mean \"due_date\"?","hint":"list all with GET /v2/spaces/space1/properties, or create it with POST /v2/spaces/space1/properties"}]}`)

		// when: filter AND sort — one bad channel must sink both
		_, err := fx.Run(ctx, "update_view", map[string]any{
			"object": "1", "filter": "done = false", "sort": "due_dat desc",
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `did you mean "due_date"`)
		assert.Contains(t, err.Error(), "sort[0].property", "the op path is re-spelled to the tool's `sort` slot")
		assert.Contains(t, err.Error(), "run describe on the type", "the REST repair hint is re-spelled")
		assert.NotContains(t, err.Error(), "ops[0]")
		assert.NotContains(t, err.Error(), "GET /v2")
		assert.Len(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1"), 1, "no blind retry")
	})

	t.Run("an ambiguous view reference is refused with the candidates", func(t *testing.T) {
		// given: "0001" tails both view ids
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1", Name: "Tasks", Type: "set"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, testTwoViewsDoc)

		// when
		_, err := fx.Run(ctx, "update_view", map[string]any{"object": "1", "view": "0001", "columns": "name"})

		// then: refused wrapper-side, candidates listed, nothing sent
		require.Error(t, err)
		assert.Contains(t, err.Error(), `view "0001" matches several views`)
		assert.Contains(t, err.Error(), `aaaa0001 ("All")`)
		assert.Contains(t, err.Error(), `bbbb0001 ("Board")`)
		assert.Empty(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1"), "an ambiguous target is never guessed")
	})

	t.Run("view omitted with several views is refused listing them", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, testTwoViewsDoc)

		_, err := fx.Run(ctx, "update_view", map[string]any{"object": "1", "columns": "name"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "this dataview has 2 views — name one with view")
		assert.Contains(t, err.Error(), `aaaa0001 ("All")`)
		assert.Empty(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1"))
	})

	t.Run("a re-spelled column key pairs with its column instead of colliding", func(t *testing.T) {
		// given: the caller spells "Due date" as dueDate. The server
		// canonicalises both spellings to one key, so sending
		// dueDate:{hidden:false} AND "Due date":{hidden:true} would collapse
		// onto one column with the sort order of the raw keys deciding —
		// the call could hide the very column it was asked to show. The
		// pairing fold is FoldKeyTerm, so the api-key and camelCase guesses
		// both meet the document's display-name spelling.
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1", Name: "Tasks", Type: "set"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, testDataviewDoc)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		want := map[string]any{
			"op": "update_view",
			"columns": map[string]any{
				"Name":     map[string]any{"hidden": false},
				"Due date": map[string]any{"hidden": false},
				"Status":   map[string]any{"hidden": true},
				"Priority": map[string]any{"hidden": true},
			},
		}

		// when
		_, err := fx.Run(ctx, "update_view", map[string]any{"object": "1", "columns": "name,dueDate"})

		// then: the document's spelling is sent, once, as a show
		require.NoError(t, err)
		assert.Equal(t, want, firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0]))
	})

	t.Run("a duplicated column key is refused", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})

		_, err := fx.Run(ctx, "update_view", map[string]any{"object": "1", "columns": "name,status,name"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `column "name" is listed more than once`)
		assert.Empty(t, fx.sent("GET /v2/spaces/space1/objects/bafyobj1"), "refused before the read")
	})

	t.Run("an object with no dataview refuses the columns channel", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200,
			`{"formatVersion":"2.0","type":"page","properties":{"name":"Doc"},"blocks":[{"id":"e0001","type":"paragraph","text":"body"}]}`)

		_, err := fx.Run(ctx, "update_view", map[string]any{"object": "1", "columns": "name"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "this object has no dataview block")
		assert.Empty(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1"))
	})

	t.Run("dry run rides the query and the receipt", func(t *testing.T) {
		fx := newFixture(t)
		fx.DryRun = true
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1", Name: "Tasks", Type: "set"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200,
			`{"dry_run":true,"diff_stats":{"blocks_changed":1}}`)

		result, err := fx.Run(ctx, "update_view", map[string]any{"object": "1", "filter": "done = false"})

		require.NoError(t, err)
		assert.Equal(t, `dry run — "Tasks": would apply filter set`, result.Text)
		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 1)
		assert.Equal(t, "true", sent[0].Query.Get("dry_run"))
	})

	t.Run("@me in the filter resolves to the caller's participant id", func(t *testing.T) {
		// find's filter already resolves the sentinel wrapper-side; the same
		// filter string on update_view must behave identically
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1", Name: "Tasks", Type: "set"})
		fx.stub("GET /v2/spaces/space1/members/me", 200, `{"id":"_participant_space1_acc"}`)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "update_view", map[string]any{"object": "1", "filter": `assignee = "@me"`})

		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		set, _ := op["set"].(map[string]any)
		assert.Equal(t, `assignee = "_participant_space1_acc"`, set["filter"])
	})
}

// TestSortGrammar pins parseSortArg's grammar directly: the accepted forms
// (multi-word names included — the last token is the direction only when it
// spells one), the normalization, and each refusal.
func TestSortGrammar(t *testing.T) {
	t.Run("accepted forms normalize", func(t *testing.T) {
		for _, tc := range []struct {
			in   string
			want string // the echo — direction always explicit
		}{
			{"name", "name asc"},
			{"due_date desc", "due_date desc"},
			{"due_date DESC", "due_date desc"},
			{"due_date desc, name", "due_date desc, name asc"},
			{"due_date desc,,name asc,", "due_date desc, name asc"},
			// a display name with a space: the LAST token is the direction,
			// everything before it is the key
			{"Due date desc", "Due date desc"},
			{"Due date", "Due date asc"},
			{"Due   date  desc", "Due date desc"}, // sloppy spacing joins to single spaces
			{"Due date desc, Name", "Due date desc, Name asc"},
			// a property literally named like a direction is all key when it
			// stands alone
			{"desc", "desc asc"},
		} {
			sorts, echo, err := parseSortArg(tc.in)
			require.NoError(t, err, "input %q", tc.in)
			assert.Equal(t, tc.want, echo, "input %q", tc.in)
			assert.NotEmpty(t, sorts)
		}
	})

	t.Run("a mistyped direction is key, not a refusal", func(t *testing.T) {
		// indistinguishable from a name ending in that word — the server's
		// key resolution owns the did-you-mean
		sorts, echo, err := parseSortArg("due_date descending")
		require.NoError(t, err)
		assert.Equal(t, "due_date descending asc", echo)
		assert.Equal(t, []map[string]any{{"property": "due_date descending", "direction": "asc"}}, sorts)
	})

	t.Run("refusals", func(t *testing.T) {
		for _, tc := range []struct {
			in      string
			wantErr string
		}{
			{" , ", "sort names no property"},
			{"Due date desc, Due date", `sort names "Due date" more than once`},
			{"a,b,c,d,e,f,g,h,i,j,k", "at most 10"},
		} {
			_, _, err := parseSortArg(tc.in)
			require.Error(t, err, "input %q", tc.in)
			assert.Contains(t, err.Error(), tc.wantErr, "input %q", tc.in)
		}
	})
}
