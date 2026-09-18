package v2service

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// servedRouteShaped is the route spelling core/api/v2/hintroutes_test.go
// refuses in served strings, applied here to the SCHEMA channel — the one
// that guard exempts, because the schema files legitimately carry each
// kind's `endpoint` line. The descriptions inside a served schema are repair
// advice too (round-two eval F7: twelve routes reached callers through them,
// and one was followed by hand into a data loss), so they name operations
// the way see_also does — by op id — and never a route.
//
// The boundary: every string inside a served kind or op SCHEMA is scanned.
// Not scanned, by design: the schema index (its endpoint and url members are
// routes), each entry's `endpoint`, `example`, `example_body`, `grammar` and
// `grammar_examples` (instances and a grammar, not advice).
var servedRouteShaped = regexp.MustCompile(`(?:GET|POST|PATCH|PUT|DELETE|HEAD) (?:/|…|\.\.\./)|(?:…|\.\.\.)/[a-z]|/v2/(?:spaces|schemas|search|auth|validate)\b|\?[A-Za-z0-9_-]+=|\b[a-z_]+=<[a-z_]+>`)

// anytypeLink is the format's own link syntax, exempt span-wise.
var anytypeLink = regexp.MustCompile(`anytype://[^\s)]*`)

func TestServedSchemaProseCarriesNoRoutes(t *testing.T) {
	t.Run("the link exemption covers the link span only", func(t *testing.T) {
		mixed := "see [it](anytype://object?objectId=x), then GET /v2/schemas/object"
		assert.NotEmpty(t, servedRouteShaped.FindString(anytypeLink.ReplaceAllString(mixed, "")))
		assert.Empty(t, servedRouteShaped.FindString(anytypeLink.ReplaceAllString("[it](anytype://object?objectId=x)", "")))
	})

	fx := newV2Fixture(t)
	var checked int
	var walk func(t *testing.T, where string, node any)
	walk = func(t *testing.T, where string, node any) {
		switch n := node.(type) {
		case map[string]any:
			for key, child := range n {
				walk(t, where+"/"+key, child)
			}
		case []any:
			for i, child := range n {
				walk(t, where+"/"+strconv.Itoa(i), child)
			}
		case string:
			checked++
			// the format's inline-markup grammar documents its own link
			// syntax (an anytype:// URL with a query) — a URL scheme, not a
			// route of this API; only that span is exempt
			if found := servedRouteShaped.FindString(anytypeLink.ReplaceAllString(n, "")); found != "" {
				t.Errorf("%s spells a route (%q) — name the operation by its op id\n  %s", where, found, n)
			}
		}
	}
	for _, op := range v2AllOpNames() {
		entry, err := fx.SchemaOp(op)
		require.NoError(t, err, op)
		var schema any
		require.NoError(t, json.Unmarshal(entry.Schema, &schema), op)
		walk(t, "ops/"+op, schema)
	}
	for kind := range v2SchemaKinds {
		entry, err := fx.SchemaKind(kind)
		require.NoError(t, err, kind)
		var schema any
		require.NoError(t, json.Unmarshal(entry.Schema, &schema), kind)
		walk(t, "kinds/"+kind, schema)
	}
	require.NotZero(t, checked)
}

func TestServedSchemaLeadsWithItsDescription(t *testing.T) {
	// the $defs blob is last: a reader (or a small consumer's context
	// window) meets the sentence that says what the schema is for first
	fx := newV2Fixture(t)
	for _, op := range v2AllOpNames() {
		entry, err := fx.SchemaOp(op)
		require.NoError(t, err, op)
		raw := string(entry.Schema)
		if !strings.Contains(raw, `"$defs"`) {
			continue
		}
		require.Less(t, strings.Index(raw, `"description"`), strings.Index(raw, `"$defs"`), "%s: $defs before the description", op)
		var schema map[string]any
		require.NoError(t, json.Unmarshal(entry.Schema, &schema), op)
		require.Contains(t, schema, "$defs", op)
	}
}

// schemaKindMention is the spelling a served description uses in place of a
// schema route; each must resolve, or the prose points at nothing.
var schemaKindMention = regexp.MustCompile(`schema kind ([a-z_]+)`)

func TestServedSchemaProseNamesThingsThatExist(t *testing.T) {
	fx := newV2Fixture(t)
	// the operation ids and the PATCH op names the F7 rewrite put into
	// descriptions; a new mention joins these lists when it is written
	for _, op := range []string{"update_type", "update_property", "list_properties"} {
		_, _, ok := v2model.Operation(op)
		require.True(t, ok, "%s is not an operation of the table", op)
	}
	for _, op := range []string{"add_property", "remove_property", "move_property", "insert_view"} {
		require.Contains(t, v2AllOpNames(), op, "%s is not a PATCH op", op)
	}
	var strs []string
	var collect func(node any)
	collect = func(node any) {
		switch n := node.(type) {
		case map[string]any:
			for _, child := range n {
				collect(child)
			}
		case []any:
			for _, child := range n {
				collect(child)
			}
		case string:
			strs = append(strs, n)
		}
	}
	for _, op := range v2AllOpNames() {
		entry, err := fx.SchemaOp(op)
		require.NoError(t, err)
		var schema any
		require.NoError(t, json.Unmarshal(entry.Schema, &schema))
		collect(schema)
	}
	for kind := range v2SchemaKinds {
		entry, err := fx.SchemaKind(kind)
		require.NoError(t, err)
		var schema any
		require.NoError(t, json.Unmarshal(entry.Schema, &schema))
		collect(schema)
	}
	var mentions int
	for _, s := range strs {
		for _, m := range schemaKindMention.FindAllStringSubmatch(s, -1) {
			mentions++
			_, ok := v2SchemaKinds[m[1]]
			assert.True(t, ok, "%q names a schema kind that does not exist: %s", m[1], s)
		}
	}
	require.NotZero(t, mentions, "the rewrite's 'schema kind X' spelling is in use")
}
