package wrapper

// tools_crossspace_test.go — find without a space searches every loaded
// space, and its handles carry their space; find type=type lists the types;
// the space-addressed tools fall back to the working space and refuse with
// the space list when there is none. The run that motivated all three is
// in APIV2.md §8.58: "find it" with no space named, and a type name the
// model had no way to learn.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

const twoSpacesBody = `{"data":[{"id":"xjwg44","name":"Soft motion"},{"id":"hpujze","name":"Weekend Trips"}],"total":2,"offset":0,"limit":100,"has_more":false}`

func TestFindAcrossSpaces(t *testing.T) {
	ctx := context.Background()

	t.Run("no space searches every space and names each row's space", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.stub("POST /v2/search", 200, searchResponse(2, false,
			v2model.ObjectRow{Id: "bafytrip", Name: "Prague trip", Type: "page", SpaceId: "hpujze"},
			v2model.ObjectRow{Id: "bafynote", Name: "Prague notes", Type: "note", SpaceId: "xjwg44"},
		))
		fx.stub("GET /v2/spaces", 200, twoSpacesBody)
		fx.stub("GET /v2/spaces/hpujze/types", 200, `{"data":[{"key":"page","name":"Page"}],"total":1,"offset":0,"limit":500,"has_more":false}`)
		fx.stub("GET /v2/spaces/xjwg44/types", 200, `{"data":[{"key":"note","name":"Note"}],"total":1,"offset":0,"limit":500,"has_more":false}`)

		// when
		result, err := fx.Run(ctx, "find", map[string]any{"query": "prague trip"})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, "1. Prague trip (Page, in Weekend Trips)")
		assert.Contains(t, result.Text, "2. Prague notes (Note, in Soft motion)")
		assert.Contains(t, result.Text, "2 matches across all spaces — add space to search one")
		sent := fx.sent("POST /v2/search")
		require.Len(t, sent, 1)
		assert.Equal(t, "prague trip", bodyJSON(t, sent[0])["query"])
		assert.Empty(t, fx.sent("POST /v2/spaces/hpujze/search"), "the global route, not a guessed space")

		session, _ := fx.store.Load()
		assert.Empty(t, session.Space, "a cross-space find sets no working space")
		require.Len(t, session.Handles, 2)
		assert.Equal(t, "hpujze", session.Handles[0].Space, "each handle carries its own space")
		js, ok := result.JSON.(findResult)
		require.True(t, ok)
		assert.True(t, js.Global)
	})

	t.Run("a handle from a cross-space find resolves through its own space", func(t *testing.T) {
		// given
		fx := newFixture(t)
		require.NoError(t, fx.store.Save(&Session{Handles: []Handle{
			{N: 1, Id: "bafytrip", Name: "Prague trip", Type: "page", Space: "hpujze"},
			{N: 2, Id: "bafynote", Name: "Prague notes", Type: "note", Space: "xjwg44"},
		}}))
		fx.stub("GET /v2/spaces/xjwg44/objects/bafynote", 200, testFullDoc)
		fx.stub("PATCH /v2/spaces/hpujze/objects/bafytrip", 200, editOKBody)

		// when
		_, err := fx.Run(ctx, "read", map[string]any{"object": "2"})
		require.NoError(t, err)
		_, err = fx.Run(ctx, "add_blocks", map[string]any{"object": "1", "markdown": "- [ ] pack"})
		require.NoError(t, err)

		// then
		assert.Len(t, fx.sent("GET /v2/spaces/xjwg44/objects/bafynote"), 1)
		assert.Len(t, fx.sent("PATCH /v2/spaces/hpujze/objects/bafytrip"), 1)
	})

	t.Run("a space that disagrees with the handle's own space is refused", func(t *testing.T) {
		fx := newFixture(t)
		require.NoError(t, fx.store.Save(&Session{Handles: []Handle{{N: 1, Id: "bafytrip", Space: "hpujze"}}}))

		_, err := fx.Run(ctx, "read", map[string]any{"object": "1", "space": "xjwg44"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `belongs to the last find in space "hpujze"`)
	})

	t.Run("the partial-results warning reaches the text", func(t *testing.T) {
		// given: a space store still loading
		fx := newFixture(t)
		resp := v2model.ListResponse[v2model.ObjectRow]{
			Data:     []v2model.ObjectRow{{Id: "bafytrip", Name: "Prague trip", Type: "page", SpaceId: "hpujze"}},
			Total:    1,
			Warnings: []v2model.Issue{{Message: "Search results are incomplete because some space stores are still loading or unavailable."}},
		}
		body, err := json.Marshal(resp)
		require.NoError(t, err)
		fx.stub("POST /v2/search", 200, string(body))
		fx.stub("GET /v2/spaces", 200, twoSpacesBody)
		fx.stub("GET /v2/spaces/hpujze/types", 200, `{"data":[],"total":0,"offset":0,"limit":500,"has_more":false}`)

		// when
		result, err := fx.Run(ctx, "find", map[string]any{"query": "prague"})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, "warning: Search results are incomplete")
	})

	t.Run("no space and no criterion lists across spaces, unnumbered", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.stub("POST /v2/search", 200, searchResponse(1, false,
			v2model.ObjectRow{Id: "bafytrip", Name: "Prague trip", Type: "page", SpaceId: "hpujze"}))
		fx.stub("GET /v2/spaces", 200, twoSpacesBody)
		fx.stub("GET /v2/spaces/hpujze/types", 200, `{"data":[],"total":0,"offset":0,"limit":500,"has_more":false}`)

		// when
		result, err := fx.Run(ctx, "find", map[string]any{})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, "listing of recent objects across your spaces")
		assert.NotContains(t, result.Text, "1. ")
		session, _ := fx.store.Load()
		assert.Empty(t, session.Handles)
	})

	t.Run("@me in a filter needs a space", func(t *testing.T) {
		fx := newFixture(t)

		_, err := fx.Run(ctx, "find", map[string]any{"filter": `Assignee = "@me"`})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "needs a space")
		assert.Empty(t, fx.sent("POST /v2/search"))
	})
}

func TestFindListsTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("type=type lists the space's types, labelled Type", func(t *testing.T) {
		// given: the server answers the type-of-types scope with type objects
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(2, false,
			v2model.ObjectRow{Id: "type-page", Name: "Page", Type: "object_type"},
			v2model.ObjectRow{Id: "type-trip", Name: "Trip", Type: "object_type"},
		))
		fx.stub("GET /v2/spaces/space1/types", 200, `{"data":[{"key":"page","name":"Page"},{"key":"trip","name":"Trip"}],"total":2,"offset":0,"limit":500,"has_more":false}`)

		// when
		result, err := fx.Run(ctx, "find", map[string]any{"space": "space1", "type": "type"})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, "1. Page (Type)")
		assert.Contains(t, result.Text, "2. Trip (Type)")
		assert.Equal(t, "type", bodyJSON(t, fx.sent("POST /v2/spaces/space1/search")[0])["type"], "the word reaches the server as written — it resolves there")
	})

	t.Run("type=type with no space lists types across spaces", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/search", 200, searchResponse(1, false,
			v2model.ObjectRow{Id: "type-trip", Name: "Trip", Type: "object_type", SpaceId: "hpujze"}))
		fx.stub("GET /v2/spaces", 200, twoSpacesBody)
		fx.stub("GET /v2/spaces/hpujze/types", 200, `{"data":[{"key":"trip","name":"Trip"}],"total":1,"offset":0,"limit":500,"has_more":false}`)

		result, err := fx.Run(ctx, "find", map[string]any{"type": "type"})

		require.NoError(t, err)
		assert.Contains(t, result.Text, "1. Trip (Type, in Weekend Trips)")
	})
}

const pageTypeDoc = `{"formatVersion":"2.0","kind":"object_type","properties":{"name":"Page"},"type_settings":{"api_key":"page","property_definitions":[]}}`

