package date

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_suggestDateForSearch(t *testing.T) {
	loc := time.FixedZone("Berlin", +2)
	now := time.Date(2022, 5, 18, 14, 56, 33, 0, loc)

	tests := []struct {
		now  time.Time
		raw  string
		want time.Time
	}{
		{
			raw:  "now",
			want: time.Date(2022, 5, 18, 14, 56, 33, 0, loc),
		},
		{
			raw:  "today",
			want: time.Date(2022, 5, 18, 0, 0, 0, 0, loc),
		},
		{
			raw:  "10 days from now",
			want: time.Date(2022, 5, 28, 14, 56, 33, 0, loc),
		},
		{
			raw:  "05.02.2021",
			want: time.Date(2021, 2, 5, 0, 0, 0, 0, loc),
		},
		{
			raw:  "1",
			want: time.Time{},
		},
		{
			raw:  "12345",
			want: time.Time{},
		},
		{
			raw:  "1994",
			want: time.Time{},
		},
		{
			raw:  "foobar",
			want: time.Time{},
		},
		{
			raw:  "",
			want: time.Time{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got := suggestDateForSearch(now, tt.raw)

			assert.Equal(t, tt.want, got)
		})
	}
}
