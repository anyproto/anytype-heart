package dateutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewDateObject(t *testing.T) {
	t.Run("include time", func(t *testing.T) {
		for _, tc := range []struct {
			ts           time.Time
			expectedId   string
			expectedName string
		}{
			{
				ts:           time.Date(2024, time.November, 7, 12, 25, 59, 0, time.UTC),
				expectedId:   "_date_2024-11-07-12-25-59Z_0000",
				expectedName: "Thu, Nov 7, 2024 12:25 PM",
			},
			{
				ts:           time.Date(1998, time.January, 1, 0, 1, 1, 0, time.UTC),
				expectedId:   "_date_1998-01-01-00-01-01Z_0000",
				expectedName: "Thu, Jan 1, 1998 12:01 AM",
			},
			{
				ts:           time.Date(1998, time.January, 1, 0, 1, 1, 0, time.FixedZone("UTC", +4*60*60)),
				expectedId:   "_date_1998-01-01-00-01-01Z_0400",
				expectedName: "Thu, Jan 1, 1998 12:01 AM",
			},
			{
				ts:           time.Date(2124, time.December, 25, 23, 34, 0, 0, time.UTC),
				expectedId:   "_date_2124-12-25-23-34-00Z_0000",
				expectedName: "Mon, Dec 25, 2124 11:34 PM",
			},
			{
				ts:           time.Date(2124, time.December, 25, 23, 34, 0, 0, time.FixedZone("UTC", -2*60*60)),
				expectedId:   "_date_2124-12-25-23-34-00Z-0200",
				expectedName: "Mon, Dec 25, 2124 11:34 PM",
			},
		} {
			do := NewDateObject(tc.ts, true)
			assert.Equal(t, tc.expectedId, do.Id())
			assert.Equal(t, tc.expectedName, do.Name())
			assert.Equal(t, tc.ts, do.Time())
		}
	})

	t.Run("do not include time", func(t *testing.T) {
		for _, tc := range []struct {
			ts           time.Time
			expectedId   string
			expectedName string
		}{
			{
				ts:           time.Date(2024, time.November, 7, 12, 25, 59, 0, time.UTC),
				expectedId:   "_date_2024-11-07",
				expectedName: "Thu, Nov 7, 2024",
			},
			{
				ts:           time.Date(1998, time.January, 1, 0, 1, 1, 0, time.UTC),
				expectedId:   "_date_1998-01-01",
				expectedName: "Thu, Jan 1, 1998",
			},
			{
				ts:           time.Date(1998, time.January, 1, 0, 1, 1, 0, time.FixedZone("UTC", +4*60*60)),
				expectedId:   "_date_1998-01-01",
				expectedName: "Thu, Jan 1, 1998",
			},
			{
				ts:           time.Date(2124, time.December, 25, 23, 34, 0, 0, time.UTC),
				expectedId:   "_date_2124-12-25",
				expectedName: "Mon, Dec 25, 2124",
			},
			{
				ts:           time.Date(2124, time.December, 25, 23, 34, 0, 0, time.FixedZone("UTC", -2*60*60)),
				expectedId:   "_date_2124-12-25",
				expectedName: "Mon, Dec 25, 2124",
			},
		} {
			do := NewDateObject(tc.ts, false)
			assert.Equal(t, tc.expectedId, do.Id())
			assert.Equal(t, tc.expectedName, do.Name())
			assert.Equal(t, tc.ts, do.Time())
		}
	})
}

func TestBuildDateObjectFromId(t *testing.T) {
	t.Run("date object ids", func(t *testing.T) {
		for _, ts := range []time.Time{
			time.Date(2024, time.December, 7, 12, 25, 59, 0, time.UTC),
			time.Date(2024, time.November, 7, 12, 25, 59, 0, time.UTC),
			time.Date(1998, time.January, 1, 0, 1, 1, 0, time.UTC),
			time.Date(1998, time.January, 1, 0, 1, 1, 0, time.FixedZone("UTC", +4*60*60)),
			time.Date(2124, time.December, 25, 23, 34, 0, 0, time.UTC),
			time.Date(2124, time.December, 25, 23, 34, 0, 0, time.FixedZone("UTC", -2*60*60)),
		} {
			withTime1 := NewDateObject(ts, true)
			withTime2, err := BuildDateObjectFromId(withTime1.Id())
			assert.NoError(t, err)
			assert.Equal(t, withTime2.Time().Unix(), withTime1.Time().Unix())
			assert.Equal(t, withTime2.Id(), withTime1.Id())

			withoutTime1 := NewDateObject(ts, true)
			withoutTime2, err := BuildDateObjectFromId(withoutTime1.Id())
			assert.NoError(t, err)
			assert.Equal(t, withoutTime2.Time().Unix(), withoutTime1.Time().Unix())
			assert.Equal(t, withoutTime2.Id(), withoutTime1.Id())
		}
	})

	t.Run("wrong format", func(t *testing.T) {
		_, err := BuildDateObjectFromId("_date_2024")
		assert.Error(t, err)

		_, err = BuildDateObjectFromId("object1")
		assert.Error(t, err)
	})

}
