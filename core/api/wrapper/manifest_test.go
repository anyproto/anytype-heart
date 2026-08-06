package wrapper

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestToolCount pins the design constraint: the tool set stays under the
// >15-tool small-model cliff (§7 — the count is part of the contract).
func TestToolCount(t *testing.T) {
	tools := Tools()
	assert.Len(t, tools, 11)
	assert.Less(t, len(tools), 15, "the tool count is a design constraint — small models degrade past ~15 tools")
}

// TestOneDefinition asserts the executor map and the tool table agree
// exactly — the one-definition contract behind CLI verbs == manifest tools.
func TestOneDefinition(t *testing.T) {
	names := map[string]bool{}
	for _, tool := range Tools() {
		names[tool.Name] = true
		assert.Contains(t, executors, tool.Name, "tool %s has no executor", tool.Name)
	}
	for name := range executors {
		assert.True(t, names[name], "executor %s has no tool definition", name)
	}
}

func TestToolTable(t *testing.T) {
	for _, tool := range Tools() {
		t.Run(tool.Name, func(t *testing.T) {
			assert.NotEmpty(t, tool.Description)
			require.NotEmpty(t, tool.Args)
			assert.NotEmpty(t, tool.Example, "every tool carries one worked example (C12)")
			// the example must satisfy the tool's own argument contract
			assert.NoError(t, validateArgs(tool, tool.Example))
			// verbs are the hyphenated tool names, and round-trip
			verb, ok := ToolByVerb(tool.Verb())
			require.True(t, ok)
			assert.Equal(t, tool.Name, verb.Name)
		})
	}
	verb, _ := ToolByName("set_properties")
	assert.Equal(t, "set-properties", verb.Verb())
}

// TestToolSchemasStrict asserts C13 on every served schema: valid JSON,
// additionalProperties: false, no $ref/recursion, every required arg
// declared, every string bounded.
func TestToolSchemasStrict(t *testing.T) {
	for _, tool := range Tools() {
		t.Run(tool.Name, func(t *testing.T) {
			raw, err := toolSchema(tool)
			require.NoError(t, err)
			assert.NotContains(t, string(raw), "$ref", "strict schemas stay non-recursive")

			var schema struct {
				Type                 string                    `json:"type"`
				AdditionalProperties bool                      `json:"additionalProperties"`
				Required             []string                  `json:"required"`
				Properties           map[string]map[string]any `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(raw, &schema))
			assert.Equal(t, "object", schema.Type)
			assert.False(t, schema.AdditionalProperties)
			for _, a := range tool.Args {
				prop, ok := schema.Properties[a.Name]
				require.True(t, ok, "arg %s missing from schema", a.Name)
				if a.Required {
					assert.Contains(t, schema.Required, a.Name)
				}
				if a.Type == ArgString && len(a.Enum) == 0 {
					assert.Contains(t, prop, "maxLength", "string arg %s must be bounded", a.Name)
				}
			}
		})
	}
}

// TestToolGBNFWellFormed asserts the §7.4 convertibility contract: every
// served grammar is well-formed GBNF with all rules defined.
func TestToolGBNFWellFormed(t *testing.T) {
	for _, tool := range Tools() {
		t.Run(tool.Name, func(t *testing.T) {
			grammar := toolGBNF(tool)
			require.NoError(t, checkGBNF(grammar))
			for _, a := range tool.Args {
				assert.Contains(t, grammar, `\"`+a.Name+`\"`, "arg %s missing from grammar", a.Name)
			}
		})
	}
	t.Run("filter string grammar", func(t *testing.T) {
		require.NoError(t, checkGBNF(filterStringGBNF))
		// the canonical keywords and every preset name are constrainable
		for _, kw := range []string{"AND", "OR", "CONTAINS", "EMPTY", "EXISTS", "currentWeek", "daysAgo", "daysFromNow"} {
			assert.Contains(t, filterStringGBNF, kw)
		}
	})
	t.Run("checker catches breakage", func(t *testing.T) {
		assert.Error(t, checkGBNF(`root ::= missing-rule`), "undefined reference must fail")
		assert.Error(t, checkGBNF(`root ::= "unterminated`), "unterminated literal must fail")
		assert.Error(t, checkGBNF(`root ::= ( "x"`), "unbalanced parens must fail")
		assert.Error(t, checkGBNF(`other ::= "x"`), "missing root must fail")
	})
}

func TestManifestJSON(t *testing.T) {
	data, err := ManifestJSON()
	require.NoError(t, err)
	var m Manifest
	require.NoError(t, json.Unmarshal(data, &m))
	assert.Equal(t, 1, m.Version)
	require.Len(t, m.Tools, len(Tools()))
	for i, tool := range Tools() {
		assert.Equal(t, tool.Name, m.Tools[i].Name)
		assert.NotEmpty(t, m.Tools[i].Parameters)
		assert.NotEmpty(t, m.Tools[i].GBNF)
	}
	assert.NotEmpty(t, m.FilterGrammar.EBNF)
	assert.NotEmpty(t, m.FilterGrammar.GBNF)
	assert.NotEmpty(t, m.FilterGrammar.Examples)
	assert.False(t, strings.Contains(string(data), "\n"), "the manifest is compact JSON (C3)")
}
