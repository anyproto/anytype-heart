package objectstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestDsObjectStore_ResolveSpaceID(t *testing.T) {
	s := newStoreFixture(t)

	s.addObjects(t, []testObject{
		{
			bundle.RelationKeyId:      pbtypes.String("object1"),
			bundle.RelationKeyName:    pbtypes.String("name1"),
			bundle.RelationKeySpaceId: pbtypes.String("space1"),
		},
		{
			bundle.RelationKeyId:      pbtypes.String("object2"),
			bundle.RelationKeyName:    pbtypes.String("name2"),
			bundle.RelationKeySpaceId: pbtypes.String("space2"),
		},
	})

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
