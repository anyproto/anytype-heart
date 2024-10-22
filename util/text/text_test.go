package text

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		length   int
		expected string
	}{
		{
			name:     "Text shorter than length",
			text:     "Hello, world!",
			length:   20,
			expected: "Hello, world!",
		},
		{
			name:     "Text equal to length",
			text:     "Hello, world!",
			length:   13,
			expected: "Hello, …",
		},
		{
			name:     "Text longer than length with space truncation",
			text:     "The quick brown fox jumps over the lazy dog",
			length:   23,
			expected: "The quick brown fox …",
		},
		{
			name:     "Text longer than length with word truncation",
			text:     "The quick brown fox jumps over the lazy dog",
			length:   14,
			expected: "The quick …",
		},
		{
			name:     "Text longer than length with non-ASCII characters",
			text:     "こんにちは、世界！",
			length:   14,
			expected: "こんにちは、世界！",
		},
		{
			name:     "Text longer than length with mixed characters",
			text:     "Hello, こんにちは、世界！",
			length:   17,
			expected: "Hello, こんにちは、世 …",
		},
		{
			name:     "Text with ellipsis already present",
			text:     "The quick brown fox jumps over the lazy dog",
			length:   10,
			expected: "The …",
		},
		{
			name:     "Text longer than length without word to truncate",
			text:     "Thisisaverylongword",
			length:   10,
			expected: "Thisisav …",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := TruncateEllipsized(test.text, test.length)
			assert.Equal(t, test.expected, actual)
		})
	}
}
