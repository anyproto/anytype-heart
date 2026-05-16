package sourceimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildBoundedEachChange_ResolvesOnlyGivenIdsSkipsUnresolved(t *testing.T) {
	pairs := map[string]readPair{
		"a": {"o1", 1},
		"b": {"o2", 2},
	}
	resolve := func(id string) (readPair, bool) { p, ok := pairs[id]; return p, ok }

	got := map[string]readPair{}
	each := buildBoundedEachChange([]string{"a", "missing", "b"}, resolve)
	each(func(id string, p readPair) { got[id] = p })

	assert.Equal(t, map[string]readPair{"a": {"o1", 1}, "b": {"o2", 2}}, got,
		"yields resolved candidates only; unresolved id skipped")
}
