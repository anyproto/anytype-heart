package wrapper

// describe's option-listing mode: the only route to the option names of a
// select the type does not name (the "also settable" rows carry no options,
// because loading them would cost one request per property on every
// describe), and the way past the inline preview's ellipsis.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// stubSpaceProperties queues the space's property listing.
func stubSpaceProperties(fx *fixture, rows ...string) {
	fx.stub("GET /v2/spaces/space1/properties", 200,
		fmt.Sprintf(`{"data":[%s],"has_more":false}`, strings.Join(rows, ",")))
}

func propRow(key, name, format string) string {
	return fmt.Sprintf(`{"key":%q,"name":%q,"format":%q}`, key, name, format)
}

func optionsBody(hasMore bool, names ...string) string {
	rows := make([]string, 0, len(names))
	for _, n := range names {
		rows = append(rows, fmt.Sprintf(`{"name":%q}`, n))
	}
	return fmt.Sprintf(`{"data":[%s],"has_more":%t}`, strings.Join(rows, ","), hasMore)
}

func TestDescribeOptions(t *testing.T) {
	t.Run("lists a property the type does not name", func(t *testing.T) {
		// given — Region is a space property, not one of the type's
		fx := newFixture(t)
		stubSpaceProperties(fx, propRow("region", "Region", "select"))
		fx.stub("GET /v2/spaces/space1/properties/region/options", 200,
			optionsBody(false, "North", "South", "East"))
		want := "Region: select(North, South, East)"

		// when
		got, err := fx.Run(context.Background(), "describe", map[string]any{
			"space": "space1", "type": "Task", "options": "Region"})

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got.Text)
	})

	t.Run("a truncated listing names the move that narrows it", func(t *testing.T) {
		// given — the ellipsis this mode exists to resolve must not return
		fx := newFixture(t)
		stubSpaceProperties(fx, propRow("region", "Region", "select"))
		fx.stub("GET /v2/spaces/space1/properties/region/options", 200,
			optionsBody(true, "Alpha", "Beta"))

		// when
		got, err := fx.Run(context.Background(), "describe", map[string]any{
			"space": "space1", "type": "Task", "options": "Region"})

		// then
		require.NoError(t, err)
		assert.Contains(t, got.Text, "starting_with")
		assert.NotContains(t, got.Text, "…", "a dead-end ellipsis is what this mode replaces")
	})

	t.Run("starting_with reaches the route's prefix search", func(t *testing.T) {
		// given
		fx := newFixture(t)
		stubSpaceProperties(fx, propRow("region", "Region", "select"))
		fx.stub("GET /v2/spaces/space1/properties/region/options", 200,
			optionsBody(false, "North"))

		// when
		_, err := fx.Run(context.Background(), "describe", map[string]any{
			"space": "space1", "type": "Task", "options": "Region", "starting_with": "Nor"})

		// then
		require.NoError(t, err)
		sent := fx.sent("GET /v2/spaces/space1/properties/region/options")
		require.Len(t, sent, 1)
		assert.Equal(t, "Nor", sent[0].Query.Get("prefix"))
	})

	t.Run("the property is addressed by name, the route by key", func(t *testing.T) {
		// given — the surface teaches names (D5); the route takes api keys
		fx := newFixture(t)
		stubSpaceProperties(fx, propRow("region_code", "Region", "select"))
		fx.stub("GET /v2/spaces/space1/properties/region_code/options", 200,
			optionsBody(false, "North"))

		// when
		got, err := fx.Run(context.Background(), "describe", map[string]any{
			"space": "space1", "type": "Task", "options": "Region"})

		// then
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(got.Text, "Region:"), "got %q", got.Text)
	})

	t.Run("a non-select is refused by name, saying what it holds", func(t *testing.T) {
		// given
		fx := newFixture(t)
		stubSpaceProperties(fx, propRow("cook_time", "Cook time", "number"))

		// when
		_, err := fx.Run(context.Background(), "describe", map[string]any{
			"space": "space1", "type": "Task", "options": "Cook time"})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"Cook time" holds number`)
	})

	t.Run("an unknown property points back at describe", func(t *testing.T) {
		// given
		fx := newFixture(t)
		stubSpaceProperties(fx, propRow("region", "Region", "select"))

		// when
		_, err := fx.Run(context.Background(), "describe", map[string]any{
			"space": "space1", "type": "Task", "options": "Regoin"})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no property named")
		assert.Contains(t, err.Error(), "describe")
	})

	t.Run("an empty option list is said plainly, not as an empty listing", func(t *testing.T) {
		// given
		fx := newFixture(t)
		stubSpaceProperties(fx, propRow("region", "Region", "select"))
		fx.stub("GET /v2/spaces/space1/properties/region/options", 200, optionsBody(false))

		// when
		got, err := fx.Run(context.Background(), "describe", map[string]any{
			"space": "space1", "type": "Task", "options": "Region", "starting_with": "Zz"})

		// then
		require.NoError(t, err)
		assert.Contains(t, got.Text, `no option starting with "Zz"`)
	})

	t.Run("without options describe still describes the type", func(t *testing.T) {
		// given — the mode must not capture the ordinary call
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/types/Task", 404,
			`{"status":404,"code":"not_found","message":"no such type"}`)

		// when
		_, err := fx.Run(context.Background(), "describe", map[string]any{
			"space": "space1", "type": "Task"})

		// then — it reached the type route, not the option route
		require.Error(t, err)
		assert.NotEmpty(t, fx.sent("GET /v2/spaces/space1/types/Task"))
	})
}

// TestDescribeRowsTranscribeIntoCreateType is the assertion §8.48 claimed
// and did not have. The surface tells the model "each row is written the way
// create_type takes a property", so every row describe prints must parse as
// a create_type property spec. It did not: a truncated option list printed
// `Status: select(Backlog, Done, …)`, which parsed as an option literally
// named "…", and a failed option listing printed the caveat inside the row,
// which parsed as a format named `select  — options could not be listed…`.
func TestDescribeRowsTranscribeIntoCreateType(t *testing.T) {
	// given — one plain select, one truncated, one whose options failed
	fx := newFixture(t)
	fx.stub("GET /v2/spaces/space1/types/task", 200,
		`{"formatVersion":"2.0","kind":"object_type","properties":{"name":"Task"},"type_settings":{"api_key":"task","property_definitions":[`+
			`{"property":"stage","name":"Stage","format":"select"},`+
			`{"property":"tag","name":"Tag","format":"multi_select"},`+
			`{"property":"status","name":"Status","format":"select"},`+
			`{"property":"cook_time","name":"Cook time","format":"number"}]}}`)
	fx.stub("GET /v2/spaces/space1/properties/stage/options", 200, optionsBody(false, "Draft", "Live"))
	fx.stub("GET /v2/spaces/space1/properties/tag/options", 200, optionsBody(true, "Alpha", "Beta"))
	fx.stub("GET /v2/spaces/space1/properties/status/options", 503, `oops`)
	fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(
		v2model.PropertyRow{Key: "stage", Name: "Stage", Format: "select"},
		v2model.PropertyRow{Key: "tag", Name: "Tag", Format: "multi_select"},
		v2model.PropertyRow{Key: "status", Name: "Status", Format: "select"},
		v2model.PropertyRow{Key: "cook_time", Name: "Cook time", Format: "number"},
	))

	// when
	result, err := fx.Run(context.Background(), "describe", map[string]any{"space": "space1", "type": "task"})
	require.NoError(t, err)

	// then — every indented row parses, and says back what it said
	var rows []string
	for _, line := range strings.Split(result.Text, "\n") {
		if !strings.HasPrefix(line, "  ") {
			continue
		}
		row := strings.TrimSpace(line)
		rows = append(rows, row)

		parsed, err := parseTypeProperties(row)
		require.NoErrorf(t, err, "describe printed a row create_type cannot parse: %q", row)
		require.Lenf(t, parsed, 1, "row %q parsed as %d properties", row, len(parsed))
		assert.Equalf(t, strings.SplitN(row, ":", 2)[0], parsed[0].Name, "row %q lost its name", row)
		for _, opt := range parsed[0].Options {
			assert.NotContainsf(t, opt, "…", "row %q yielded an ellipsis as an option name", row)
			assert.NotContainsf(t, opt, "could not be listed", "row %q yielded prose as an option name", row)
		}
	}
	require.NotEmpty(t, rows)

	// and the whole property block transcribes as ONE create_type argument
	joined := strings.Join(rows, ", ")
	all, err := parseTypeProperties(joined)
	require.NoErrorf(t, err, "joining describe's rows is not a valid properties argument: %q", joined)
	assert.Len(t, all, len(rows))
}

// TestCreateTypeRefusesAnUnaddressableMintedProperty covers the one outcome
// where the server succeeds and the tool still must stop: POST /properties
// returns an empty key. That is authoritative (storedApiKeyOf — the internal
// key is then the only address), not a glitch, and typeDocument would fall
// back to spelling the property by NAME, which can bind a different relation
// than the one just minted.
func TestCreateTypeRefusesAnUnaddressableMintedProperty(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse())
	fx.stub("POST /v2/spaces/space1/types", 200, `{"key":"t","dry_run":true}`)
	fx.stub("POST /v2/spaces/space1/properties", 201, `{"key":"","name":"★"}`)

	// when
	_, err := fx.Run(context.Background(), "create_type", map[string]any{
		"space": "space1", "name": "Thing", "properties": "★: select(A, B)"})

	// then — refused, the residue named, and no type created
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no key for it")
	assert.Contains(t, err.Error(), "these properties WERE created and remain in the space: ★")
	assert.Len(t, fx.sent("POST /v2/spaces/space1/types"), 1, "the dry run only")
}
