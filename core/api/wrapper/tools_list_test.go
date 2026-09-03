package wrapper

// tools_list_test.go — what `read` answers for a Query and a Collection:
// the definition in the writing tools' own vocabulary, and the rows as
// numbered handles.

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
)

// testQueryDoc is a Query as the wrapper reads one: ?keys=name, so the type
// term and every property key are DISPLAY names ("Query", "Set of", "Due
// date"), and the view's filter is the structured §6.2 tree.
const testQueryDoc = `{"formatVersion":"2.0","etag":"e1","id":"bafyquery","type":"Query",` +
	`"properties":{"Name":"Open tasks","Set of":["bafytype"]},` +
	`"blocks":[{"id":"dataview","type":"dataview","properties":[` +
	`{"property":"Name","format":"text"},{"property":"Done","format":"checkbox"},{"property":"Due date","format":"date"}],` +
	`"views":[{"id":"view1","name":"All",` +
	`"filters":[{"property":"Done","condition":"equal","value":false},` +
	`{"property":"Due date","condition":"less","date_preset":"current_week"}],` +
	`"sorts":[{"property":"Due date","direction":"desc"}],` +
	`"columns":[{"property":"Name"}]}]}]}`

const testCollectionDoc = `{"formatVersion":"2.0","etag":"e1","id":"bafycoll","type":"Collection",` +
	`"properties":{"Name":"Reading list"},` +
	`"blocks":[{"id":"dataview","type":"dataview","is_collection":true,` +
	`"properties":[{"property":"Name","format":"text"}],` +
	`"views":[{"id":"view1","name":"All","sorts":[{"property":"Name"}],"columns":[{"property":"Name"}]}]}]}`

// testListTypesBody is the space's type listing: the two list kinds are
// keyed `set` and `collection` and NAMED Query and Collection — the whole
// reason detection cannot match on one spelling.
const testListTypesBody = `{"data":[{"key":"set","name":"Query"},{"key":"collection","name":"Collection"},` +
	`{"key":"task","name":"Task"},{"key":"book","name":"Book"}],"total":4,"offset":0,"limit":500,"has_more":false}`

// testTypeObjectDoc is the source type read the query's definition resolves
// its `type:` line from.
const testTypeObjectDoc = `{"formatVersion":"2.0","kind":"object_type","id":"bafytype","type":"Type","properties":{"Name":"Task"}}`

// lineAfter returns the remainder of the line starting with prefix.
func lineAfter(t *testing.T, text, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	t.Fatalf("no line starting with %q in:\n%s", prefix, text)
	return ""
}

func TestReadQuery(t *testing.T) {
	ctx := context.Background()

	// given
	fx := newFixture(t)
	fx.seedSession("space1", Handle{N: 1, Id: "bafyquery", Name: "Open tasks", Type: "set"})
	fx.stub("GET /v2/spaces/space1/objects/bafyquery", 200, testQueryDoc)
	fx.stub("GET /v2/spaces/space1/objects/bafytype", 200, testTypeObjectDoc)
	fx.stub("GET /v2/spaces/space1/types", 200, testListTypesBody)
	fx.stub("GET /v2/spaces/space1/queries/bafyquery/objects", 200, searchResponse(2, false,
		v2model.ObjectRow{Id: "bafyrow1", Name: "Fix the sink", Type: "task"},
		v2model.ObjectRow{Id: "bafyrow2", Name: "Call Mo", Type: "task"}))
	want := "Done = false AND Due_date < currentWeek()"

	// when
	result, err := fx.Run(ctx, "read", map[string]any{"object": "1"})

	// then: the definition speaks the writing tools' vocabulary
	require.NoError(t, err)
	assert.Contains(t, result.Text, "Open tasks (handle 1) — a Query")
	assert.Equal(t, "Task", lineAfter(t, result.Text, "type: "),
		"the source is named by its TYPE name — the spelling find and describe take")
	assert.Equal(t, want, lineAfter(t, result.Text, "filter: "))
	assert.Equal(t, "Due date desc", lineAfter(t, result.Text, "sort: "))

	// and the rows are addressable, numbered after the query's own handle
	assert.Contains(t, result.Text, "2. Fix the sink (Task)")
	assert.Contains(t, result.Text, "3. Call Mo (Task)")
	assert.Contains(t, result.Text, "2 rows")

	rows := fx.sent("GET /v2/spaces/space1/queries/bafyquery/objects")
	require.Len(t, rows, 1)
	assert.Equal(t, "view1", rows[0].Query.Get("view"),
		"the rows come from the view whose filter was printed — otherwise the heading describes a different set")
	assert.Equal(t, "name", rows[0].Query.Get("keys"))

	js, ok := result.JSON.(listReadResult)
	require.True(t, ok)
	assert.Equal(t, want, js.Definition.Filter)
	assert.Equal(t, 1, js.Handle)
	require.Len(t, js.Rows, 2)
	assert.Equal(t, 2, js.Rows[0].N)
	assert.NotEmpty(t, js.Document, "the machine channel still carries the document itself")

	t.Run("the printed filter is a filter the writing tools take", func(t *testing.T) {
		// the parser this asserts against IS the one the server runs on the
		// way in — what a tool prints must be accepted as what it takes
		_, parseErr := filterstring.Parse(js.Definition.Filter, filterstring.Options{})
		require.NoError(t, parseErr)

		fx.stub("PATCH /v2/spaces/space1/objects/bafyquery", 200, editOKBody)
		_, err := fx.Run(ctx, "update_view", map[string]any{"object": "1", "filter": js.Definition.Filter})

		require.NoError(t, err)
		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyquery")
		require.Len(t, sent, 1)
		set, _ := firstOp(t, sent[0])["set"].(map[string]any)
		assert.Equal(t, want, set["filter"], "the string read back out is the string that goes back in")
	})

	t.Run("the sort is update_view's own grammar", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyquery"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyquery", 200, editOKBody)

		_, err := fx.Run(ctx, "update_view", map[string]any{"object": "1", "sort": "Due date desc"})

		require.NoError(t, err)
		set, _ := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyquery")[0])["set"].(map[string]any)
		assert.Equal(t, []any{map[string]any{"property": "Due date", "direction": "desc"}}, set["sorts"])
	})
}

