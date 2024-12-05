package date

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/dateutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func Test_suggestDateForSearch(t *testing.T) {
	loc := time.FixedZone("Berlin", +2)
	now := time.Date(2022, 5, 18, 14, 56, 33, 0, loc)

	tests := []struct {
		raw  string
		want time.Time
	}{
		{
			raw:  "now",
			want: time.Date(2022, 5, 18, 14, 56, 33, 0, loc),
		},
		{
			raw:  "date",
			want: time.Date(2022, 5, 18, 0, 0, 0, 0, loc),
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

func TestSuggestDateObjectIdsFromFilter(t *testing.T) {
	// given
	dateObject1 := dateutil.NewDateObject(time.Now(), false)
	dateObject2 := dateutil.NewDateObject(time.Now().Add(2*time.Hour), true)
	filters := []*model.BlockContentDataviewFilter{{
		RelationKey: bundle.RelationKeyId.String(),
		Condition:   model.BlockContentDataviewFilter_In,
		Value:       pbtypes.StringList([]string{dateObject1.Id(), "plainObj1", "planObj2", dateObject2.Id()}),
	}}

	// when
	ids := suggestDateObjectIds("", filters)

	// then
	require.Len(t, ids, 2)
	assert.Equal(t, dateObject1.Id(), ids[0])
	assert.Equal(t, dateObject2.Id(), ids[1])
}
