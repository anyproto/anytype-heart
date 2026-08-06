package v2service

import (
	"encoding/json"
	"net/http"
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

	t.Run("object kind serves the embedded format schema", func(t *testing.T) {
		entry, err := fx.SchemaKind("object")
		require.NoError(t, err)
		assert.JSONEq(t, string(anyblockjson.SchemaJSON()), string(entry.Schema))
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
