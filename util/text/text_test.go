package text

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTruncateEllipsized(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		length   int
		expected string
	}{
		{
			name:     "3 spaces",
			text:     "   ",
			length:   3,
			expected: "   ",
		},
		{
			name:     "4 spaces",
			text:     "    ",
			length:   3,
			expected: "  …",
		},
		{
			name:     "Space",
			text:     " ",
			length:   3,
			expected: " ",
		},
		{
			name:     "Divine emoji not fit",
			text:     "🌍",
			length:   1,
			expected: " …",
		},
		{
			name:     "Big emoji fit",
			text:     "👨‍👩‍👧‍👦",
			length:   11,
			expected: "👨‍👩‍👧‍👦",
		},
		{
			name:     "Big emoji fit",
			text:     "👨‍👩‍👧‍👦",
			length:   10,
			expected: " …",
		},
		{
			name:     "Divine emoji fit",
			text:     "🌍",
			length:   4,
			expected: "🌍",
		},
		{
			name:     "Text with divine emoji not fit",
			text:     "Hello 🌍",
			length:   7,
			expected: "Hello …",
		},
		{
			name:     "Text with divine emojies fit",
			text:     "Hello 🌍",
			length:   10,
			expected: "Hello 🌍",
		},
		{
			name:     "Text shorter than length",
			text:     "Hello, world!",
			length:   20,
			expected: "Hello, world!",
		},
		{
			name:     "Text shorter than length",
			text:     "Hello, world!",
			length:   12,
			expected: "Hello, …",
		},
		{
			name:     "Text equal to length",
			text:     "Hello, world!",
			length:   13,
			expected: "Hello, world!",
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
			name:     "Text longer than length with mixed characters space",
			text:     "Hello, こんにちは 世界！",
			length:   15,
			expected: "Hello, こんにちは …",
		},
		{
			name:     "Text longer than length with mixed characters no space",
			text:     "Hello, こんにちは、世界！",
			length:   15,
			expected: "Hello, …",
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

func TestTruncateEllipsized2(t *testing.T) {
	require.Equal(t, 2, len([]rune("ма")))
	for i, r := range []rune("мこ🌍世а") {
		println(i)
		println(r)
	}
}
