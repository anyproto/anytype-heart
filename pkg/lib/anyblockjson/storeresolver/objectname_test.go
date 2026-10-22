package storeresolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// objectNameFixture builds a resolver over a space holding one named page
// and one object that never got a name.
func objectNameFixture(t *testing.T) *Resolvers {
	index := spaceindex.NewStoreFixture(t)
	index.AddObjects(t, []spaceindex.TestObject{
		{
			bundle.RelationKeyId:             domain.String("bafyreinamedpage"),
			bundle.RelationKeyName:           domain.String("Local-first UX"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		},
		{
			bundle.RelationKeyId:             domain.String("bafyreinameless"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		},
	})
	return New(index)
}

// ObjectName distinguishes named objects, unnamed objects, and missing rows.
func TestObjectName(t *testing.T) {
	t.Run("a named object resolves to its display name", func(t *testing.T) {
		// given
		r := objectNameFixture(t)

		// when
		name, ok := r.ObjectName("bafyreinamedpage")

		// then
		require.True(t, ok)
		assert.Equal(t, "Local-first UX", name)
	})

	t.Run("an object with no name answers no", func(t *testing.T) {
		// given
		r := objectNameFixture(t)

		// when
		name, ok := r.ObjectName("bafyreinameless")

		// then
		assert.False(t, ok, "an unnamed object has no display name")
		assert.Empty(t, name)
	})

	t.Run("an id this space has no row for answers no", func(t *testing.T) {
		// given
		r := objectNameFixture(t)

		// when
		name, ok := r.ObjectName("bafyreiunknown")

		// then
		assert.False(t, ok)
		assert.Empty(t, name)
	})
}

// The object resolver exposes existence/deletion checks, while SpaceId enables
// participant id folding. Both must reach the codec through Options.
func TestOptions_WiresObjectNamesAndSpaceId(t *testing.T) {
	// given
	r := objectNameFixture(t)

	// when
	opts := r.Options()

	// then
	require.NotNil(t, opts.ResolveObjectNames, "the object resolver is pre-wired")
	name, ok := opts.ResolveObjectNames.ObjectName("bafyreinamedpage")
	require.True(t, ok)
	assert.Equal(t, "Local-first UX", name)
	assert.Equal(t, "test", opts.SpaceId,
		"the index's own space id rides along — the participant fold needs it (§9)")
}
