package main

import (
	"sort"
	"testing"

	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/require"
	lexicographic_sort "github.com/tolgaOzen/lexicographic-sort"
)

func Test_moveAfter(t *testing.T) {
	type args struct {
		tasks     []*Task
		fromIndex int
		toIndex   int
	}
	tests := []struct {
		name     string
		args     args
		expected []*Task
	}{
		{
			name: "to middle",
			args: args{
				tasks: []*Task{
					&Task{
						ID:    "1",
						Order: "a",
					},
					&Task{
						ID:    "2",
						Order: "b",
					},
					&Task{
						ID:    "3",
						Order: "c",
					},
				},
				fromIndex: 2,
				toIndex:   1,
			},
			expected: []*Task{
				&Task{
					ID:    "1",
					Order: "a",
				},
				&Task{
					ID:    "3",
					Order: "an",
				},
				&Task{
					ID:    "2",
					Order: "b",
				},
			},
		},
		{
			name: "to beginning",
			args: args{
				tasks: []*Task{
					&Task{
						ID:    "1",
						Order: "a",
					},
					&Task{
						ID:    "2",
						Order: "b",
					},
					&Task{
						ID:    "3",
						Order: "c",
					},
				},
				fromIndex: 2,
				toIndex:   0,
			},
			expected: []*Task{
				&Task{
					ID:    "3",
					Order: "A",
				},
				&Task{
					ID:    "1",
					Order: "a",
				},
				&Task{
					ID:    "2",
					Order: "b",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			moveTask(tt.args.tasks, tt.args.fromIndex, tt.args.toIndex)
			sort.Sort(Tasks(tt.args.tasks))
			require.Equal(t, len(tt.expected), len(tt.args.tasks))
			for i := range tt.expected {
				require.Equal(t, tt.expected[i], tt.args.tasks[i])
			}
		})
	}
}

func Test_moveAfterIter(t *testing.T) {
	list := []*Task{
		&Task{
			ID:    "1",
			Order: "a",
		},
		&Task{
			ID:    "2",
			Order: "b",
		},
	}
	for i := 0; i < 100; i++ {
		moveTask(list, 1, 0)
		sort.Sort(Tasks(list))
	}

	require.Equal(t, "1", list[0].ID)
}

func Test_moveAfterIter2(t *testing.T) {
	a := bson.NewObjectId().Hex()
	b := bson.NewObjectId().Hex()
	for i := 0; i < 1000; i++ {
		x := lexicographic_sort.GenerateBetween(a, b)
		if x <= a || x >= b {
			t.Fatalf("invalid: %s", x)
		}
		b = x
	}
}
