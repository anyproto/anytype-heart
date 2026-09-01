package api

// openapiprose_test.go guards the prose of the v2 OpenAPI document: the
// summaries and descriptions a reader outside this repository actually sees.
//
// It reads the generated document (core/api/docs/v2/openapi.json, embedded
// above as openapiV2JSON and served verbatim at /v2/docs/openapi.json) rather
// than the annotations in core/api/v2/handler/*.go. The generated document is
// what reaches a reader: it catches whatever swag rewrites on the way, and it
// catches prose that arrives from a model comment rather than from a handler.
//
// v1 is deliberately not covered. Its document is already published at
// developers.anytype.io; guarding it here would be a rewrite request for a
// document this package does not otherwise touch.

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// maxProseDescription bounds every description in the document except the
// API-level one. A description carries only what is specific to its endpoint;
// anything longer is boilerplate that belongs in the API description, which
// states the shared behaviour (auth, idempotency keys, dry runs, preconditions,
// pagination, the error shape, warnings) once for all 45 operations.
const maxProseDescription = 400

// apiDescription is the one description exempt from maxProseDescription: it is
// where the shared behaviour is stated, so it is long by design. The pattern
// rules below still apply to it.
const apiDescription = "/info/description"

// proseRules reject references that mean nothing outside this repository, and
// the punctuation the v2 prose does not use.
var proseRules = []struct {
	name    string
	pattern *regexp.Regexp
	fix     string
}{
	{
		name:    "numbered constraint",
		pattern: regexp.MustCompile(`C\d+`),
		fix:     "state the rule in plain words; a constraint number names nothing a reader can look up",
	},
	{
		name:    "section mark",
		pattern: regexp.MustCompile(`§`),
		fix:     "state the rule in plain words; the sections are internal documents",
	},
	{
		name:    "phase name",
		pattern: regexp.MustCompile(`(?i)phase \d`),
		fix:     "phase names describe how this API was built, not how it behaves",
	},
	{
		name:    "internal filename",
		pattern: regexp.MustCompile(`[\w/]+\.md`),
		fix:     "a reader cannot open a file in this repository; say what the file says",
	},
	{
		name:    "em dash",
		pattern: regexp.MustCompile("—"),
		fix:     "use a full stop; two short sentences beat one qualified sentence",
	},
}

// allCapsRun finds runs of three or more capitals: emphasis by shouting.
var allCapsRun = regexp.MustCompile(`\b[A-Z]{3,}\b`)

// allowedAllCaps are the all-caps tokens that are spellings rather than
// emphasis. Keeping the set this small is the point: it leaves no room for an
// EVERY or an ONLY to come back in.
var allowedAllCaps = map[string]bool{
	"GET": true, "POST": true, "PATCH": true, "PUT": true, "DELETE": true,
	"JSON": true, "URL": true, "UTF": true, "API": true, "JWT": true,
}

// proseEntry is one piece of prose in the document, addressed by JSON pointer
// so a failure names the annotation to edit.
type proseEntry struct {
	pointer string
	kind    string // "summary" or "description"
	text    string
}

// collectProse walks the document and returns every summary and description.
// Values under example, examples and default are skipped: those are data a
// caller sends, not prose a caller reads.
func collectProse(t *testing.T, doc []byte) []proseEntry {
	t.Helper()

	var root any
	require.NoError(t, json.Unmarshal(doc, &root))

	var entries []proseEntry
	var walk func(node any, pointer string)
	walk = func(node any, pointer string) {
		switch n := node.(type) {
		case map[string]any:
			keys := make([]string, 0, len(n))
			for k := range n {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if k == "example" || k == "examples" || k == "default" {
					continue
				}
				child := pointer + "/" + k
				if text, ok := n[k].(string); ok && (k == "summary" || k == "description") {
					entries = append(entries, proseEntry{pointer: child, kind: k, text: text})
					continue
				}
				walk(n[k], child)
			}
		case []any:
			for i, v := range n {
				walk(v, pointer+"/"+strconv.Itoa(i))
			}
		}
	}
	walk(root, "")

	require.NotEmpty(t, entries, "the embedded v2 document carries no prose at all")
	return entries
}

func TestV2DocumentProse(t *testing.T) {
	// given: the generated v2 document, exactly as it is served
	entries := collectProse(t, openapiV2JSON)

	t.Run("no reference a reader outside this repository cannot follow", func(t *testing.T) {
		for _, entry := range entries {
			for _, rule := range proseRules {
				// then
				assert.NotRegexp(t, rule.pattern, entry.text,
					"%s carries a %s (%s): %s", entry.pointer, rule.name, rule.fix, entry.text)
			}
		}
	})

	t.Run("no emphasis by shouting", func(t *testing.T) {
		for _, entry := range entries {
			for _, run := range allCapsRun.FindAllString(entry.text, -1) {
				// then
				assert.True(t, allowedAllCaps[run],
					"%s shouts %q: say the dangerous thing first instead of capitalising it — %s",
					entry.pointer, run, entry.text)
			}
		}
	})

	t.Run("a description carries only what is specific to its endpoint", func(t *testing.T) {
		for _, entry := range entries {
			if entry.kind != "description" || entry.pointer == apiDescription {
				continue
			}
			// then
			assert.LessOrEqual(t, len(entry.text), maxProseDescription,
				"%s is %d characters: shared behaviour belongs in the API description in core/api/v2/doc.go, and an empty description is a correct outcome",
				entry.pointer, len(entry.text))
		}
	})

	t.Run("every operation has a one-line summary", func(t *testing.T) {
		var doc struct {
			Paths map[string]map[string]struct {
				Summary string `json:"summary"`
			} `json:"paths"`
		}
		require.NoError(t, json.Unmarshal(openapiV2JSON, &doc))

		methods := map[string]bool{"get": true, "post": true, "put": true, "patch": true, "delete": true}
		operations := 0
		for path, item := range doc.Paths {
			for method, op := range item {
				if !methods[method] {
					continue
				}
				operations++
				// then
				require.NotEmpty(t, op.Summary, "%s %s has no summary", strings.ToUpper(method), path)
				assert.NotContains(t, op.Summary, "\n",
					"%s %s has a multi-line summary: %s", strings.ToUpper(method), path, op.Summary)
			}
		}
		assert.NotZero(t, operations)
	})
}

func TestV2UploadRequestBodyForms(t *testing.T) {
	var doc struct {
		Paths map[string]map[string]struct {
			RequestBody struct {
				Required bool `json:"required"`
				Content  map[string]struct {
					Schema struct {
						Required   []string `json:"required"`
						Properties map[string]struct {
							Type   string `json:"type"`
							Format string `json:"format"`
						} `json:"properties"`
					} `json:"schema"`
				} `json:"content"`
			} `json:"requestBody"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(openapiV2JSON, &doc))
	body := doc.Paths["/v2/spaces/{space_id}/files"]["post"].RequestBody
	require.True(t, body.Required)

	jsonForm, ok := body.Content["application/json"]
	require.True(t, ok)
	assert.Equal(t, []string{"url"}, jsonForm.Schema.Required)
	assert.Equal(t, "string", jsonForm.Schema.Properties["url"].Type)

	multipart, ok := body.Content["multipart/form-data"]
	require.True(t, ok)
	assert.Equal(t, []string{"file"}, multipart.Schema.Required)
	assert.Equal(t, "binary", multipart.Schema.Properties["file"].Format)
}