func TestReadCollection(t *testing.T) {
	ctx := context.Background()

	t.Run("members are numbered and the collection keeps its own handle", func(t *testing.T) {
		// given: the collection is handle 2 of a previous find
		fx := newFixture(t)
		fx.seedSession("space1",
			Handle{N: 1, Id: "bafyother", Name: "Something else"},
			Handle{N: 2, Id: "bafycoll", Name: "Reading list", Type: "collection"})
		fx.stub("GET /v2/spaces/space1/objects/bafycoll", 200, testCollectionDoc)
		fx.stub("GET /v2/spaces/space1/types", 200, testListTypesBody)
		fx.stub("GET /v2/spaces/space1/collections/bafycoll/objects", 200, searchResponse(2, false,
			v2model.ObjectRow{Id: "bafybook1", Name: "Dune", Type: "book"},
			v2model.ObjectRow{Id: "bafybook2", Name: "Solaris", Type: "book"}))

		// when
		result, err := fx.Run(ctx, "read", map[string]any{"object": "2"})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, "Reading list (handle 2) — a Collection")
		assert.Contains(t, result.Text, "a manual list")
		assert.NotContains(t, result.Text, "filter:", "a collection's membership is not a filter")
		assert.Equal(t, "Name asc", lineAfter(t, result.Text, "sort: "))
		assert.Contains(t, result.Text, "3. Dune (Book)")
		assert.Contains(t, result.Text, "4. Solaris (Book)")

		// the rows are appended, so every earlier number still means what it did
		session, err := fx.store.Load()
		require.NoError(t, err)
		require.Len(t, session.Handles, 4)
		coll, ok := session.handle(2)
		require.True(t, ok)
		assert.Equal(t, "bafycoll", coll.Id, "the list's own handle survives the read that used it")
		row, ok := session.handle(3)
		require.True(t, ok)
		assert.Equal(t, "bafybook1", row.Id)

		// and it still addresses the collection on the next call
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafycoll", 200, editOKBody)
		_, err = fx.Run(ctx, "set_properties", map[string]any{"object": "2", "set": map[string]any{"Name": "Later"}})
		require.NoError(t, err)
		assert.Len(t, fx.sent("PATCH /v2/spaces/space1/objects/bafycoll"), 1)
	})

	t.Run("a list read by id is numbered first, and its rows behind it", func(t *testing.T) {
		// given: a working space, nothing numbered
		fx := newFixture(t)
		fx.seedSession("space1")
		fx.stub("GET /v2/spaces/space1/objects/bafycoll", 200, testCollectionDoc)
		fx.stub("GET /v2/spaces/space1/types", 200, testListTypesBody)
		fx.stub("GET /v2/spaces/space1/collections/bafycoll/objects", 200, searchResponse(1, false,
			v2model.ObjectRow{Id: "bafybook1", Name: "Dune", Type: "book"}))

		// when
		result, err := fx.Run(ctx, "read", map[string]any{"object": "bafycoll"})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, "Reading list (handle 1) — a Collection")
		assert.Contains(t, result.Text, "2. Dune (Book)")
	})
}

