package pagination

import (
	"reflect"
	"testing"
)

func TestPaginate(t *testing.T) {
	type args struct {
		records []int
		offset  int
		limit   int
	}
	tests := []struct {
		name          string
		args          args
		wantPaginated []int
		wantHasMore   bool
	}{
		{
			name: "Offset=0, Limit=2 (first two items)",
			args: args{
				records: []int{1, 2, 3, 4, 5},
				offset:  0,
				limit:   2,
			},
			wantPaginated: []int{1, 2},
			wantHasMore:   true, // items remain: [3,4,5]
		},
		{
			name: "Offset=2, Limit=2 (middle slice)",
			args: args{
				records: []int{1, 2, 3, 4, 5},
				offset:  2,
				limit:   2,
			},
			wantPaginated: []int{3, 4},
			wantHasMore:   true, // item 5 remains
		},
		{
			name: "Offset=4, Limit=2 (tail of the slice)",
			args: args{
				records: []int{1, 2, 3, 4, 5},
				offset:  4,
				limit:   2,
			},
			wantPaginated: []int{5},
			wantHasMore:   false,
		},
		{
			name: "Offset > length (should return empty)",
			args: args{
				records: []int{1, 2, 3, 4, 5},
				offset:  10,
				limit:   2,
			},
			wantPaginated: []int{},
			wantHasMore:   false,
		},
		{
			name: "Limit > length (should return entire slice)",
			args: args{
				records: []int{1, 2, 3},
				offset:  0,
				limit:   10,
			},
			wantPaginated: []int{1, 2, 3},
			wantHasMore:   false,
		},
		{
			name: "Zero limit (no items returned)",
			args: args{
				records: []int{1, 2, 3, 4, 5},
				offset:  1,
				limit:   0,
			},
			wantPaginated: []int{},
			wantHasMore:   true, // items remain: [2,3,4,5]
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPaginated, gotHasMore := Paginate(tt.args.records, tt.args.offset, tt.args.limit)

			if !reflect.DeepEqual(gotPaginated, tt.wantPaginated) {
				t.Errorf("Paginate() gotPaginated = %v, want %v", gotPaginated, tt.wantPaginated)
			}
			if gotHasMore != tt.wantHasMore {
				t.Errorf("Paginate() gotHasMore = %v, want %v", gotHasMore, tt.wantHasMore)
			}
		})
	}
}
