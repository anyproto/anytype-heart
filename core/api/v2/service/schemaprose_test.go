package v2service

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// servedRouteShaped is the route spelling core/api/v2/hintroutes_test.go
// refuses in served strings, applied here to the SCHEMA channel — the one
// that guard exempts, because the schema files legitimately carry each
// kind's `endpoint` line. The descriptions inside a served schema are repair
// advice too (round-two eval F7: twelve routes reached callers through them,
// and one was followed by hand into a data loss), so they name operations
// the way see_also does — by op id — and never a route.
var servedRouteShaped = regexp.MustCompile(`(?:GET|POST|PATCH|PUT|DELETE|HEAD) (?:/|…|\.\.\./)|(?:…|\.\.\.)/[a-z]|/v2/(?:spaces|schemas|search|auth|validate)\b|\?[A-Za-z0-9_-]+=|\b[a-z_]+=<[a-z_]+>`)

func TestServedSchemaProseCarriesNoRoutes(t *testing.T) {
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
				walk(t, where+"/"+string(rune('0'+i)), child)
			}
		case string:
			checked++
			// the format's inline-markup grammar documents its own link
			// syntax (an anytype:// URL with a query) — a URL scheme, not a
			// route of this API
			if strings.Contains(n, "anytype://") {
				return
			}
			if found := servedRouteShaped.FindString(n); found != "" {
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
