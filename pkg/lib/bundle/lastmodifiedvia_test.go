package bundle

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

// lastModifiedVia sits next to lastModifiedBy and must be provisioned exactly
// as it is: a client (API v1 above all) shows a detail only when the space
// has the relation, and only the required/system lists put it in every space,
// existing ones included.
func TestLastModifiedViaIsPlacedLikeLastModifiedBy(t *testing.T) {
	for name, list := range map[string][]domain.RelationKey{
		"RequiredInternalRelations": RequiredInternalRelations,
		"SystemRelations":           SystemRelations,
		"DerivedRelationsKeys":      DerivedRelationsKeys,
	} {
		// red if the key is dropped from a list lastModifiedBy is on
		assert.Equal(t,
			slices.Contains(list, RelationKeyLastModifiedBy),
			slices.Contains(list, RelationKeyLastModifiedVia), name)
		assert.True(t, slices.Contains(list, RelationKeyLastModifiedVia), name)
	}
	by, err := GetRelation(RelationKeyLastModifiedBy)
	require.NoError(t, err)
	via, err := GetRelation(RelationKeyLastModifiedVia)
	require.NoError(t, err)
	// red if it ever becomes writable, hidden, or a stored (non-derived) detail
	assert.Equal(t, by.ReadOnly, via.ReadOnly)
	assert.Equal(t, by.Hidden, via.Hidden)
	assert.Equal(t, by.DataSource, via.DataSource)
	assert.True(t, via.ReadOnly)
}
