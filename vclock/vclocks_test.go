package vclock

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSort(t *testing.T) {
	clocks := VClocks{
		// on A perspective
		NewFromMap(map[string]uint64{"A": 1, "B": 2, "C": 1}),
		NewFromMap(map[string]uint64{"A": 2, "B": 2, "C": 1}),
		NewFromMap(map[string]uint64{"A": 3, "B": 3, "C": 3}),
		NewFromMap(map[string]uint64{"A": 4, "B": 5, "C": 5}),

		// on B perspective
		NewFromMap(map[string]uint64{"B": 1, "C": 1}),
		NewFromMap(map[string]uint64{"B": 2, "C": 1}),
		NewFromMap(map[string]uint64{"B": 3, "C": 1}),
		NewFromMap(map[string]uint64{"A": 2, "B": 4, "C": 1}),
		NewFromMap(map[string]uint64{"A": 2, "B": 5, "C": 1}),

		// on C perspective
		NewFromMap(map[string]uint64{"C": 1}),
		NewFromMap(map[string]uint64{"B": 3, "C": 2}),
		NewFromMap(map[string]uint64{"B": 3, "C": 3}),
		NewFromMap(map[string]uint64{"A": 2, "B": 5, "C": 4}),
		NewFromMap(map[string]uint64{"A": 2, "B": 5, "C": 5}),
	}

	sort.Sort(clocks)

	var result string
	for _, clock := range clocks {
		result += fmt.Sprintln(clock)
	}

	require.Equal(t, `{"C":1}
{"B":1, "C":1}
{"B":2, "C":1}
{"B":3, "C":1}
{"A":1, "B":2, "C":1}
{"A":2, "B":2, "C":1}
{"A":2, "B":4, "C":1}
{"B":3, "C":3}
{"B":3, "C":2}
{"A":3, "B":3, "C":3}
{"A":2, "B":5, "C":1}
{"A":2, "B":5, "C":4}
{"A":2, "B":5, "C":5}
{"A":4, "B":5, "C":5}
`, result)
}
