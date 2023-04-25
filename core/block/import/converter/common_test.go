package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReplaceChunks(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		oldToNew map[string]string
		want     []string
	}{
		{
			name: "Test 1",
			s:    "thequickbrownfoxjumpsoverthelazydog",
			oldToNew: map[string]string{
				"brown": "blue",
				"lazy":  "energetic",
				"jumps": "flies",
			},
			want: []string{"thequick", "blue", "fox", "flies", "overthe", "energetic"},
		},
		{
			name: "Test 2",
			s:    "loremipsumdolorsitamet",
			oldToNew: map[string]string{
				"ipsum": "filler",
				"dolor": "pain",
				"amet":  "meet",
			},
			want: []string{"lorem", "filler", "pain", "sit", "meet"},
		},
		{
			name: "Test 3",
			s:    "abcde",
			oldToNew: map[string]string{
				"a": "1",
				"b": "2",
				"c": "3",
				"d": "4",
				"e": "5",
			},
			want: []string{"1", "2", "3", "4", "5"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := replaceChunks(tc.s, tc.oldToNew)

			if !assert.Equal(t, tc.want, got) {
				t.Errorf("replaceChunks(%q, %v) = %q; want %q", tc.s, tc.oldToNew, got, tc.want)
			}
		})
	}
}
