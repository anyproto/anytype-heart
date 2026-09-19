package text

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Merging two marks of the same type and param must yield their union. Marks are sorted by
// From before merging, so From is already the smaller one, but To was previously taken from
// the second mark unconditionally, which truncated the first whenever the second ended
// earlier - cutting the tail off a link or a color.
func TestMergeAdjacentMarks_Union(t *testing.T) {
	linkAt := func(from, to int32) *model.BlockContentTextMark {
		return &model.BlockContentTextMark{
			Type:  model.BlockContentTextMark_Link,
			Param: "https://example.com/x",
			Range: &model.Range{From: from, To: to},
		}
	}

	for _, tc := range []struct {
		name  string
		marks []*model.BlockContentTextMark
		want  []*model.BlockContentTextMark
	}{
		{
			name:  "second mark contained in the first does not shorten it",
			marks: []*model.BlockContentTextMark{linkAt(0, 5), linkAt(0, 2)},
			want:  []*model.BlockContentTextMark{linkAt(0, 5)},
		},
		{
			name:  "shorter mark given first does not shorten the longer one",
			marks: []*model.BlockContentTextMark{linkAt(0, 2), linkAt(0, 5)},
			want:  []*model.BlockContentTextMark{linkAt(0, 5)},
		},
		{
			name:  "interior mark does not shorten the surrounding one",
			marks: []*model.BlockContentTextMark{linkAt(0, 5), linkAt(1, 3)},
			want:  []*model.BlockContentTextMark{linkAt(0, 5)},
		},
		{
			name:  "interior mark given first does not shorten the surrounding one",
			marks: []*model.BlockContentTextMark{linkAt(1, 3), linkAt(0, 5)},
			want:  []*model.BlockContentTextMark{linkAt(0, 5)},
		},
		{
			name:  "overlapping mark extends the merged range",
			marks: []*model.BlockContentTextMark{linkAt(0, 5), linkAt(2, 7)},
			want:  []*model.BlockContentTextMark{linkAt(0, 7)},
		},
		{
			name:  "adjacent marks are merged end to end",
			marks: []*model.BlockContentTextMark{linkAt(0, 2), linkAt(2, 5)},
			want:  []*model.BlockContentTextMark{linkAt(0, 5)},
		},
		{
			name:  "disjoint marks are left alone",
			marks: []*model.BlockContentTextMark{linkAt(0, 2), linkAt(3, 5)},
			want:  []*model.BlockContentTextMark{linkAt(0, 2), linkAt(3, 5)},
		},
		{
			name: "marks with a different param are not merged",
			marks: []*model.BlockContentTextMark{
				linkAt(0, 5),
				{Type: model.BlockContentTextMark_Link, Param: "https://other.example/y", Range: &model.Range{From: 0, To: 2}},
			},
			want: []*model.BlockContentTextMark{
				linkAt(0, 5),
				{Type: model.BlockContentTextMark_Link, Param: "https://other.example/y", Range: &model.Range{From: 0, To: 2}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// when
			got := mergeAdjacentMarks(tc.marks)

			// then
			assert.Equal(t, tc.want, got)
		})
	}
}