func TestReadListFallsBackToTheDocument(t *testing.T) {
	ctx := context.Background()

	t.Run("an ordinary page carrying a dataview is not a list", func(t *testing.T) {
		// given: an inline query embedded in a page — the dataview block is
		// necessary but not sufficient, the type term decides
		const pageWithDataview = `{"formatVersion":"2.0","type":"Page","properties":{"Name":"Notes"},` +
			`"blocks":[{"id":"e0001","type":"dataview","properties":[{"property":"Name","format":"text"}],` +
			`"views":[{"id":"v1","name":"All","columns":[{"property":"Name"}]}]}]}`
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafypage"})
		fx.stub("GET /v2/spaces/space1/objects/bafypage", 200, pageWithDataview)
		fx.stub("GET /v2/spaces/space1/types", 200, testListTypesBody)

		// when
		result, err := fx.Run(ctx, "read", map[string]any{"object": "1"})

		// then
		require.NoError(t, err)
		assert.Equal(t, pageWithDataview, result.Text)
		assert.Empty(t, fx.sent("GET /v2/spaces/space1/queries/bafypage/objects"))
	})

	t.Run("a rows listing that fails serves the document rather than failing the read", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyquery"})
		fx.stub("GET /v2/spaces/space1/objects/bafyquery", 200, testQueryDoc)
		fx.stub("GET /v2/spaces/space1/types", 200, testListTypesBody)
		fx.stub("GET /v2/spaces/space1/queries/bafyquery/objects", 500,
			`{"status":500,"code":"internal_error","message":"boom"}`)

		// when
		result, err := fx.Run(ctx, "read", map[string]any{"object": "1"})

		// then: an enrichment must never break the read it decorates
		require.NoError(t, err)
		assert.Equal(t, testQueryDoc, result.Text)
	})

	t.Run("mode=outline stays the cheap survey", func(t *testing.T) {
		// given: the outline envelope carries no dataview to read a
		// definition out of
		const outline = `{"formatVersion":"2.0","type":"Query","outline":[{"indent":0,"id":"dataview","type":"dataview","text":""}]}`
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyquery"})
		fx.stub("GET /v2/spaces/space1/objects/bafyquery", 200, outline)

		// when
		result, err := fx.Run(ctx, "read", map[string]any{"object": "1", "mode": "outline"})

		// then
		require.NoError(t, err)
		assert.Equal(t, outline, result.Text)
		assert.Empty(t, fx.sent("GET /v2/spaces/space1/queries/bafyquery/objects"))
	})
}

func TestRenderFilterString(t *testing.T) {
	dateSeconds := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC).Unix()

	t.Run("a group renders with its operator and parentheses", func(t *testing.T) {
		// given
		nodes := []servedFilter{{
			Operator: "or",
			Filters: []servedFilter{
				{Property: "Status", Condition: "in", Value: json.RawMessage(`["In progress","Blocked"]`)},
				{Property: "Assignee", Condition: "not_empty"},
			},
		}}
		want := `(Status IN ("In progress", "Blocked") OR Assignee IS NOT EMPTY)`

		// when
		got, err := renderFilterString(nodes, nil)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("a date property's stored seconds render as the date the syntax takes", func(t *testing.T) {
		// given
		nodes := []servedFilter{{
			Property: "Due date", Condition: "less",
			Value: json.RawMessage(strconv.FormatInt(dateSeconds, 10)),
		}}
		want := `Due_date < "2026-08-07T00:00:00Z"`

		// when
		got, err := renderFilterString(nodes, map[string]string{"Due date": "date"})

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("a counting preset carries its day count", func(t *testing.T) {
		nodes := []servedFilter{{
			Property: "Last modified date", Condition: "greater",
			DatePreset: "number_of_days_ago", Value: json.RawMessage(`7`),
		}}

		got, err := renderFilterString(nodes, nil)

		require.NoError(t, err)
		assert.Equal(t, "Last_modified_date > daysAgo(7)", got)
	})

	t.Run("a name no identifier can spell is reported, not printed", func(t *testing.T) {
		// given: the compact syntax's keys are identifiers, and no escape
		// for "C++" exists by design — find's own description says so
		nodes := []servedFilter{{Property: "C++", Condition: "equal", Value: json.RawMessage(`true`)}}

		// when
		_, err := renderFilterString(nodes, nil)

		// then
		require.Error(t, err)
	})

	t.Run("every rendering is parsed before it is served", func(t *testing.T) {
		for _, nodes := range [][]servedFilter{
			{{Property: "Done", Condition: "equal", Value: json.RawMessage(`false`)}},
			{{Property: "Tags", Condition: "all_in", Value: json.RawMessage(`["urgent","q3"]`)}},
			{{Property: "Name", Condition: "contains", Value: json.RawMessage(`"report"`)}},
			{{Property: "Priority", Condition: "greater_or_equal", Value: json.RawMessage(`3`)}},
			{{Property: "Due date", Condition: "less", DatePreset: "current_week"}},
			{{Property: "Assignee", Condition: "empty"}},
		} {
			got, err := renderFilterString(nodes, nil)
			require.NoError(t, err)
			_, parseErr := filterstring.Parse(got, filterstring.Options{})
			assert.NoError(t, parseErr, "rendered %q", got)
		}
	})
}
