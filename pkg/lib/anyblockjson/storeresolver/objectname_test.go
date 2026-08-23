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

// ObjectName is the object-namespace seam the `#name` reference suffix reads
// from (§9). The two negative answers matter as much as the positive one:
// export writes a suffix only when this says yes, so a "yes, empty string"
// would put a dangling `#` on every reference to an unnamed object.
//
// How these can fail: a lookup keyed on anything but the object id misses
// the named row and the first case fails; reading any field but `name` fails
// it too; returning true for an empty name fails the second and third.
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
		assert.False(t, ok, "no name is an answer of no, never a blank suffix")
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

// Options() is the one line every wiring copies, so what it pre-wires is
// what every export/import actually runs with. The object-name seam and the
// space id ride it like the four resolvers before them: forget either and
// the suffix never fires (silently) or the participant fold never fires
// (silently), with nothing else failing.
//
// How this can fail: drop `ResolveObjectNames: r` or `SpaceId:
// r.index.SpaceId()` from Options() and the matching assertion fails.
func TestOptions_WiresObjectNamesAndSpaceId(t *testing.T) {
	// given
	r := objectNameFixture(t)

	// when
	opts := r.Options()

	// then
	require.NotNil(t, opts.ResolveObjectNames, "the suffix seam is pre-wired")
	name, ok := opts.ResolveObjectNames.ObjectName("bafyreinamedpage")
	require.True(t, ok)
	assert.Equal(t, "Local-first UX", name)
	assert.Equal(t, "test", opts.SpaceId,
		"the index's own space id rides along — the participant fold needs it (§9)")
}
