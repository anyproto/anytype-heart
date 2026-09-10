package wrapper

// tools_addressing_test.go — the two repairs to the reference channel:
// create hands back a handle instead of dead-ending, and every
// object-addressing tool offers the space slot whose absence sent a space
// id into `object`.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRegistersAHandle(t *testing.T) {
	ctx := context.Background()

	t.Run("the created object is addressable without a find", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/objects", 200, `{"id":"bafynew","type":"page","etag":"e1"}`)
		fx.stub("PATCH /v2/spaces/space1/objects/bafynew", 200, `{"etag":"e2"}`)

		// when
		created, err := fx.Run(ctx, "create", map[string]any{
			"space": "space1", "type": "page", "name": "Report",
		})
		require.NoError(t, err)

		// then: the handle is named, and it resolves
		assert.Contains(t, created.Text, "handle 1", "a number the caller cannot see is a number it cannot pass")
		_, err = fx.Run(ctx, "add_blocks", map[string]any{"object": "1", "markdown": "hi"})
		require.NoError(t, err, "create's handle addresses the object it just made")
		require.Len(t, fx.sent("PATCH /v2/spaces/space1/objects/bafynew"), 1)
	})

	t.Run("a dry run numbers nothing — there is no object to address", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/objects", 200, `{"type":"page","dry_run":true}`)
		fx.DryRun = true

		// when
		result, err := fx.Run(ctx, "create", map[string]any{
			"space": "space1", "type": "page", "name": "Report",
		})

		// then
		require.NoError(t, err)
		assert.NotContains(t, result.Text, "handle")
		session, err := fx.store.Load()
		require.NoError(t, err)
		assert.Empty(t, session.Handles, "there is no object to address")
	})

	t.Run("numbering continues in the working space and restarts across spaces", func(t *testing.T) {
		// given a find already numbered one object in space1
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "found1"})
		fx.stub("POST /v2/spaces/space1/objects", 200, `{"id":"bafya","type":"page"}`)
		fx.stub("POST /v2/spaces/space2/objects", 200, `{"id":"bafyb","type":"page"}`)

		// when
		same, err := fx.Run(ctx, "create", map[string]any{"space": "space1", "type": "page", "name": "A"})
		require.NoError(t, err)
		other, err := fx.Run(ctx, "create", map[string]any{"space": "space2", "type": "page", "name": "B"})
		require.NoError(t, err)

		// then
		assert.Contains(t, same.Text, "handle 2", "the find's handle 1 survives")
		assert.Contains(t, other.Text, "handle 1", "a new space restarts numbering — a handle resolves through Space")
	})
}

func TestSpaceArgAddressesAnObject(t *testing.T) {
	ctx := context.Background()

	t.Run("a full object id resolves with no find at all", func(t *testing.T) {
		// given: no session — the state that produced "no working session"
		fx := newFixture(t)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj", 200, `{"etag":"e1"}`)

		// when
		_, err := fx.Run(ctx, "add_blocks", map[string]any{
			"object": "bafyobj", "space": "space1", "markdown": "hi",
		})

		// then
		require.NoError(t, err)
		require.Len(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj"), 1)
	})

	t.Run("without the space slot the same call still refuses", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		_, err := fx.Run(ctx, "add_blocks", map[string]any{"object": "bafyobj", "markdown": "hi"})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no working session")
	})

	t.Run("the space is not written back — handles keep pointing where find put them", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "obj1"})
		fx.stub("PATCH /v2/spaces/space2/objects/bafyobj", 200, `{"etag":"e1"}`)
		fx.stub("PATCH /v2/spaces/space1/objects/obj1", 200, `{"etag":"e2"}`)

		// when: a one-off edit in another space, then the handle again
		_, err := fx.Run(ctx, "add_blocks", map[string]any{"object": "bafyobj", "space": "space2", "markdown": "x"})
		require.NoError(t, err)
		_, err = fx.Run(ctx, "add_blocks", map[string]any{"object": "1", "markdown": "y"})
		require.NoError(t, err)

		// then
		assert.Len(t, fx.sent("PATCH /v2/spaces/space1/objects/obj1"), 1, "handle 1 still resolves in space1")
	})

	t.Run("a space that disagrees with the handle's space is refused, not guessed", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "obj1"})

		// when
		_, err := fx.Run(ctx, "add_blocks", map[string]any{"object": "1", "space": "space2", "markdown": "x"})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "belongs to the last find")
		assert.Empty(t, fx.sent("PATCH /v2/spaces/space2/objects/obj1"), "nothing is written to the space the caller named")
	})
}

func TestSpaceArgAcceptsTheListingItPrints(t *testing.T) {
	t.Run("a rendered spaces row resolves to its id", func(t *testing.T) {
		// given: exactly what `spaces` served, handed straight back
		want := "hvkwsi"

		// when
		got := spaceArg(map[string]any{"space": "AB experiments — hvkwsi"})

		// then
		assert.Equal(t, want, got, "what a tool prints must be accepted as what it takes")
	})

	t.Run("a bare id is untouched", func(t *testing.T) {
		assert.Equal(t, "hvkwsi", spaceArg(map[string]any{"space": "hvkwsi"}))
	})

	t.Run("a name containing a dash is not mistaken for the separator", func(t *testing.T) {
		// given: the separator is an em-dash with spaces, not any hyphen
		want := "tq5csa"

		// when
		got := spaceArg(map[string]any{"space": "well-known ideas — tq5csa"})

		// then
		assert.Equal(t, want, got)
	})

	t.Run("an absent space stays empty", func(t *testing.T) {
		assert.Equal(t, "", spaceArg(map[string]any{}))
	})
}
