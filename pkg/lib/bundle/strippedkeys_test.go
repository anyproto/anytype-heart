package bundle

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultStrippedKeys(t *testing.T) {
	assert.Len(t, DefaultStrippedKeys, 5)

	assert.True(t, IsDefaultStrippedKey(RelationKeySyncStatus))
	assert.True(t, IsDefaultStrippedKey(RelationKeySyncError))
	assert.True(t, IsDefaultStrippedKey(RelationKeySyncDate))
	assert.True(t, IsDefaultStrippedKey(RelationKeyLastUsedDate))
	assert.True(t, IsDefaultStrippedKey(RelationKeyLastOpenedDate))

	assert.False(t, IsDefaultStrippedKey(RelationKeyName))
	assert.False(t, IsDefaultStrippedKey(RelationKeyId))
}
