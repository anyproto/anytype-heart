package wrapper

// steer_test.go — the argument-diagnosis table and the REST→tool vocabulary
// (steer.go). Every case here is a refusal that was CORRECT and unactionable
// in a live small-model run, and the assertion is that the repair is named.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// the eval account's real ids: a space id is two dot-joined parts, and the
// part after the dot is the base36 replication key — the same for every space
// one account owns, which is why the CID half looks like the whole id.
const (
	evalSpaceId     = "bafyreihwvsaekzzyb54o7um4hdpvpn5b2invn75lmijhhtghblvphxwz2i.28y6mgnwgodt7"
	evalSpacePrefix = "bafyreihwvsaekzzyb54o7um4hdpvpn5b2invn75lmijhhtghblvphxwz2i"
)

// spaceNotFound is the server's own refusal for an unknown space, verbatim.
func spaceNotFound(spaceId string) string {
	return `{"status":404,"code":"not_found","message":"space \"` + spaceId +
		`\" not found — list spaces with GET /v2/spaces"}`
}

// spacesBody renders a stub space list.
func spacesBody(ids ...string) string {
	rows := make([]v2model.SpaceRow, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, v2model.SpaceRow{Id: id, Name: "Space"})
	}
	resp := v2model.ListResponse[v2model.SpaceRow]{Data: rows, Total: len(rows), Limit: 100}
	return mustJSON(resp)
}

// TestSpaceRefSteering pins §8.34's defect 1. `gemma4:e4b` truncated the
// space id at the dot on 74 of 79 find calls — plausibly reading
// `.28y6mgnwgodt7` as a file extension — and earned `space "bafyrei…" not
// found`, which is true and names no repair. In one attempt it called
// `spaces`, was served the full ids, and went straight back to the truncated
// form seven times until the turn budget ended the attempt.
func TestSpaceRefSteering(t *testing.T) {
	ctx := context.Background()

	t.Run("a truncated space id is named as one, with the full id", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/"+evalSpacePrefix+"/search", 404, spaceNotFound(evalSpacePrefix))
		fx.stub("GET /v2/spaces", 200, spacesBody(evalSpaceId, "bafyreiotherspacezzz.28y6mgnwgodt7"))

		_, err := fx.Run(ctx, "find", map[string]any{"space": evalSpacePrefix, "query": "Quarterly"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `space "`+evalSpacePrefix+`" not found`,
			"the server's fact stays — the repair is appended to it, never instead of it")
		assert.Contains(t, err.Error(), evalSpaceId,
			"the repair carries the full id: the model already has everything else right")
		assert.Contains(t, err.Error(), "two parts joined by a dot")
		assert.NotContains(t, err.Error(), spacesListRepair,
			"the specific repair supersedes the generic one — the model had already listed the spaces")
	})

	t.Run("the steer reaches every tool taking space", func(t *testing.T) {
		// describe, not find: the mistake is a property of the ARGUMENT, so
		// it is diagnosed on Run's error path and no executor knows about it
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/"+evalSpacePrefix+"/types/task", 404, spaceNotFound(evalSpacePrefix))
		fx.stub("GET /v2/spaces", 200, spacesBody(evalSpaceId))

		_, err := fx.Run(ctx, "describe", map[string]any{"space": evalSpacePrefix, "type": "task"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), evalSpaceId)
	})

	t.Run("a space id that is nobody's prefix keeps the plain refusal", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/nosuchspace/search", 404, spaceNotFound("nosuchspace"))
		fx.stub("GET /v2/spaces", 200, spacesBody(evalSpaceId))

		_, err := fx.Run(ctx, "find", map[string]any{"space": "nosuchspace", "query": "x"})

		require.Error(t, err)
		assert.NotContains(t, err.Error(), "two parts joined by a dot",
			"the steer names a mistake it can prove — a typo is not this mistake")
	})

	t.Run("the list is read only after the server refuses", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/"+evalSpaceId+"/search", 200, searchResponse(0, false))

		_, err := fx.Run(ctx, "find", map[string]any{"space": evalSpaceId, "query": "x"})

		require.NoError(t, err)
		assert.Empty(t, fx.sent("GET /v2/spaces"),
			"a working call must not pay for the diagnosis of a mistake it did not make")
	})

	t.Run("an unreadable space list leaves the refusal standing", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/"+evalSpacePrefix+"/search", 404, spaceNotFound(evalSpacePrefix))
		fx.stub("GET /v2/spaces", 500, `{"status":500,"code":"internal_error","message":"boom"}`)

		_, err := fx.Run(ctx, "find", map[string]any{"space": evalSpacePrefix, "query": "x"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `space "`+evalSpacePrefix+`" not found`)
		assert.NotContains(t, err.Error(), "boom",
			"the steer is best-effort on top of a refusal that already stands")
	})
}

