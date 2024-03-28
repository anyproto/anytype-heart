package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObjectPath_String(t *testing.T) {
	tests := []struct {
		name     string
		path     ObjectPath
		expected string
	}{
		{
			name:     "Only ObjectId",
			path:     ObjectPath{ObjectId: "objectId"},
			expected: "objectId",
		},
		{
			name:     "ObjectId with BlockId",
			path:     NewObjectPathWithBlock("objectId", "blockId"),
			expected: "objectId/b/blockId",
		},
		{
			name:     "ObjectId with RelationKey",
			path:     NewObjectPathWithRelation("objectId", "relationKey"),
			expected: "objectId/r/relationKey",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.path.String())
		})
	}
}

func TestNewFromPath(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		expected    ObjectPath
		expectError bool
	}{
		{
			name:     "Valid path with BlockId",
			path:     "objectId/b/blockId",
			expected: NewObjectPathWithBlock("objectId", "blockId"),
		},
		{
			name:     "Valid path with RelationKey",
			path:     "objectId/r/relationKey",
			expected: NewObjectPathWithRelation("objectId", "relationKey"),
		},
		{
			name:        "Invalid path format",
			path:        "invalidFormatPath",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewFromPath(tt.path)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestObjectPath_HasBlock_HasRelation_IsEmpty(t *testing.T) {
	var path ObjectPath

	// Test IsEmpty
	assert.True(t, path.IsEmpty())

	path = NewObjectPathWithBlock("objectId", "blockId")
	assert.False(t, path.IsEmpty())
	assert.True(t, path.HasBlock())
	assert.False(t, path.HasRelation())

	path = NewObjectPathWithRelation("objectId", "relationKey")
	assert.False(t, path.IsEmpty())
	assert.False(t, path.HasBlock())
	assert.True(t, path.HasRelation())
}

func TestObjectPath_ObjectRelativePath(t *testing.T) {
	tests := []struct {
		name     string
		path     ObjectPath
		expected string
	}{
		{
			name:     "Only ObjectId",
			path:     ObjectPath{ObjectId: "objectId"},
			expected: "",
		},
		{
			name:     "ObjectId with BlockId",
			path:     NewObjectPathWithBlock("objectId", "blockId"),
			expected: "b/blockId",
		},
		{
			name:     "ObjectId with RelationKey",
			path:     NewObjectPathWithRelation("objectId", "relationKey"),
			expected: "r/relationKey",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.path.ObjectRelativePath())
		})
	}
}
