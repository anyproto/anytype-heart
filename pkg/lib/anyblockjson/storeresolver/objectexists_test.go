package storeresolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

// ObjectExists is the existence half of the object-namespace seam (§9): it
// answers from the same cached point lookup ObjectName pays for, and it is
// the answer ObjectName structurally CANNOT give — that seam's ok is
// `name != ""`, so a present-and-unnamed object answers "no" there. The
// missing-reference rule rewrites and drops on this answer, so the two
// negative shapes below are the ones that protect live data.
//
// How these can fail: read existence off ObjectName's ok and the unnamed
// case fails — the exact conflation that would rewrite live references to
// `_missing_object`; key the lookup on anything but the id, or read a
// partial row as absence, and the named case fails.
func TestObjectExists(t *testing.T) {
	t.Run("a present, named object exists", func(t *testing.T) {
		// given
		r := objectNameFixture(t)

		// when
		exists, known := r.ObjectExists("bafyreinamedpage")

		// then
		require.True(t, known)
		assert.True(t, exists)
	})

	t.Run("a present object with NO name still exists", func(t *testing.T) {
		// given — the trap: ObjectName answers ("", false) for this id
		r := objectNameFixture(t)

		// when
		exists, known := r.ObjectExists("bafyreinameless")
		_, named := r.ObjectName("bafyreinameless")

		// then
		require.True(t, known)
		assert.True(t, exists, "untitled is not missing")
		assert.False(t, named, "and namedness stays a separate question")
	})

	t.Run("an id the index has no row for does not exist", func(t *testing.T) {
		// given
		r := objectNameFixture(t)

		// when
		exists, known := r.ObjectExists("bafyreineverseen")

		// then
		require.True(t, known)
		assert.False(t, exists)
	})

	t.Run("the capability is discoverable off Options", func(t *testing.T) {
		// given — the codec finds it by type assertion on
		// Options.ResolveObjectNames (the TypeResolver pattern), so the
		// standard wiring arms the missing-reference rule with no extra step
		r := objectNameFixture(t)

		// when
		opts := r.Options()
		_, ok := opts.ResolveObjectNames.(anyblockjson.ObjectExistenceResolver)

		// then
		assert.True(t, ok)
	})
}
