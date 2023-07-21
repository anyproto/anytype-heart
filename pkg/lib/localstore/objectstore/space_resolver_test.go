package objectstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDsObjectStore_ResolveSpaceID(t *testing.T) {
	s := newStoreFixture(t)

	err := s.StoreSpaceID("object1", "space1")
	require.NoError(t, err)

	err = s.StoreSpaceID("object2", "space2")
	require.NoError(t, err)

	got, err := s.ResolveSpaceID("object1")
	require.NoError(t, err)
	assert.Equal(t, "space1", got)

	got, err = s.ResolveSpaceID("object2")
	require.NoError(t, err)
	assert.Equal(t, "space2", got)

	_, err = s.ResolveSpaceID("object3")
	require.Error(t, err)
	assert.True(t, isNotFound(err))
}
