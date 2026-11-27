package v2model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPointerToken(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"plain", "plain"},
		{"a/b", "a~1b"},
		{"a~b", "a~0b"},
		{"a~1b/c", "a~01b~1c"}, // "~" escapes first, so an escape is never re-read
	} {
		assert.Equal(t, tc.want, PointerToken(tc.name), tc.name)
	}
	assert.Equal(t, "/a~1b~0c", MemberPointer("a/b~c"))
}
