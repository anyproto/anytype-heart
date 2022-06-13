package table

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source *state.State
		want   *state.State
	}{
		{
			name:   "empty",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{}),
			want:   mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{}),
		},
		{
			name: "invalid ids",
			source: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{"row1-c11", "row1-col2"},
				{"row2-col3"},
			}),
			want: mkTestTable([]string{"col1", "col2"}, []string{"row1", "row2"}, [][]string{
				{"row1-col2"},
				{},
			}),
		},
		{
			name: "wrong order",
			source: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, [][]string{
				{"row1-col3", "row1-col1", "row1-col2"},
				{"row2-col3", "row2-c1", "row2-col1"},
			}),
			want: mkTestTable([]string{"col1", "col2", "col3"}, []string{"row1", "row2"}, [][]string{
				{"row1-col1", "row1-col2", "row1-col3"},
				{"row2-col1", "row2-col3"},
			}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb, err := newTableBlockFromState(tc.source, "table")

			require.NoError(t, err)

			st := tc.source.Copy()
			err = tb.block.(Block).Normalize(st)
			require.NoError(t, err)

			assert.Equal(t, tc.want.Blocks(), st.Blocks())
		})
	}
}
