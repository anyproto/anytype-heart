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
