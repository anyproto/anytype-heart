package wrapper

// tools_crossobject_test.go — the two repairs to cross-object work: an
// objects-format property value addressed the way every OTHER slot on this
// surface is addressed (a handle number, an exact name), and one
// set_properties call that writes to several objects.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// propsChangedBody is a one-property-changed edit receipt.
const propsChangedBody = `{"diff_stats":{"blocks_added":0,"blocks_removed":0,"blocks_changed":0,"blocks_moved":0,"properties_changed":1}}`

// typesWithMemberBody names the two types the ambiguity refusal spells.
const typesWithMemberBody = `{"data":[{"key":"participant","name":"Member"},{"key":"contact","name":"Contact"}],"total":2,"offset":0,"limit":500,"has_more":false}`

func TestObjectValueAddressing(t *testing.T) {
	ctx := context.Background()

	t.Run("an exact name resolves to the object's id", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafytask", Name: "Ship it"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false,
			v2model.ObjectRow{Id: "bafyalex", Name: "Alex", Type: "participant"}))
		fx.stub("PATCH /v2/spaces/space1/objects/bafytask", 200, propsChangedBody)
		want := map[string]any{"assignee": "bafyalex"}

		// when
		result, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1", "set": map[string]any{"Assignee": "Alex"},
		})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, `ok — "Ship it"`)
		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafytask")
		require.Len(t, sent, 1)
		assert.Equal(t, want, firstOp(t, sent[0])["set"])

		lookups := fx.sent("POST /v2/spaces/space1/search")
		require.Len(t, lookups, 1)
		body := bodyJSON(t, lookups[0])
		assert.Equal(t, `name = "Alex"`, body["filter"],
			"the lookup is an equality filter — full text ranks and stems, and this promises an EXACT name")
		assert.NotContains(t, body, "query")
	})

	t.Run("a handle number resolves without any lookup", func(t *testing.T) {
		// given: the find that numbered the object is the whole address
		fx := newFixture(t)
		fx.seedSession("space1",
			Handle{N: 1, Id: "bafytask", Name: "Ship it"},
			Handle{N: 2, Id: "bafyalex", Name: "Alex", Type: "participant"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafytask", 200, propsChangedBody)
		want := map[string]any{"assignee": "bafyalex"}

		// when
		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1", "set": map[string]any{"Assignee": "2"},
		})

		// then
		require.NoError(t, err)
		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafytask")
		require.Len(t, sent, 1)
		assert.Equal(t, want, firstOp(t, sent[0])["set"])
		assert.Empty(t, fx.sent("POST /v2/spaces/space1/search"),
			"a handle is the session's own numbering — nothing to look up")
	})

	t.Run("an id the session already served is never re-read as a name", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1",
			Handle{N: 1, Id: "bafytask", Name: "Ship it"},
			Handle{N: 2, Id: "bafyalex", Name: "Alex"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafytask", 200, propsChangedBody)
		want := map[string]any{"assignee": "bafyalex"}

		// when
		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1", "set": map[string]any{"Assignee": "bafyalex"},
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, want, firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafytask")[0])["set"])
		assert.Empty(t, fx.sent("POST /v2/spaces/space1/search"), "an id IS the address")
	})

	t.Run("a name nothing answers to passes through for the server to refuse", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafytask"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(0, false))
		fx.stub("PATCH /v2/spaces/space1/objects/bafytask", 200, propsChangedBody)
		want := map[string]any{"assignee": "bafyunknown"}

		// when
		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1", "set": map[string]any{"Assignee": "bafyunknown"},
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, want, firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafytask")[0])["set"],
			"the value reaches the server byte for byte — its not-found refusal is the one that stands")
	})

	t.Run("a name several objects answer to is refused with the candidates", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1",
			Handle{N: 1, Id: "bafytask"},
			Handle{N: 2, Id: "bafyalex1", Name: "Alex", Type: "participant"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(2, false,
			v2model.ObjectRow{Id: "bafyalex1", Name: "Alex", Type: "participant"},
			v2model.ObjectRow{Id: "bafyalex2", Name: "Alex", Type: "contact"}))
		fx.stub("GET /v2/spaces/space1/types", 200, typesWithMemberBody)

		// when
		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1", "set": map[string]any{"Assignee": "Alex"},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `value of "Assignee"`)
		assert.Contains(t, err.Error(), `"Alex" names 2 objects`)
		assert.Contains(t, err.Error(), "handle 2 (Member)",
			"a candidate the last find numbered is offered by its number — the cheapest address here")
		assert.Contains(t, err.Error(), "bafyalex2 (Contact)")
		assert.Empty(t, fx.sent("PATCH /v2/spaces/space1/objects/bafytask"),
			"an ambiguous reference is refused, never guessed — and nothing is written")
	})

	t.Run("add and remove resolve the same way as set", func(t *testing.T) {
		// given: remove skips the option guard but still resolves references
		// — an entry that does not match removes nothing
		fx := newFixture(t)
		fx.seedSession("space1",
			Handle{N: 1, Id: "bafytask"},
			Handle{N: 2, Id: "bafymo", Name: "Mo"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false,
			v2model.ObjectRow{Id: "bafyalex", Name: "Alex", Type: "participant"}))
		fx.stub("PATCH /v2/spaces/space1/objects/bafytask", 200, propsChangedBody)

		// when
		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1",
			"add":    map[string]any{"Assignee": []any{"Alex"}},
			"remove": map[string]any{"Assignee": []any{"2"}},
		})

		// then
		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafytask")[0])
		assert.Equal(t, map[string]any{"assignee": []any{"bafyalex"}}, op["add"])
		assert.Equal(t, map[string]any{"assignee": []any{"bafymo"}}, op["remove"])
	})

	t.Run("create and add resolve the same way", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false,
			v2model.ObjectRow{Id: "bafyalex", Name: "Alex", Type: "participant"}))
		fx.stub("POST /v2/spaces/space1/objects", 200, `{"id":"bafynew","type":"task"}`)
		fx.stub("GET /v2/spaces/space1/types", 200, typesWithMemberBody)
		want := map[string]any{"assignee": []any{"bafyalex"}}

		// when
		_, err := fx.Run(ctx, "create", map[string]any{
			"space": "space1", "type": "task", "name": "Ship it",
			"properties": map[string]any{"Assignee": []any{"Alex"}},
		})

		// then
		require.NoError(t, err)
		sent := fx.sent("POST /v2/spaces/space1/objects")
		require.Len(t, sent, 1)
		assert.Equal(t, want, bodyJSON(t, sent[0])["properties"],
			"a list of names resolves entry by entry")
	})
}