func TestSpaceDefaults(t *testing.T) {
	ctx := context.Background()

	t.Run("describe uses the working space when no space is given", func(t *testing.T) {
		// given: a find set the working space
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "obj1"})
		fx.stub("GET /v2/spaces/space1/types/Page", 200, pageTypeDoc)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse())

		// when
		result, err := fx.Run(ctx, "describe", map[string]any{"type": "Page"})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, "type Page")
		require.Len(t, fx.sent("GET /v2/spaces/space1/types/Page"), 1)
	})

	t.Run("create uses the working space when no space is given", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "obj1"})
		fx.stub("POST /v2/spaces/space1/objects", 200, `{"id":"bafynew","type":"page"}`)

		// when
		result, err := fx.Run(ctx, "create", map[string]any{"type": "page", "name": "Packing list"})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, "created bafynew")
		require.Len(t, fx.sent("POST /v2/spaces/space1/objects"), 1)
	})

	t.Run("with no space anywhere the refusal lists the spaces", func(t *testing.T) {
		// given: a fresh session, the state of every first call
		fx := newFixture(t)
		fx.stub("GET /v2/spaces", 200, twoSpacesBody)

		// when
		_, err := fx.Run(ctx, "create", map[string]any{"type": "page", "name": "Packing list"})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create needs a space and none is known yet — pass one of these as space: Soft motion — xjwg44; Weekend Trips — hpujze")
		assert.Empty(t, fx.sent("POST /v2/spaces/space1/objects"), "nothing was created")
		var argErr *ArgumentError
		assert.False(t, errors.As(err, &argErr), "a missing space is a workspace fact, not a shape mistake")
	})

	t.Run("a cross-space find sets no working space, so create still asks for one", func(t *testing.T) {
		// given
		fx := newFixture(t)
		require.NoError(t, fx.store.Save(&Session{Handles: []Handle{{N: 1, Id: "bafytrip", Space: "hpujze"}}}))
		fx.stub("GET /v2/spaces", 200, twoSpacesBody)

		// when
		_, err := fx.Run(ctx, "create_type", map[string]any{"name": "Trip"})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create_type needs a space and none is known yet")
	})

	t.Run("a spaces row handed back as space still resolves", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/hpujze/objects", 200, `{"id":"bafynew","type":"page"}`)

		_, err := fx.Run(ctx, "create", map[string]any{"space": "Weekend Trips — hpujze", "type": "page", "name": "X"})

		require.NoError(t, err)
		require.Len(t, fx.sent("POST /v2/spaces/hpujze/objects"), 1)
	})
}

func TestManifestInstructionsAndExamples(t *testing.T) {
	t.Run("every tier serves its instructions in the manifest", func(t *testing.T) {
		for _, tier := range []Tier{TierSmall, TierLarge} {
			m, err := BuildManifestForTier(tier)
			require.NoError(t, err)
			assert.Equal(t, tierInstructions(tier), m.Instructions, "the manifest and MCP initialize speak the same words (%s)", tier)
			assert.Contains(t, m.Instructions, "SQL WHERE clause", "the filter syntax is named, not spelled out")
			assert.Contains(t, m.Instructions, "find type=type lists the types")
		}
	})

	t.Run("the small tier's filter examples name no type and no invented vocabulary", func(t *testing.T) {
		m, err := BuildManifestForTier(TierSmall)
		require.NoError(t, err)
		assert.Equal(t, smallFilterExamples, m.FilterGrammar.Examples)
		for _, ex := range m.FilterGrammar.Examples {
			assert.NotContains(t, ex, "task", "an example is read as the vocabulary by a 3B model")
			assert.NotContains(t, ex, "type IN")
		}
		large, err := BuildManifestForTier(TierLarge)
		require.NoError(t, err)
		assert.NotEqual(t, smallFilterExamples, large.FilterGrammar.Examples, "the large tier keeps the parser's full example list")
	})

	t.Run("no example teaches Task as a type", func(t *testing.T) {
		for _, tool := range Tools() {
			if v, ok := tool.Example["type"]; ok {
				assert.NotEqual(t, "Task", v, "%s's example names Task — the one type name in a 3B model's context becomes its vocabulary", tool.Name)
			}
			assert.NotContains(t, tool.Description, "e.g. Task")
			for _, a := range tool.Args {
				assert.NotContains(t, a.Description, "e.g. Task", "%s.%s", tool.Name, a.Name)
			}
		}
	})

	t.Run("the small tier's fixed cost does not grow", func(t *testing.T) {
		// the bytes a host puts in front of the model before the user's
		// first word: tool text, the served instructions and the filter
		// examples. Measured at 8.0 KB of tool text on a 4,096-token model
		// (APIV2.md §8.58); this ceiling stops growth until the tiered
		// descriptions decision lowers it.
		const ceiling = 10 * 1024
		m, err := BuildManifestForTier(TierSmall)
		require.NoError(t, err)
		total := len(m.Instructions)
		for _, tool := range m.Tools {
			total += len(tool.Description) + len(tool.Parameters) + len(tool.Example)
		}
		for _, ex := range m.FilterGrammar.Examples {
			total += len(ex)
		}
		assert.LessOrEqual(t, total, ceiling, "the small tier's fixed context cost grew past the ceiling")
	})
}
