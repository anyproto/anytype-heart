package wrapper

// steer_test.go — the argument-diagnosis table and the REST→tool vocabulary
// (steer.go). Every case here is a refusal that was CORRECT and unactionable
// in a live small-model run, and the assertion is that the repair is named.

import (
	"context"
	"encoding/json"
	"net/http"
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

// spacesListRepair is how the server's space-not-found hint reads on this
// surface; the space steer supersedes it, which these tests pin.
const spacesListRepair = "list spaces with " + spacesToolSpelling

// spaceNotFound is the server's own refusal for an unknown space, verbatim
// (spaceNotFoundError): the fact in the message, the list steer as an issue
// on the parameter, typed.
func spaceNotFound(spaceId string) string {
	return `{"status":404,"code":"not_found","message":"space \"` + spaceId + `\" not found",` +
		`"issues":[{"path":"space_id","message":"no space with this id is open on this account",` +
		`"hint":"list spaces with GET /v2/spaces","see_also":[{"op":"list_spaces"}]}]}`
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
//
// The server ships each route as a typed reference (see_also) beside the
// prose, so the re-spelling is a lookup on the operation, not a regex on
// the sentence.
func TestRestVocabulary(t *testing.T) {
	ctx := context.Background()

	t.Run("the space refusal names the tool, on the tool surface", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/nosuchspace/search", 404, spaceNotFound("nosuchspace"))
		fx.stub("GET /v2/spaces", 200, spacesBody(evalSpaceId))

		_, err := fx.Run(ctx, "find", map[string]any{"space": "nosuchspace", "query": "x"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), spacesListRepair)
		assert.NotContains(t, err.Error(), "GET /v2/spaces")
	})

	t.Run("the block-not-found hint names read, not the query parameter", func(t *testing.T) {
		// the server's CURRENT phrasing (addressableBlocksHint), typed
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 404,
			`{"status":404,"code":"not_found","message":"block \"zzzzz\" not found","issues":[{"path":"ops[0].id","message":"the addressable blocks are the entries of the document's blocks array","hint":"GET /v2/spaces/{space_id}/objects/{object_id}?outline=true lists them. Ids nested inside a block are served but are not block references.","see_also":[{"op":"get_object","query":{"outline":"true"}}]}]}`)

		_, err := fx.Run(ctx, "check_item", map[string]any{"object": "1", "block": "zzzzz", "checked": true})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "`read` with mode=outline lists them",
			"the outline is the one read that lists the blocks of EVERY object — the full read of a query or collection serves its rows instead")
		assert.NotContains(t, err.Error(), "?outline=true")
		assert.NotContains(t, err.Error(), "/v2/")
	})

	// the vocabulary, on the exact shapes the server ships: each case is a
	// hint reachable from a route the wrapper calls (routes.go), with the
	// references the server attaches to it
	t.Run("every reachable server hint is re-spelled by its references", func(t *testing.T) {
		cases := []struct {
			name  string
			issue v2model.Issue
			want  string
		}{
			{
				name:  "spaces",
				issue: v2model.Issue{}.Hintf("list spaces with %s", v2model.RefListSpaces()),
				want:  "list spaces with the `spaces` tool",
			},
			{
				name:  "type keys (R9 create/search)",
				issue: v2model.Issue{}.Hintf("list all with %s", v2model.RefListTypes("space1")),
				want:  "list all with a type listing (not in this tool set; `find` results show each object's type)",
			},
			{
				name:  "type keys, parameter unbound (placeholder route)",
				issue: v2model.Issue{}.Hintf("list keys with %s", v2model.NewRef(v2model.OpListTypes)),
				want:  "list keys with a type listing (not in this tool set; `find` results show each object's type)",
			},
			{
				name: "property keys — the create half is not on this surface, and says so by name",
				issue: v2model.Issue{}.Hintf("list all with %s, or create it with %s",
					v2model.RefListProperties("space1"), v2model.RefCreateProperty("space1")),
				want: "list all with `describe` on the type (which lists up to 120 property names), or create it with an operation outside this tool set (create_property)",
			},
			{
				name:  "option names, property bound",
				issue: v2model.Issue{}.Hintf("check the names against %s", v2model.RefListPropertyOptions("space1", "Status")),
				want:  "check the names against `describe` with options=Status",
			},
			{
				name:  "option names, property unbound",
				issue: v2model.Issue{}.Hintf("check the names against %s", v2model.NewRef(v2model.OpListPropertyOptions, "space_id", "space1")),
				want:  "check the names against `describe` with options naming the property",
			},
			{
				name:  "members — not offered",
				issue: v2model.Issue{}.Hintf("list members with %s instead", v2model.RefListMembers("space1")),
				want:  "list members with an operation outside this tool set (list_members) instead",
			},
			{
				name: "the locator's two reads: the outline read extends the plain one and is replaced whole",
				issue: v2model.Issue{}.Hintf("read the object with %s and copy the text; %s truncates text to a snippet",
					v2model.RefGetObject("space1", "obj1"), v2model.RefGetObject("space1", "obj1").With("outline", "true")),
				want: "read the object with `read` and copy the text; `read` with mode=outline truncates text to a snippet",
			},
			{
				name:  "resend with a parameter",
				issue: v2model.Issue{}.Hintf("or resend with %s to create it", v2model.Resend("create_missing_options", "true")),
				want:  "or resend with a parameter these tools do not take (create_missing_options=true) to create it",
			},
			{
				name:  "a dotted real space id inside a bound route is consumed with it",
				issue: v2model.Issue{}.Hintf("list them with %s", v2model.RefListObjects("bafyreiabc.28y6mgnwgodt7")),
				want:  "list them with `find` (which serves handles, not full ids, on this tool set)",
			},
			{
				name:  "a full-id read is a shape `read` cannot ask for, and says so",
				issue: v2model.Issue{}.Hintf("read it with %s", v2model.NewRef(v2model.OpGetObject).With("ids", "full")),
				want:  "read it with a full-id read (not in this tool set)",
			},
			{
				name:  "query and collection reads are `read` on the list object",
				issue: v2model.Issue{}.Hintf("use %s", v2model.RefGetCollectionObjects("space1", "col1")),
				want:  "use `read` on the collection",
			},
			{
				name: "one pass: a bound value that spells another reference is not rewritten again",
				issue: v2model.Issue{}.Hintf("check %s, or resend with %s",
					v2model.RefListPropertyOptions("space1", "?create_missing_options=true"), v2model.Resend("create_missing_options", "true")),
				want: "check `describe` with options=?create_missing_options=true, or resend with a parameter these tools do not take (create_missing_options=true)",
			},
			{
				name:  "a hint from a server build without references falls to the catch-all",
				issue: v2model.Issue{Hint: `"image" objects come from file uploads (POST /v2/spaces/{space_id}/files)`},
				want:  `"image" objects come from file uploads (the HTTP API)`,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				issues := []v2model.Issue{tc.issue}
				te := &ToolError{Status: 400, Message: "refused", Issues: issues, Text: renderErrorText("refused", issues)}
				deRest(te)
				assert.Equal(t, tc.want, te.Issues[0].Hint)
				assert.Contains(t, te.Text, tc.want, "the rendered text is rewritten in place by the same references")
				assert.NotContains(t, te.Text, "/v2/",
					"no route survives the pass outside a protected span — the catch-all takes what the references do not name")
				assert.Nil(t, te.Issues[0].SeeAlso, "rendered references are dropped from the JSON channel")
			})
		}
	})

	t.Run("issue messages and hints are rewritten too", func(t *testing.T) {
		// the rendered text is built from message + issues, and the JSON
		// channel serves the issues themselves — both have to be clean
		issues := []v2model.Issue{v2model.Issue{
			Path:    "/properties/prio",
			Message: `unknown property key "prio" — known keys: status`,
		}.Hintf("list all with %s", v2model.RefListProperties("space1"))}
		te := &ToolError{Status: 400, Message: "unknown property keys", Issues: issues, Text: renderErrorText("unknown property keys", issues)}
		deRest(te)
		assert.Equal(t, "list all with `describe` on the type (which lists up to 120 property names)", te.Issues[0].Hint)
		assert.Equal(t, "unknown property keys\n  /properties/prio: unknown property key \"prio\" — known keys: status (list all with `describe` on the type (which lists up to 120 property names))", te.Text)
	})

	t.Run("a message is a fact and is not rewritten by the hint's references", func(t *testing.T) {
		issues := []v2model.Issue{v2model.Issue{
			Path:    "/properties/x",
			Message: `property "GET /v2/spaces/space1/properties" has no option named "a"`,
		}.Hintf("list them with %s", v2model.RefListProperties("space1"))}
		te := &ToolError{Status: 400, Message: "refused", Issues: issues, Text: renderErrorText("refused", issues)}
		deRest(te)
		assert.NotContains(t, te.Issues[0].Message, "`describe`",
			"only the catch-all touches a message — a quoted value is never re-spelled as a tool")
		assert.Contains(t, te.Issues[0].Message, "the HTTP API")
		assert.Equal(t, "list them with `describe` on the type (which lists up to 120 property names)", te.Issues[0].Hint)
	})

	t.Run("the server's own envelope round-trips: marshal → decode → re-spell", func(t *testing.T) {
		// built with the server's constructors and marshalled by its
		// MarshalJSON, so this fixture cannot drift from the wire shape the
		// way a hand-written JSON literal can
		served := v2model.NotFound(`space "bafyreiabc.28y6mgnwgodt7" not found`,
			v2model.Issue{Path: "space_id", Message: "no space with this id is open on this account"}.
				Hintf("list spaces with %s", v2model.RefListSpaces()))
		body, err := json.Marshal(served)
		require.NoError(t, err)

		te := decodeAPIError(http.StatusNotFound, body).(*ToolError)
		deRest(te)

		assert.Equal(t, "space \"bafyreiabc.28y6mgnwgodt7\" not found\n  space_id: no space with this id is open on this account (list spaces with the `spaces` tool)", te.Text)
		assert.Nil(t, te.Issues[0].SeeAlso)
	})

	t.Run("a bound value that looks like a route is not redacted by the catch-all — in the text either", func(t *testing.T) {
		issue := v2model.Issue{Path: "/x", Message: "m"}.Hintf("check the names against %s", v2model.RefListPropertyOptions("space1", "GET /v2/spaces/space1/properties"))
		want := "check the names against `describe` with options=GET /v2/spaces/space1/properties"
		assert.Equal(t, want, deRestIssue(issue).Hint,
			"the catch-all sees only the prose between the renderings, never a tool spelling")

		issues := []v2model.Issue{issue}
		te := &ToolError{Status: 400, Message: "refused", Issues: issues, Text: renderErrorText("refused", issues)}
		deRest(te)
		assert.Equal(t, "refused\n  /x: m ("+want+")", te.Text)
	})

	t.Run("a message that quotes the hint's own text is not rewritten as a hint", func(t *testing.T) {
		hint := v2model.Issue{}.Hintf("list all with %s", v2model.RefListTypes("s")).Hint
		issues := []v2model.Issue{v2model.Issue{Path: "/type", Message: `unknown type "` + hint + `"`}.Hintf("list all with %s", v2model.RefListTypes("s"))}
		te := &ToolError{Status: 400, Message: `type "` + hint + `" not found`, Issues: issues, Text: renderErrorText(`type "`+hint+`" not found`, issues)}
		deRest(te)
		assert.NotContains(t, te.Message, "a type listing", "the quoted input is the caller's value, redacted by the catch-all only")
		assert.Contains(t, te.Message, `type "list all with the HTTP API`)
		assert.Equal(t, 1, strings.Count(te.Text, "a type listing (not in this tool set"), "only the rendered hint span is re-spelled")
	})

	t.Run("an executor's edit to the text survives the hint substitution", func(t *testing.T) {
		issues := []v2model.Issue{v2model.Issue{Path: "ops[0].id", Message: "not found"}.
			Hintf("list keys with %s", v2model.RefListProperties("space1"))}
		te := &ToolError{Status: 404, Message: "refused", Issues: issues, Text: renderErrorText("refused", issues)}
		te.Text = strings.Replace(te.Text, "ops[0].id", "block", 1) + " — wrote 1 of 3"
		deRest(te)
		assert.Equal(t, "refused\n  block: not found (list keys with `describe` on the type (which lists up to 120 property names)) — wrote 1 of 3", te.Text)
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

	t.Run("success-path warnings render their hint, re-spelled", func(t *testing.T) {
		// a warning's repair lives in its hint (the query and collection
		// reads put it there); printing the message alone hid it from every
		// MCP caller
		warnings := []v2model.Issue{
			v2model.Issue{Message: "the view's filter was not applied"}.Hintf("list keys with %s", v2model.RefListProperties("space1")),
			{Message: "plain"},
		}
		assert.Equal(t, "\nwarning: the view's filter was not applied — list keys with `describe` on the type (which lists up to 120 property names)\nwarning: plain",
			warningsText(warnings))
	})
}

// TestToolVocabularyIsRouteFree guards the table and its fallback: for every
// operation the API has, the spelling must carry no route — a row that
// reintroduced one would be re-caught by the catch-all and redacted into
// "the HTTP API", silently losing the repair the row exists to give — and an
// operation with no row must be named, so the caller learns which repair is
// not on this surface instead of guessing.
func TestToolVocabularyIsRouteFree(t *testing.T) {
	for _, op := range v2model.OperationIds() {
		spelling := toolSpelling(v2model.NewRef(op))
		assert.NotContains(t, spelling, "/v2/", "op %s", op)
		assert.NotEmpty(t, spelling, "op %s", op)
		if _, offered := toolVocab[op]; !offered {
			assert.Contains(t, spelling, op, "an operation outside the tool set is named")
		}
	}
	assert.NotContains(t, toolSpelling(v2model.RefGetObject("s", "o").With("outline", "true")), "outline=true",
		"the outline query renders as the tool's mode, not as a query parameter")
}

// TestWrapperAsksForNameVocabulary: the wrapper does ZERO translation of
// server errors' key vocabulary (APIV2_VOCABULARY.md layer 3) — instead it
// asks every mutation and search for ?keys=name, and the SERVER spells its
// referential refusals (known lists, did-you-mean) in names natively
// (§4.3). What this layer owes is the parameter on the wire and the text
// served verbatim.
func TestWrapperAsksForNameVocabulary(t *testing.T) {
	fx := newFixture(t)
	fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
	fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(
		v2model.PropertyRow{Key: "due_date", Name: "Due date", Format: "date"},
		v2model.PropertyRow{Key: "status", Name: "Status", Format: "select"},
	))
	// the server's own name-mode refusal (?keys=name — v2service refs.go):
	// names in the phrase, the known list and the suggestion
	fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 400,
		`{"status":400,"code":"validation_failed","message":"unknown properties","issues":[{"path":"/properties/prio","message":"unknown property \"prio\" — known properties: Due date, Status","hint":"did you mean Status?"}]}`)

	_, err := fx.Run(context.Background(), "set_properties", map[string]any{
		"object": "1", "set": map[string]any{"prio": "high"},
	})

	require.Error(t, err)
	sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
	require.Len(t, sent, 1)
	assert.Equal(t, "name", sent[0].Query.Get("keys"),
		"every mutation asks for the name vocabulary — that request is what buys name-mode errors")
	assert.Contains(t, err.Error(), `unknown property "prio" — known properties: Due date, Status`,
		"the server's name-mode text is served verbatim — no wrapper-side re-spelling")
	assert.Contains(t, err.Error(), "did you mean Status?")
	assert.NotContains(t, err.Error(), "property key")
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}
