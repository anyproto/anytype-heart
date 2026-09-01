package v2service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
)

func TestV2Schemas(t *testing.T) {
	fx := newV2FixtureBare(t)

	t.Run("index lists every kind with its endpoint and url", func(t *testing.T) {
		// when
		index := fx.SchemaIndex()

		// then
		require.NotEmpty(t, index.Kinds)
		kinds := map[string]bool{}
		for _, entry := range index.Kinds {
			kinds[entry.Kind] = true
			assert.NotEmpty(t, entry.Endpoint, entry.Kind)
			assert.Equal(t, "/v2/schemas/"+entry.Kind, entry.Url)
		}
		for _, want := range []string{"object", "shortcut", "type", "template", "property", "set", "collection", "file", "filters", "search", "space", "chat", "chatMessage", "chatMessageEdit", "chatReaction", "chatRead"} {
			assert.True(t, kinds[want], "missing kind %s", want)
		}
	})

	t.Run("the chat kinds are strict-mode-decodable and their examples fit (Phase 6)", func(t *testing.T) {
		// every chat body an agent must author has a schema kind (§5) —
		// incl. the tiny edit/reaction bodies, because the handlers decode
		// strictly and a guessed field name eats an avoidable 400;
		// strictness follows C13
		for _, kind := range []string{"chat", "chatMessage", "chatMessageEdit", "chatReaction", "chatRead"} {
			entry, err := fx.SchemaKind(kind)
			require.NoError(t, err, kind)
			var schema struct {
				AdditionalProperties *bool                      `json:"additionalProperties"`
				Properties           map[string]json.RawMessage `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(entry.Schema, &schema), kind)
			require.NotNil(t, schema.AdditionalProperties, "%s must pin additionalProperties", kind)
			assert.False(t, *schema.AdditionalProperties, "%s must be strict (C13)", kind)

			// the worked example must only use declared fields
			var example map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(entry.Example, &example), kind)
			for field := range example {
				assert.Contains(t, schema.Properties, field,
					"example field %q of kind %s is not in its own schema", field, kind)
			}
		}
	})

	t.Run("the space kind is strict and its example fits (Phase 7)", func(t *testing.T) {
		// POST /v2/spaces is an authoring surface, so it carries a schema
		// kind (§5); strictness follows C13
		entry, err := fx.SchemaKind("space")
		require.NoError(t, err)
		var schema struct {
			AdditionalProperties *bool                      `json:"additionalProperties"`
			Required             []string                   `json:"required"`
			Properties           map[string]json.RawMessage `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(entry.Schema, &schema))
		require.NotNil(t, schema.AdditionalProperties)
		assert.False(t, *schema.AdditionalProperties, "the space kind must be strict (C13)")
		assert.Equal(t, []string{"name"}, schema.Required)

		var example map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(entry.Example, &example))
		for field := range example {
			assert.Contains(t, schema.Properties, field,
				"example field %q is not in the space schema", field)
		}
	})

	t.Run("chat schema bounds match the enforced caps — the schema must not out-promise the store", func(t *testing.T) {
		// a constrained-decoding model OBEYS the schema: advertising
		// maxLength 65536 against the store's 8000-UTF-16-unit cap (the
		// original Phase-6 defect) steers it straight into a rejection;
		// this drift test pins the served bounds to the enforced constants
		var bounds = func(kind string) map[string]struct {
			MaxLength int `json:"maxLength"`
			MaxItems  int `json:"maxItems"`
		} {
			entry, err := fx.SchemaKind(kind)
			require.NoError(t, err, kind)
			var schema struct {
				Properties map[string]struct {
					MaxLength int `json:"maxLength"`
					MaxItems  int `json:"maxItems"`
				} `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(entry.Schema, &schema), kind)
			return schema.Properties
		}
		message := bounds("chatMessage")
		assert.Equal(t, chatmodel.MaxMessageLength, message["text"].MaxLength,
			"chatMessage.text maxLength must equal the store cap (UTF-16 code units)")
		assert.Equal(t, maxChatAttachments, message["attachments"].MaxItems,
			"chatMessage.attachments maxItems must equal the enforced cap")
		edit := bounds("chatMessageEdit")
		assert.Equal(t, chatmodel.MaxMessageLength, edit["text"].MaxLength,
			"chatMessageEdit.text maxLength must equal the store cap")
	})

	t.Run("Phase-2 schema bounds match the enforced constants (M6)", func(t *testing.T) {
		// the property/set/collection/file kinds advertise
		// additionalProperties:false plus bounds; the endpoints now bind
		// strict and enforce those bounds (schema_write.go constants) — this
		// drift test pins the served JSON to the enforced values so neither
		// side can move alone
		var bounds = func(kind string) map[string]struct {
			MaxLength int    `json:"maxLength"`
			MaxItems  int    `json:"maxItems"`
			Pattern   string `json:"pattern"`
		} {
			entry, err := fx.SchemaKind(kind)
			require.NoError(t, err, kind)
			var schema struct {
				Properties map[string]struct {
					MaxLength int    `json:"maxLength"`
					MaxItems  int    `json:"maxItems"`
					Pattern   string `json:"pattern"`
				} `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(entry.Schema, &schema), kind)
			return schema.Properties
		}

		property := bounds("property")
		assert.Equal(t, maxV2NameLength, property["name"].MaxLength)
		assert.Equal(t, maxV2KeyLength, property["key"].MaxLength)
		assert.Equal(t, v2PropertyKeyPattern.String(), property["key"].Pattern,
			"the advertised key pattern must be the enforced one")
		assert.Equal(t, maxV2PropertyOptions, property["options"].MaxItems)

		// the option entries' bounds are nested under options.items
		entry, err := fx.SchemaKind("property")
		require.NoError(t, err)
		var propertySchema struct {
			Properties struct {
				Options struct {
					Items struct {
						Properties map[string]struct {
							MaxLength int `json:"maxLength"`
						} `json:"properties"`
					} `json:"items"`
				} `json:"options"`
			} `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(entry.Schema, &propertySchema))
		assert.Equal(t, maxV2NameLength, propertySchema.Properties.Options.Items.Properties["name"].MaxLength)
		assert.Equal(t, maxV2OptionColorLength, propertySchema.Properties.Options.Items.Properties["color"].MaxLength)

		set := bounds("set")
		assert.Equal(t, maxV2NameLength, set["name"].MaxLength)
		assert.Equal(t, maxV2KeyLength, set["type"].MaxLength)
		assert.Equal(t, maxV2FilterLength, set["filter"].MaxLength)
		assert.Equal(t, maxV2SetSorts, set["sorts"].MaxItems)
		assert.Equal(t, maxV2SetViews, set["views"].MaxItems)

		collection := bounds("collection")
		assert.Equal(t, maxV2NameLength, collection["name"].MaxLength)
		assert.Equal(t, maxV2CollectionItems, collection["items"].MaxItems)

		file := bounds("file")
		assert.Equal(t, maxV2UrlLength, file["url"].MaxLength)
	})

	t.Run("every kind serves parseable schema and example (C12/C13)", func(t *testing.T) {
		for _, entry := range fx.SchemaIndex().Kinds {
			got, err := fx.SchemaKind(entry.Kind)
			require.NoError(t, err, entry.Kind)
			var schema, example any
			require.NoError(t, json.Unmarshal(got.Schema, &schema), "schema of %s must be valid JSON", entry.Kind)
			require.NoError(t, json.Unmarshal(got.Example, &example), "example of %s must be valid JSON", entry.Kind)
		}
	})

	t.Run("AnyBlock examples pass the format's own validation", func(t *testing.T) {
		// the object, type and template examples are full AnyBlock documents;
		// serving an example the format rejects would poison every agent
		for _, kind := range []string{"object", "type", "template"} {
			entry, err := fx.SchemaKind(kind)
			require.NoError(t, err)
			assert.NoError(t, anyblockjson.Validate(entry.Example), "example of kind %s", kind)
		}
	})

	t.Run("object kind serves the complete embedded format schema with C13 bounds", func(t *testing.T) {
		entry, err := fx.SchemaKind("object")
		require.NoError(t, err)
		want, err := strictDiscoverySchema(anyblockjson.SchemaJSON())
		require.NoError(t, err)
		assert.JSONEq(t, string(want), string(entry.Schema))
	})

	t.Run("the filters kind carries the filter-string grammar (§5 Phase 4)", func(t *testing.T) {
		// one concept, one slot (C2): the structured-array schema AND the
		// string grammar ride the same kind — the artifact the Phase-5 GBNF
		// conversion consumes
		entry, err := fx.SchemaKind("filters")
		require.NoError(t, err)
		assert.Contains(t, entry.Grammar, "orExpr", "the EBNF the parser pins")
		assert.Contains(t, entry.Grammar, `"HAS" , "ALL"`)
		require.NotEmpty(t, entry.GrammarExamples)
		for _, example := range entry.GrammarExamples {
			_, err := filterstring.Parse(example, filterstring.Options{})
			assert.NoError(t, err, "served grammar example %q must parse", example)
		}
	})

	t.Run("no other kind carries a grammar", func(t *testing.T) {
		entry, err := fx.SchemaKind("search")
		require.NoError(t, err)
		assert.Empty(t, entry.Grammar)
		assert.Empty(t, entry.GrammarExamples)
	})

	t.Run("the served EBNF defines every token the parser accepts", func(t *testing.T) {
		// the grammar is the Phase-5 GBNF input: a keyword or preset the
		// parser accepts but the EBNF omits (or an undefined production like
		// identifier/number) would make a generated GBNF reject valid input
		entry, err := fx.SchemaKind("filters")
		require.NoError(t, err)
		for _, token := range []string{
			`"OR"`, `"AND"`, `"NOT"`, `"CONTAINS"`, `"IN"`, `"HAS"`, `"ALL"`, `"IS"`, `"EMPTY"`, `"EXISTS"`,
			`"true"`, `"false"`, `"!="`, `">="`, `"<="`,
			"yesterday", "today", "tomorrow", "lastWeek", "currentWeek", "nextWeek",
			"lastMonth", "currentMonth", "nextMonth", "lastYear", "currentYear", "nextYear",
			"daysAgo", "daysFromNow",
			"identifier  =", "number      =", "case-insensitively",
		} {
			assert.Contains(t, entry.Grammar, token, "the served EBNF must carry %s", token)
		}
	})

	t.Run("the search kind is fully strict-mode-decodable (C13)", func(t *testing.T) {
		// the recursive structured `filters` channel must not leak into the
		// generation-facing search schema: constrained decoders (OpenAI
		// strict, GBNF) reject an array without items, which would poison
		// the whole kind — the one the Phase-5 find tool constrains against
		for _, kind := range []string{"search", "set"} {
			entry, err := fx.SchemaKind(kind)
			require.NoError(t, err)
			var schema struct {
				Properties map[string]json.RawMessage `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(entry.Schema, &schema))
			assert.NotContains(t, schema.Properties, "filters",
				"kind %s must steer to the filter string / kind filters instead of embedding the recursive array", kind)
		}
		// every array in the search schema carries an items schema
		entry, err := fx.SchemaKind("search")
		require.NoError(t, err)
		var schema struct {
			Properties map[string]map[string]json.RawMessage `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(entry.Schema, &schema))
		for name, prop := range schema.Properties {
			if string(prop["type"]) == `"array"` {
				assert.Contains(t, prop, "items", "search.%s is an array and needs items for strict decoding", name)
			}
		}
	})

	t.Run("the search example's filter string parses", func(t *testing.T) {
		entry, err := fx.SchemaKind("search")
		require.NoError(t, err)
		var example struct {
			Filter string `json:"filter"`
		}
		require.NoError(t, json.Unmarshal(entry.Example, &example))
		require.NotEmpty(t, example.Filter)
		_, err = filterstring.Parse(example.Filter, filterstring.Options{})
		assert.NoError(t, err, "the served worked example must parse (C12)")
	})

	t.Run("unknown kind is a 404 naming the available kinds", func(t *testing.T) {
		_, err := fx.SchemaKind("wat")
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Contains(t, apiErr.Message, "object")
	})
}

func TestDiscoverySchemasAreClosedAndBounded(t *testing.T) {
	fx := newV2FixtureBare(t)
	index := fx.SchemaIndex()
	var problems []string
	var badReferences []string
	var cycles []string
	for _, indexed := range append(index.Kinds, index.Ops...) {
		var raw json.RawMessage
		if indexed.Url == "/v2/schemas/ops/"+indexed.Kind {
			entry, err := fx.SchemaOp(indexed.Kind)
			require.NoError(t, err)
			raw = entry.Schema
		} else {
			entry, err := fx.SchemaKind(indexed.Kind)
			require.NoError(t, err)
			raw = entry.Schema
		}
		var schema map[string]any
		require.NoError(t, json.Unmarshal(raw, &schema), indexed.Kind)
		if indexed.Kind != "filters" {
			assert.Equal(t, false, schema["additionalProperties"], "%s root must be closed", indexed.Kind)
		}
		auditStrictDiscoveryNode(schema, indexed.Kind+"#", &problems)
		bad, foundCycles := auditDiscoveryReferences(indexed.Kind, schema)
		badReferences = append(badReferences, bad...)
		cycles = append(cycles, foundCycles...)
	}
	assert.Empty(t, problems)
	assert.Empty(t, badReferences)
	sort.Strings(cycles)
	assert.Equal(t, []string{
		"filters: filterNode -> filterNode",
		"object: filterNode -> filterNode",
		"template: filterNode -> filterNode",
		"type: filterNode -> filterNode",
	}, cycles)
}

func auditStrictDiscoveryNode(value any, path string, problems *[]string) {
	switch node := value.(type) {
	case map[string]any:
		if schemaNodeSupports(node, "object") {
			additional := node["additionalProperties"]
			additionalSchema, mapShape := additional.(map[string]any)
			_, boundedMap := positiveJSONNumber(node["maxProperties"])
			if _, fixed := node["properties"].(map[string]any); fixed {
				if additional != false && node["unevaluatedProperties"] != false && !(mapShape && boundedMap) {
					*problems = append(*problems, path+": open object")
				}
			} else if additional != false && node["unevaluatedProperties"] != false && !(mapShape && len(additionalSchema) > 0 && boundedMap) {
				*problems = append(*problems, path+": untyped open object")
			}
		}
		if schemaNodeSupports(node, "string") && !schemaStringIsBounded(node) {
			*problems = append(*problems, path+": unbounded string")
		}
		if schemaNodeSupports(node, "array") {
			if _, ok := positiveJSONNumber(node["maxItems"]); !ok {
				*problems = append(*problems, path+": unbounded array")
			}
			if items, ok := node["items"]; !ok || (items != true && items != false && !isJSONSchemaObject(items)) {
				*problems = append(*problems, path+": untyped array")
			}
		}
		for key, child := range node {
			auditStrictDiscoveryNode(child, fmt.Sprintf("%s/%s", path, key), problems)
		}
	case []any:
		for i, child := range node {
			auditStrictDiscoveryNode(child, fmt.Sprintf("%s/%d", path, i), problems)
		}
	}
}

func isJSONSchemaObject(value any) bool {
	schema, ok := value.(map[string]any)
	return ok && len(schema) > 0
}

func auditDiscoveryReferences(kind string, root map[string]any) (badReferences, cycles []string) {
	var allReferences []string
	collectDiscoveryReferences(root, &allReferences)
	for _, ref := range allReferences {
		if !strings.HasPrefix(ref, "#/") {
			badReferences = append(badReferences, fmt.Sprintf("%s: external reference %s", kind, ref))
		} else if _, ok := resolveDiscoveryReference(root, ref); !ok {
			badReferences = append(badReferences, fmt.Sprintf("%s: missing reference %s", kind, ref))
		}
	}

	definitions, _ := root["$defs"].(map[string]any)
	graph := make(map[string][]string, len(definitions))
	for name, definition := range definitions {
		var refs []string
		collectDiscoveryReferences(definition, &refs)
		for _, ref := range refs {
			const prefix = "#/$defs/"
			if !strings.HasPrefix(ref, prefix) {
				continue
			}
			target := strings.Split(strings.TrimPrefix(ref, prefix), "/")[0]
			graph[name] = append(graph[name], decodeDiscoveryPointerPart(target))
		}
	}

	visiting := make([]string, 0, len(graph))
	visited := make(map[string]bool, len(graph))
	recorded := map[string]bool{}
	var visit func(string)
	visit = func(name string) {
		for i, active := range visiting {
			if active == name {
				cycle := kind + ": " + strings.Join(append(append([]string{}, visiting[i:]...), name), " -> ")
				recorded[cycle] = true
				return
			}
		}
		if visited[name] {
			return
		}
		visiting = append(visiting, name)
		for _, target := range graph[name] {
			visit(target)
		}
		visiting = visiting[:len(visiting)-1]
		visited[name] = true
	}
	for name := range graph {
		visit(name)
	}
	for cycle := range recorded {
		cycles = append(cycles, cycle)
	}
	sort.Strings(badReferences)
	sort.Strings(cycles)
	return badReferences, cycles
}

func collectDiscoveryReferences(value any, output *[]string) {
	switch value := value.(type) {
	case map[string]any:
		if ref, ok := value["$ref"].(string); ok {
			*output = append(*output, ref)
		}
		for _, child := range value {
			collectDiscoveryReferences(child, output)
		}
	case []any:
		for _, child := range value {
			collectDiscoveryReferences(child, output)
		}
	}
}

func resolveDiscoveryReference(root map[string]any, ref string) (any, bool) {
	if ref == "#" {
		return root, true
	}
	if !strings.HasPrefix(ref, "#/") {
		return nil, false
	}
	var current any = root
	for _, rawPart := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		part := decodeDiscoveryPointerPart(rawPart)
		switch node := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = node[part]
			if !ok {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(node) {
				return nil, false
			}
			current = node[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func decodeDiscoveryPointerPart(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~1", "/"), "~0", "~")
}

func TestAnyBlockDiscoveryExamplesValidateAgainstServedSchema(t *testing.T) {
	fx := newV2FixtureBare(t)
	for _, kind := range []string{"object", "type", "template"} {
		entry, err := fx.SchemaKind(kind)
		require.NoError(t, err)
		assert.NoError(t, validateAgainstSchema(t, entry.Schema, entry.Example), kind)
	}
}

func TestAnyBlockDiscoveryFlatteningPreservesInheritedPropertyConstraints(t *testing.T) {
	fx := newV2FixtureBare(t)
	entry, err := fx.SchemaKind("type")
	require.NoError(t, err)

	var invalid map[string]any
	require.NoError(t, json.Unmarshal(entry.Example, &invalid))
	typeSettings := invalid["type_settings"].(map[string]any)
	definitions := typeSettings["property_definitions"].([]any)
	definitions[0].(map[string]any)["object_types"] = []any{float64(123)}
	raw, err := json.Marshal(invalid)
	require.NoError(t, err)

	assert.Error(t, anyblockjson.Validate(raw), "the source schema rejects a non-string target type")
	assert.Error(t, validateAgainstSchema(t, entry.Schema, raw),
		"the normalized discovery schema must retain propertyDefinition's string-item constraint")
}
