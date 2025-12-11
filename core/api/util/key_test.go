package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToPropertyApiKey(t *testing.T) {
	tests := []struct {
		name        string
		internalKey string
		expected    string
	}{
		{
			name:        "camelCase to snake_case",
			internalKey: "dueDate",
			expected:    "due_date",
		},
		{
			name:        "single word lowercase",
			internalKey: "name",
			expected:    "name",
		},
		{
			name:        "single word uppercase",
			internalKey: "Name",
			expected:    "name",
		},
		{
			name:        "multiple words camelCase",
			internalKey: "lastModifiedDate",
			expected:    "last_modified_date",
		},
		{
			name:        "BSON ID remains unchanged",
			internalKey: "67b0d3e3cda913b84c1299b1",
			expected:    "67b0d3e3cda913b84c1299b1",
		},
		{
			name:        "already snake_case",
			internalKey: "already_snake_case",
			expected:    "already_snake_case",
		},
		{
			name:        "empty string",
			internalKey: "",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToPropertyApiKey(tt.internalKey)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToTypeApiKey(t *testing.T) {
	tests := []struct {
		name        string
		internalKey string
		expected    string
	}{
		{
			name:        "standard type with prefix",
			internalKey: "ot-page",
			expected:    "page",
		},
		{
			name:        "camelCase type with prefix",
			internalKey: "ot-taskList",
			expected:    "task_list",
		},
		{
			name:        "type without prefix",
			internalKey: "page",
			expected:    "page",
		},
		{
			name:        "BSON ID with prefix",
			internalKey: "ot-67b0d3e3cda913b84c1299b1",
			expected:    "67b0d3e3cda913b84c1299b1",
		},
		{
			name:        "BSON ID without prefix",
			internalKey: "67b0d3e3cda913b84c1299b1",
			expected:    "67b0d3e3cda913b84c1299b1",
		},
		{
			name:        "empty string",
			internalKey: "",
			expected:    "",
		},
		{
			name:        "only prefix",
			internalKey: "ot-",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToTypeApiKey(tt.internalKey)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToTagApiKey(t *testing.T) {
	tests := []struct {
		name        string
		internalKey string
		expected    string
	}{
		{
			name:        "standard tag with prefix",
			internalKey: "opt-color",
			expected:    "color",
		},
		{
			name:        "camelCase tag with prefix",
			internalKey: "opt-backgroundColor",
			expected:    "background_color",
		},
		{
			name:        "tag without prefix",
			internalKey: "color",
			expected:    "color",
		},
		{
			name:        "BSON ID with prefix",
			internalKey: "opt-67b0d3e3cda913b84c1299b1",
			expected:    "67b0d3e3cda913b84c1299b1",
		},
		{
			name:        "BSON ID without prefix",
			internalKey: "67b0d3e3cda913b84c1299b1",
			expected:    "67b0d3e3cda913b84c1299b1",
		},
		{
			name:        "empty string",
			internalKey: "",
			expected:    "",
		},
		{
			name:        "only prefix",
			internalKey: "opt-",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToTagApiKey(tt.internalKey)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsBsonId(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected bool
	}{
		{
			name:     "valid BSON ID",
			key:      "67b0d3e3cda913b84c1299b1",
			expected: true,
		},
		{
			name:     "valid BSON ID with all digits",
			key:      "123456789012345678901234",
			expected: true,
		},
		{
			name:     "invalid - too short",
			key:      "67b0d3e3cda913b84c1299b",
			expected: false,
		},
		{
			name:     "invalid - too long",
			key:      "67b0d3e3cda913b84c1299b12",
			expected: false,
		},
		{
			name:     "invalid - contains uppercase",
			key:      "67B0D3E3CDA913B84C1299B1",
			expected: false,
		},
		{
			name:     "invalid - contains non-hex characters",
			key:      "67b0d3e3-da913b84c1299b1",
			expected: false,
		},
		{
			name:     "invalid - all letters (no digits)",
			key:      "abcdefabcdefabcdefabcdef",
			expected: false,
		},
		{
			name:     "empty string",
			key:      "",
			expected: false,
		},
		{
			name:     "regular word",
			key:      "page",
			expected: false,
		},
		{
			name:     "24 non-hex characters",
			key:      "this-is-exactly-24-chars",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsBsonId(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEdgeCases(t *testing.T) {
	t.Run("ToPropertyApiKey with special characters", func(t *testing.T) {
		// Test that special characters in property names are handled
		assert.Equal(t, "user_id", ToPropertyApiKey("userID"))
		assert.Equal(t, "html_content", ToPropertyApiKey("HTMLContent"))
		assert.Equal(t, "api_key", ToPropertyApiKey("APIKey"))
	})

	t.Run("ToTypeApiKey with double prefix", func(t *testing.T) {
		// Test that double prefix doesn't cause issues
		assert.Equal(t, "ot_page", ToTypeApiKey("ot-ot-page"))
	})

	t.Run("ToTagApiKey with mixed case prefix", func(t *testing.T) {
		// Prefix should be case-sensitive
		assert.Equal(t, "opt_color", ToTagApiKey("OPT-color"))
		assert.Equal(t, "opt_color", ToTagApiKey("Opt-color"))
	})
}

func BenchmarkToPropertyApiKey(b *testing.B) {
	b.Run("CamelCase", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ToPropertyApiKey("lastModifiedDate")
		}
	})

	b.Run("BsonId", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ToPropertyApiKey("67b0d3e3cda913b84c1299b1")
		}
	})
}

func BenchmarkIsBsonId(b *testing.B) {
	b.Run("Valid", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			IsBsonId("67b0d3e3cda913b84c1299b1")
		}
	})

	b.Run("Invalid", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			IsBsonId("not-a-bson-id")
		}
	})
}