// TestObjectRefSteering pins §8.33's defect 2. The refusal for a
// block-shaped `object` was `object "767cb" not found in space "bafyrei…"`
// — true, and naming no repair: a small model sent that call three times
// byte-identically and then abandoned the task. The shape is recognisable
// and its repair is known, so the wrapper names it — on Run's error path,
// which is why every tool taking `object` gets it and not just edit_text.
func TestObjectRefSteering(t *testing.T) {
	ctx := context.Background()
	notFound := func(ref string) string {
		return `{"status":404,"code":"not_found","message":"object \"` + ref + `\" not found in space \"space1\""}`
	}

	t.Run("a block reference in object says where a block goes", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		// edit_text with no block sends the id-less PATCH (the server locates,
		// §8.43): the PATCH is the call that 404s on the bad object ref
		fx.stub("PATCH /v2/spaces/space1/objects/767cb", 404, notFound("767cb"))

		_, err := fx.Run(ctx, "edit_text", map[string]any{
			"object": "767cb", "find": "Q3", "replace": "Q4",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `object "767cb" not found in space "space1"`,
			"the server's fact stays — the hint is added to it, never instead of it")
		assert.Contains(t, err.Error(), "that is a block reference: read serves those, and they go in `block`")
		assert.Contains(t, err.Error(), "handle number from the last find")
	})

	t.Run("a full minted block id is the same mistake", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/62d5c4a1b9e04f3a8c7d1e2b", 404, notFound("62d5c4a1b9e04f3a8c7d1e2b"))

		_, err := fx.Run(ctx, "read", map[string]any{"object": "62d5c4a1b9e04f3a8c7d1e2b"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "a block reference belongs in a block argument, not in `object`",
			"read takes no block argument, so the repair names the category and not a slot read does not have")
	})

	t.Run("the space id in object is named as the space id", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(
			v2model.PropertyRow{Key: "done", Name: "Done", Format: "checkbox"}))
		fx.stub("PATCH /v2/spaces/space1/objects/space1", 404, notFound("space1"))

		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "space1", "set": map[string]any{"done": true},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "that is the space id, not an object",
			"the steer reaches every tool taking object, including ones with no block argument")
	})

	t.Run("a TRUNCATED space id in object is named too", func(t *testing.T) {
		// the §8.34 mistake landing in the other id-shaped argument: the
		// value is not the working space, it is its first part
		fx := newFixture(t)
		fx.seedSession(evalSpaceId, Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/"+evalSpaceId+"/objects/"+evalSpacePrefix, 404,
			`{"status":404,"code":"not_found","message":"object \"`+evalSpacePrefix+`\" not found in space \"`+evalSpaceId+`\""}`)

		_, err := fx.Run(ctx, "read", map[string]any{"object": evalSpacePrefix})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "that is the start of the space id",
			"a value that is a prefix of the working space is not an object id that happens to be missing")
	})

	t.Run("a name in object points at find", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/Kimubabe", 404, notFound("Kimubabe"))

		_, err := fx.Run(ctx, "read", map[string]any{"object": "Kimubabe"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "run find with query naming it")
	})

	t.Run("a not-found about anything else is left alone", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 404,
			`{"status":404,"code":"not_found","message":"property \"assignee\" not found in space \"space1\""}`)

		_, err := fx.Run(ctx, "check_item", map[string]any{"object": "1", "block": "e0002", "checked": true})

		require.Error(t, err)
		assert.NotContains(t, err.Error(), "handle number from the last find",
			"the object resolved — a 404 about something else must not be re-explained as a bad object")
	})
}

