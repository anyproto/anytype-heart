package wrapper

// tools_type_test.go — create_type. Two things are asserted everywhere here
// and nowhere else in the suite: that a refusal happens BEFORE any write
// (the type cannot be deleted, so a refusal that fires after the mints is a
// permanent mess), and that the option-bearing properties are created
// through the route that actually creates options.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// requestOrder returns every recorded request as "METHOD path", in the order
// the tool sent them. create_type's whole safety argument is an ORDER — the
// name check first, the property mints second, the type create last — so the
// order is the thing to assert.
func requestOrder(fx *fixture) []string {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	out := make([]string, 0, len(fx.requests))
	for _, r := range fx.requests {
		out = append(out, r.Method+" "+r.Path)
	}
	return out
}

// typeDefinitionsSent digs the property_definitions out of a recorded type
// document.
func typeDefinitionsSent(t *testing.T, r recordedRequest) []map[string]any {
	t.Helper()
	body := bodyJSON(t, r)
	settings, ok := body["type_settings"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := settings["property_definitions"].([]any)
	require.True(t, ok, "type_settings carries property_definitions")
	defs := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		def, ok := entry.(map[string]any)
		require.True(t, ok)
		defs = append(defs, def)
	}
	return defs
}

func TestParseTypeProperties(t *testing.T) {
	t.Run("several properties parse in order", func(t *testing.T) {
		// given
		ddl := "Cook time: number, Source: url, Notes: text"
		want := []typePropertyDecl{
			{Name: "Cook time", Format: "number"},
			{Name: "Source", Format: "url"},
			{Name: "Notes", Format: "text"},
		}

		// when
		got, err := parseTypeProperties(ddl)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("the split is paren-aware: both levels are comma-separated", func(t *testing.T) {
		// given — the property list and the option list each hold commas, and
		// one option name holds one too; a depth-blind split reads this as
		// seven properties, five of them nonsense
		ddl := `Rating: select(Low, Medium, High), Course: multi_select(Starter, light, Main), Cook time: number`
		want := []typePropertyDecl{
			{Name: "Rating", Format: "select", Options: []string{"Low", "Medium", "High"}},
			{Name: "Course", Format: "multi_select", Options: []string{"Starter", "light", "Main"}},
			{Name: "Cook time", Format: "number"},
		}

		// when
		got, err := parseTypeProperties(ddl)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("an option name may carry its own parentheses", func(t *testing.T) {
		// given
		ddl := "Rating: select(Low (rare), High)"
		want := []typePropertyDecl{{Name: "Rating", Format: "select", Options: []string{"Low (rare)", "High"}}}

		// when
		got, err := parseTypeProperties(ddl)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("an unknown format is refused naming the offender and the valid set", func(t *testing.T) {
		// given
		ddl := "Cook time: number, Rating: rating"

		// when
		_, err := parseTypeProperties(ddl)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `property "Rating": unknown format "rating"`)
		for _, format := range typePropertyFormats {
			assert.Contains(t, err.Error(), format, "the refusal lists every valid format")
		}
	})

	t.Run("a format spelling folds by case and separator only", func(t *testing.T) {
		// given
		ddl := "Rating: Select(Low), Course: Multi-Select(Main), Cooked: CHECKBOX"

		// when
		got, err := parseTypeProperties(ddl)

		// then
		require.NoError(t, err)
		require.Len(t, got, 3)
		assert.Equal(t, "select", got[0].Format)
		assert.Equal(t, "multi_select", got[1].Format)
		assert.Equal(t, "checkbox", got[2].Format)
	})

	t.Run("options on a non-select are refused with the reason", func(t *testing.T) {
		// given
		ddl := "Weight: number(Heavy, Light)"

		// when
		_, err := parseTypeProperties(ddl)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `property "Weight": options are only meaningful on select and multi_select, not number`)
		assert.Contains(t, err.Error(), "drop the parentheses", "a refusal names its repair")
	})

	t.Run("unbalanced parentheses are refused, not guessed at", func(t *testing.T) {
		// given/when/then
		_, err := parseTypeProperties("Rating: select(Low, High")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unclosed "("`)

		_, err = parseTypeProperties("Rating: select Low, High)")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `")" with no matching "("`)
	})

	t.Run("an entry with no format is refused showing the form", func(t *testing.T) {
		// given/when
		_, err := parseTypeProperties("Cook time")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `property "Cook time" names no format`)
		assert.Contains(t, err.Error(), "Cook time: number")
	})

	t.Run("two spellings of one property are refused", func(t *testing.T) {
		// given/when
		_, err := parseTypeProperties("Cook time: number, cook_time: text")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `declares "Cook time" and "cook_time"`)
		assert.Contains(t, err.Error(), "keep one")
	})

	t.Run("one option named twice is refused", func(t *testing.T) {
		// given/when
		_, err := parseTypeProperties("Rating: select(Low, Low)")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `lists option "Low" twice`)
	})

	t.Run("an empty list is no properties, not an error", func(t *testing.T) {
		// given/when
		got, err := parseTypeProperties("")

		// then
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

func TestCreateType(t *testing.T) {
	ctx := context.Background()

	t.Run("a type with several properties rides one document", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(
			v2model.PropertyRow{Key: "source", Name: "Source", Format: "url"}))
		fx.stub("POST /v2/spaces/space1/types", 200, `{"key":"cookbook_entry","dry_run":true}`)
		fx.stub("POST /v2/spaces/space1/types", 201,
			`{"id":"bafyreig5vvi7","key":"cookbook_entry","etag":"c18d5afd"}`)
		want := []map[string]any{
			{"property": "Cook time", "name": "Cook time", "format": "number"},
			{"property": "source", "name": "Source", "format": "url"},
		}

		// when
		result, err := fx.Run(ctx, "create_type", map[string]any{
			"space": "space1", "name": "Cookbook entry",
			"properties": "Cook time: number, Source: url",
		})

		// then
		require.NoError(t, err)
		sent := fx.sent("POST /v2/spaces/space1/types")
		require.Len(t, sent, 2, "the pre-flight dry run, then the create")
		assert.Equal(t, "true", sent[0].Query.Get("dry_run"))
		assert.Empty(t, sent[1].Query.Get("dry_run"))
		assert.Equal(t, want, typeDefinitionsSent(t, sent[1]),
			"an existing property is addressed by its exact key, a new one by the caller's name")
		assert.Equal(t, map[string]any{"name": "Cookbook entry"}, bodyJSON(t, sent[1])["properties"])
		assert.Empty(t, fx.sent("POST /v2/spaces/space1/properties"),
			"no select carries options, so nothing needs the property route")
		assert.Contains(t, result.Text, `created type "Cookbook entry" (cookbook_entry)`)
		assert.Contains(t, result.Text, "Cook time: number")
		assert.Contains(t, result.Text, "Source: url  (this space's existing property)")
		assert.Contains(t, result.Text, `create objects of it with create: type "Cookbook entry"`)
	})

	t.Run("a select with options is created through the property route", func(t *testing.T) {
		// given — the type document CANNOT carry the options (the write path
		// drops them silently, measured against a live heart), so the property
		// is created first, with its options, and the type then references it
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse())
		fx.stub("POST /v2/spaces/space1/types", 200, `{"key":"cookbook_entry","dry_run":true}`)
		fx.stub("POST /v2/spaces/space1/properties", 201,
			`{"id":"bafyreig3azs6","key":"rating","created":{"options":[{"property":"rating","name":"Low"},{"property":"rating","name":"High"}]}}`)
		fx.stub("POST /v2/spaces/space1/types", 201, `{"id":"bafyreig5vvi7","key":"cookbook_entry"}`)
		want := map[string]any{
			"name": "Rating", "format": "select",
			"options": []any{map[string]any{"name": "Low"}, map[string]any{"name": "High"}},
		}

		// when
		result, err := fx.Run(ctx, "create_type", map[string]any{
			"space": "space1", "name": "Cookbook entry",
			"properties": "Rating: select(Low, High)",
		})

		// then
		require.NoError(t, err)
		minted := fx.sent("POST /v2/spaces/space1/properties")
		require.Len(t, minted, 1)
		assert.Equal(t, want, bodyJSON(t, minted[0]))
		// the ORDER is the safety argument: nothing before the last request
		// can create a type, so no failure here leaves a half-made one
		assert.Equal(t, []string{
			"GET /v2/spaces/space1/properties",
			"POST /v2/spaces/space1/types",      // the pre-flight dry run
			"POST /v2/spaces/space1/properties", // the option-bearing property
			"POST /v2/spaces/space1/types",      // the type itself, last
		}, requestOrder(fx))
		created := fx.sent("POST /v2/spaces/space1/types")[1]
		defs := typeDefinitionsSent(t, created)
		require.Len(t, defs, 1)
		assert.Equal(t, "rating", defs[0]["property"], "the type addresses the minted key, which cannot be ambiguous")
		assert.NotContains(t, defs[0], "options",
			"the document must not state options the server drops — a select that claims options it lacks is worse than none")
		assert.Contains(t, result.Text, "Rating: select(Low, High)")
	})

	t.Run("the receipt round-trips as the properties argument", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse())
		fx.stub("POST /v2/spaces/space1/types", 200, `{"key":"t","dry_run":true}`)
		fx.stub("POST /v2/spaces/space1/properties", 201, `{"id":"p1","key":"rating"}`)
		fx.stub("POST /v2/spaces/space1/types", 201, `{"id":"t1","key":"t"}`)

		// when
		result, err := fx.Run(ctx, "create_type", map[string]any{
			"space": "space1", "name": "Thing",
			"properties": "Rating: select(Low, High), Cook time: number",
		})

		// then
		require.NoError(t, err)
		js, ok := result.JSON.(createTypeResult)
		require.True(t, ok)
		require.Len(t, js.Properties, 2)
		rendered := make([]string, 0, len(js.Properties))
		for _, p := range js.Properties {
			rendered = append(rendered, typePropertyDecl{Name: p.Name, Format: p.Format, Options: p.Options}.render())
		}
		reparsed, err := parseTypeProperties(rendered[0] + ", " + rendered[1])
		require.NoError(t, err)
		assert.Equal(t, "Rating: select(Low, High)", rendered[0])
		assert.Len(t, reparsed, 2, "what the tool prints is accepted as what the tool takes")
	})

	t.Run("a duplicate type name is refused before anything is created", func(t *testing.T) {
		// given — the type name is checked by the SERVER, which is the only
		// party that knows the whole namespace, and it is checked with
		// dry_run=true so the refusal costs nothing permanent
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse())
		fx.stub("POST /v2/spaces/space1/types", 400,
			`{"status":400,"code":"validation_failed","message":"type key already exists","issues":[{"path":"/properties/name","message":"key \"cookbook\" is taken by type \"Cookbook\" in space space1","hint":"update it with PATCH /v2/spaces/space1/types/cookbook, or pick a different key"}]}`)

		// when
		_, err := fx.Run(ctx, "create_type", map[string]any{
			"space": "space1", "name": "Cookbook",
			"properties": "Rating: select(Low, High)",
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `key "cookbook" is taken by type "Cookbook"`, "the server's fact survives")
		assert.Contains(t, err.Error(), `a type named "Cookbook" is already in this space`)
		assert.Contains(t, err.Error(), "cannot rename or delete a type")
		assert.Empty(t, fx.sent("POST /v2/spaces/space1/properties"),
			"the refusal fired before the property mint — nothing was created")
		assert.Len(t, fx.sent("POST /v2/spaces/space1/types"), 1, "only the dry run was sent")
	})

	t.Run("a name a bundled type reserves steers to using that type", func(t *testing.T) {
		// given — a dozen everyday names (Recipe, Book, Movie, Project…) are
		// reserved by bundled types and appear in NO listing until something
		// uses them, so the refusal is the only place the caller can learn the
		// type exists at all
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/types", 400,
			`{"status":400,"code":"validation_failed","message":"type key is reserved","issues":[{"path":"/properties/name","message":"key \"recipe\" is taken by bundled type \"Recipe\" — it already exists"}]}`)

		// when
		_, err := fx.Run(ctx, "create_type", map[string]any{"space": "space1", "name": "Recipe"})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `the built-in type "Recipe" already exists`)
		assert.Contains(t, err.Error(), `create with type "Recipe" installs it on first use`)
	})

	t.Run("an existing property with a different format is refused", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(
			v2model.PropertyRow{Key: "rating", Name: "Rating", Format: "number"}))

		// when
		_, err := fx.Run(ctx, "create_type", map[string]any{
			"space": "space1", "name": "Cookbook entry",
			"properties": "Rating: select(Low, High)",
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `property "Rating" already exists in this space with format number, not select`)
		assert.Contains(t, err.Error(), `write it as "Rating: number"`)
		assert.Empty(t, fx.sent("POST /v2/spaces/space1/types"), "refused before the pre-flight — nothing was sent")
	})

	t.Run("an existing select that lacks a declared option is refused, naming what it holds", func(t *testing.T) {
		// given — the type reuses the space's property as it is, so accepting
		// this would leave the caller believing in options that do not exist
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(
			v2model.PropertyRow{Key: "rating", Name: "Rating", Format: "select"}))
		fx.stub("GET /v2/spaces/space1/properties/rating/options", 200,
			optionsResponse("Low", "Medium"))

		// when
		_, err := fx.Run(ctx, "create_type", map[string]any{
			"space": "space1", "name": "Cookbook entry",
			"properties": "Rating: select(Low, Sublime)",
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `property "Rating" already exists in this space and has no option "Sublime"`)
		assert.Contains(t, err.Error(), "it holds: Low, Medium")
		assert.Contains(t, err.Error(), "nothing was created")
		assert.Empty(t, fx.sent("POST /v2/spaces/space1/types"))
	})

	t.Run("an existing select the declaration already satisfies passes", func(t *testing.T) {
		// given — transcribing a describe row back must not be an error
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(
			v2model.PropertyRow{Key: "rating", Name: "Rating", Format: "select"}))
		fx.stub("GET /v2/spaces/space1/properties/rating/options", 200, optionsResponse("Low", "Medium"))
		fx.stub("POST /v2/spaces/space1/types", 200, `{"key":"t","dry_run":true}`)
		fx.stub("POST /v2/spaces/space1/types", 201, `{"id":"t1","key":"t"}`)

		// when
		result, err := fx.Run(ctx, "create_type", map[string]any{
			"space": "space1", "name": "Thing", "properties": "Rating: select(Low, Medium)",
		})

		// then
		require.NoError(t, err)
		assert.Empty(t, fx.sent("POST /v2/spaces/space1/properties"), "the property already exists — it is reused, not minted")
		assert.Contains(t, result.Text, "(this space's existing property)")
	})

	t.Run("dry run stops at the pre-flight", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.Runner.DryRun = true
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse())
		fx.stub("POST /v2/spaces/space1/types", 200,
			`{"key":"cookbook_entry","dry_run":true,"created":{"properties":[{"key":"rating","name":"Rating","format":"select"}]}}`)

		// when
		result, err := fx.Run(ctx, "create_type", map[string]any{
			"space": "space1", "name": "Cookbook entry", "properties": "Rating: select(Low, High)",
		})

		// then
		require.NoError(t, err)
		assert.Len(t, fx.sent("POST /v2/spaces/space1/types"), 1)
		assert.Empty(t, fx.sent("POST /v2/spaces/space1/properties"),
			"--dry-run must not mint the properties the real run would")
		assert.Contains(t, result.Text, `dry run — a type "Cookbook entry" would be created`)
		assert.Contains(t, result.Text, "Rating: select(Low, High)")
	})

	t.Run("a type with no properties is allowed and says so", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/types", 200, `{"key":"bare","dry_run":true}`)
		fx.stub("POST /v2/spaces/space1/types", 201, `{"id":"b1","key":"bare"}`)

		// when
		result, err := fx.Run(ctx, "create_type", map[string]any{"space": "space1", "name": "Bare"})

		// then
		require.NoError(t, err)
		sent := fx.sent("POST /v2/spaces/space1/types")
		require.Len(t, sent, 2)
		assert.NotContains(t, bodyJSON(t, sent[1]), "type_settings")
		assert.Empty(t, fx.sent("GET /v2/spaces/space1/properties"), "no declarations, no property listing")
		assert.Contains(t, result.Text, "(no properties")
	})

	t.Run("a failed property mint says the type was not created", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse())
		fx.stub("POST /v2/spaces/space1/types", 200, `{"key":"t","dry_run":true}`)
		fx.stub("POST /v2/spaces/space1/properties", 400,
			`{"status":400,"code":"validation_failed","message":"property key already exists"}`)

		// when
		_, err := fx.Run(ctx, "create_type", map[string]any{
			"space": "space1", "name": "Thing", "properties": "Rating: select(Low)",
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `create property "Rating" with its options — the type was NOT created`)
		assert.Len(t, fx.sent("POST /v2/spaces/space1/types"), 1, "the dry run only")
	})

	t.Run("an ambiguous property spelling is refused with the candidates", func(t *testing.T) {
		// given — the package rule: ambiguity is refused, never guessed
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(
			v2model.PropertyRow{Key: "rating", Name: "Rating", Format: "select"},
			v2model.PropertyRow{Key: "rating_2", Name: "Rating", Format: "text"}))

		// when
		_, err := fx.Run(ctx, "create_type", map[string]any{
			"space": "space1", "name": "Thing", "properties": "Rating: select(Low)",
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `property "Rating" matches several properties`)
		assert.Empty(t, fx.sent("POST /v2/spaces/space1/types"))
	})
}

// TestTypePropertyFormatsGolden pins the published enum. The list is the
// `property` discovery kind's `format` enum (v2service schemas.go) and the
// OpenAPI models' `enums:` tag — narrower than what the AnyBlock document
// schema accepts, deliberately. A format added to the published enum and not
// here would be unreachable from create_type; one added here and not
// published would be refused by the server after this tool accepted it.
func TestTypePropertyFormatsGolden(t *testing.T) {
	assert.Equal(t, []string{
		"text", "number", "select", "multi_select", "date", "files",
		"checkbox", "url", "email", "phone", "objects",
	}, typePropertyFormats)
	for format := range selectFormats {
		assert.Contains(t, typePropertyFormats, format, "every option-bearing format must be declarable")
	}
}

// optionsResponse renders a stub option listing.
func optionsResponse(names ...string) string {
	rows := make([]v2model.OptionRow, 0, len(names))
	for _, name := range names {
		rows = append(rows, v2model.OptionRow{Name: name})
	}
	resp := v2model.ListResponse[v2model.OptionRow]{Data: rows, Total: len(rows), Limit: 100}
	data, _ := json.Marshal(resp)
	return string(data)
}