func TestSetPropertiesOverSeveralObjects(t *testing.T) {
	ctx := context.Background()

	// seedThree installs three numbered objects and the property stubs one
	// set_properties call needs.
	seedThree := func(fx *fixture) {
		fx.seedSession("space1",
			Handle{N: 1, Id: "bafyo1", Name: "A"},
			Handle{N: 2, Id: "bafyo2", Name: "B"},
			Handle{N: 3, Id: "bafyo3", Name: "C"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("GET /v2/spaces/space1/properties/status/options", 200,
			`{"data":[{"name":"Done"}],"total":1,"offset":0,"limit":50,"has_more":false}`)
	}

	t.Run("one call writes the same change to three objects", func(t *testing.T) {
		// given
		fx := newFixture(t)
		seedThree(fx)
		for _, id := range []string{"bafyo1", "bafyo2", "bafyo3"} {
			fx.stub("PATCH /v2/spaces/space1/objects/"+id, 200, propsChangedBody)
		}
		want := map[string]any{"status": "Done"}

		// when
		result, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1,2,3", "set": map[string]any{"Status": "Done"},
		})

		// then
		require.NoError(t, err)
		for _, id := range []string{"bafyo1", "bafyo2", "bafyo3"} {
			sent := fx.sent("PATCH /v2/spaces/space1/objects/" + id)
			require.Len(t, sent, 1, "object %s written exactly once", id)
			assert.Equal(t, want, firstOp(t, sent[0])["set"])
		}
		assert.Contains(t, result.Text, `ok — "A": 1 properties changed`)
		assert.Contains(t, result.Text, `ok — "C": 1 properties changed`)
		assert.Contains(t, result.Text, "3 objects written")
		assert.Len(t, fx.sent("GET /v2/spaces/space1/properties"), 1,
			"one property index and one option check serve the whole list")
		assert.Len(t, fx.sent("GET /v2/spaces/space1/properties/status/options"), 1)
	})

	t.Run("a reference that does not resolve refuses before the first write", func(t *testing.T) {
		// given: handle 9 was never numbered
		fx := newFixture(t)
		seedThree(fx)

		// when
		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1,9,3", "set": map[string]any{"Status": "Done"},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nothing was written")
		assert.Contains(t, err.Error(), "no handle 9")
		assert.Empty(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyo1"),
			"every reference resolves before any object is touched")
		assert.Empty(t, fx.sent("GET /v2/spaces/space1/properties"),
			"and before the property index is even fetched")
	})

	t.Run("a failure part-way through reports what was written and what was not", func(t *testing.T) {
		// given: the second object is gone (there is no cross-object
		// transaction — the first write stands)
		fx := newFixture(t)
		seedThree(fx)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyo1", 200, propsChangedBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyo2", 404,
			`{"status":404,"code":"not_found","message":"object \"bafyo2\" not found"}`)

		// when
		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1,2,3", "set": map[string]any{"Status": "Done"},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `wrote 1 of 3 ("A")`)
		assert.Contains(t, err.Error(), `"B" was not written`)
		assert.Contains(t, err.Error(), "nothing rolls back")
		assert.Contains(t, err.Error(), `"C" were not attempted`)
		assert.Contains(t, err.Error(), `object "bafyo2" not found`, "the server's own words survive")
		assert.Len(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyo1"), 1, "the first write stands")
		assert.Empty(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyo3"),
			"stopping keeps the outcome a prefix the caller can re-send the tail of")
	})

	t.Run("an object listed twice is refused", func(t *testing.T) {
		// given
		fx := newFixture(t)
		seedThree(fx)

		// when
		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1,1", "set": map[string]any{"Status": "Done"},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `object "1" is listed more than once`)
		assert.Contains(t, err.Error(), "nothing was written")
	})

	t.Run("more objects than one call carries is refused by count", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1")
		list := ""
		for i := 0; i <= maxSetPropertiesObjects; i++ {
			if i > 0 {
				list += ","
			}
			list += "bafyobj" + string(rune('a'+i))
		}

		// when
		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": list, "set": map[string]any{"Status": "Done"},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at most 16 objects in one call")
	})
}