// TestRestVocabulary pins §8.34's defect 2. The server names routes because
// routes are its vocabulary; a tool-calling caller has tools and no routes,
// so a hint like "list spaces with GET /v2/spaces" tells the model to do
// something it cannot do while the tool that does it goes unnamed — which is
// exactly what the e4b run shows, the model having already called `spaces`.
func TestRestVocabulary(t *testing.T) {
	ctx := context.Background()

	t.Run("the space refusal names the tool, on the tool surface", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/nosuchspace/search", 404, spaceNotFound("nosuchspace"))
		fx.stub("GET /v2/spaces", 200, spacesBody(evalSpaceId))

		_, err := fx.Run(ctx, "find", map[string]any{"space": "nosuchspace", "query": "x"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "list them with the `spaces` tool")
		assert.NotContains(t, err.Error(), "GET /v2/spaces")
	})

	t.Run("the block-not-found hint names read, not the query parameter", func(t *testing.T) {
		// the server's CURRENT phrasing (v2AddressableBlocksHint), which the
		// retired ops-only rewrite did not cover
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 404,
			`{"status":404,"code":"not_found","message":"block \"zzzzz\" not found","issues":[{"path":"ops[0].id","message":"the addressable blocks are the entries of the document's blocks array","hint":"GET the object with ?outline=true to list them. Ids nested inside a block are served but are not block references."}]}`)

		_, err := fx.Run(ctx, "check_item", map[string]any{"object": "1", "block": "zzzzz", "checked": true})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "run read with mode=outline to list the block labels")
		assert.NotContains(t, err.Error(), "?outline=true")
	})

	// the rest of the vocabulary, on the exact strings the server ships: each
	// row is a hint reachable from a route the wrapper calls (routes.go)
	t.Run("every reachable server hint is re-spelled", func(t *testing.T) {
		cases := []struct {
			name, from, want string
		}{
			{
				name: "spaces",
				from: `space "s1" not found — list spaces with GET /v2/spaces`,
				want: "list them with the `spaces` tool",
			},
			{
				name: "type keys (R9 create/search)",
				from: `unknown type key "tsak" (list all with GET /v2/spaces/space1/types)`,
				want: "check the type key (find results show each object's type)",
			},
			{
				name: "type keys (shortcut create, literal placeholder)",
				from: `the shortcut needs a type key (list keys with GET /v2/spaces/{space_id}/types)`,
				want: "check the type key (find results show each object's type)",
			},
			{
				name: "property keys",
				from: `unknown property key "prio" (list all with GET /v2/spaces/space1/properties, or create it with POST /v2/spaces/space1/properties)`,
				want: "run describe on the type to list the property keys it takes",
			},
			{
				name: "option names",
				from: `too many new options in one request (creating an option is permanent and there is no delete surface — check the names against GET /v2/spaces/{space_id}/properties/{property_key}/options, or set values in smaller batches if they are all genuinely new)`,
				want: "check the names against describe, which lists the live option names",
			},
			{
				name: "block ids (retired phrasing)",
				from: `block "zzz" not found — GET the object with ?outline=true to list block ids`,
				want: "run read with mode=outline to list the block labels",
			},
			{
				name: "members",
				from: `the caller's account identity is not available on this server — list members with GET /v2/spaces/{space_id}/members instead`,
				want: "the tool set has no member listing",
			},
			{
				name: "removed property (§8.41)",
				from: `remove "due_date" from the request — values objects already hold stay readable, and reappear if the property is restored; for a different property, list them with GET /v2/spaces/space1/properties`,
				want: "for a different property, run describe on the type to list its live property keys",
			},
			{
				name: "removed type (§8.41)",
				from: `use a live type instead — list them with GET /v2/spaces/space1/types`,
				want: "use a live type instead (find results show each object's type)",
			},
			{
				name: "a hint this table has never seen",
				from: `"image" objects come from file uploads (POST /v2/spaces/{space_id}/files)`,
				want: "the HTTP API",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				te := &ToolError{Status: 404, Text: tc.from}
				deRest(te)
				assert.Contains(t, te.Text, tc.want)
				assert.NotContains(t, te.Text, "/v2/",
					"no route survives the pass — the last rule catches what the table does not name")
			})
		}
	})

	t.Run("issue messages and hints are rewritten too", func(t *testing.T) {
		// the rendered text is built from message + issues, and the JSON
		// channel serves the issues themselves — both have to be clean
		te := &ToolError{
			Status: 400,
			Text:   "unknown property keys",
			Issues: []v2model.Issue{{
				Path:    "/properties/prio",
				Message: `unknown property key "prio" — known keys: status`,
				Hint:    "list all with GET /v2/spaces/space1/properties, or create it with POST /v2/spaces/space1/properties",
			}},
		}
		deRest(te)
		assert.Equal(t, "run describe on the type to list the property keys it takes", te.Issues[0].Hint)
	})

	t.Run("prose that merely mentions a version prefix is untouched", func(t *testing.T) {
		te := &ToolError{Status: 400, Text: "the /v2 surface accepts ops only"}
		deRest(te)
		assert.Equal(t, "the /v2 surface accepts ops only", te.Text,
			"the rule matches a METHOD + route, not the string /v2")
	})

	t.Run("a DOTTED real space id is consumed whole by the fallback", func(t *testing.T) {
		// real space ids carry a dot (bafyreiabc….28y6mgnwgodt7) and the
		// server interpolates them into hints verbatim (list_read, search,
		// refs). The old fallback pattern stopped at the dot, leaving the id
		// tail glued to the replacement: "the HTTP API.28y6mgnwgodt7/…" —
		// invisible to every fixture because they all used the dot-free
		// `space1` (§8.41-9).
		te := &ToolError{Status: 404, Text: `list them with GET /v2/spaces/bafyreiabc.28y6mgnwgodt7/objects`}
		deRest(te)
		assert.Equal(t, "list them with the HTTP API", te.Text,
			"the whole dotted route is replaced, tail included")

		// and a sentence-ending dot still terminates the route
		te = &ToolError{Status: 404, Text: `use GET /v2/spaces/bafyreiabc.28y6mgnwgodt7/objects. Then retry.`}
		deRest(te)
		assert.Equal(t, "use the HTTP API. Then retry.", te.Text,
			"a dot followed by whitespace is prose, not route")
	})
}

// TestRestVocabularyReplacementsAreToolShaped guards the table itself: a
// replacement that reintroduced a route would be re-caught by the generic
// rule and redacted into "the HTTP API", silently losing the repair the row
// exists to give.
func TestRestVocabularyReplacementsAreToolShaped(t *testing.T) {
	for _, sub := range restVocab {
		assert.False(t, strings.Contains(sub.to, "/v2/"),
			"a replacement must not reintroduce a route: %q", sub.to)
	}
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}
