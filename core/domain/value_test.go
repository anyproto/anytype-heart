package domain

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareMaps(t *testing.T) {
	for i, tc := range []struct {
		a, b map[string]Value
		want int
	}{
		{
			a:    map[string]Value{"a": Int64(1), "b": String("b")},
			b:    map[string]Value{"a": Int64(1), "b": String("b")},
			want: 0,
		},
		{
			a:    map[string]Value{"a": Int64(1)},
			b:    map[string]Value{"b": String("b")},
			want: -1,
		},
		{
			a:    map[string]Value{"b": String("b")},
			b:    map[string]Value{"a": Int64(1)},
			want: 1,
		},
		{
			a:    map[string]Value{"a": Int64(1), "b": String("b")},
			b:    map[string]Value{"a": Int64(2), "b": String("b")},
			want: -1,
		},
		{
			a:    map[string]Value{"a": Int64(1), "b": String("c")},
			b:    map[string]Value{"a": Int64(1), "b": String("b")},
			want: 1,
		},
		{
			a:    map[string]Value{"a": NewValueMap(map[string]Value{"a": Int64(1)})},
			b:    map[string]Value{"a": NewValueMap(map[string]Value{"a": Int64(1)})},
			want: 0,
		},
		{
			a:    map[string]Value{"a": NewValueMap(map[string]Value{"a": Int64(1)})},
			b:    map[string]Value{"a": NewValueMap(map[string]Value{"a": Int64(2)})},
			want: -1,
		},
		{
			a:    map[string]Value{"a": NewValueMap(map[string]Value{"a": Int64(2)})},
			b:    map[string]Value{"a": NewValueMap(map[string]Value{"a": Int64(1)})},
			want: 1,
		},
	} {
		t.Run(fmt.Sprintf("case %d", i+1), func(t *testing.T) {
			got := compareMaps(&GenericMap[string]{data: (tc.a)}, &GenericMap[string]{data: (tc.b)})
			assert.Equal(t, tc.want, got)
		})
	}
}
