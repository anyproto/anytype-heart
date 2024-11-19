package dateutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTimeToDateId(t *testing.T) {
	assert.Equal(t, "_date_2024-11-07-12-25-59Z_0000", TimeToDateId(time.Date(2024, time.November, 7, 12, 25, 59, 0, time.UTC), true))
	assert.Equal(t, "_date_1998-01-01-00-01-01Z_0000", TimeToDateId(time.Date(1998, time.January, 1, 0, 1, 1, 0, time.UTC), true))
	assert.Equal(t, "_date_1998-01-01-00-01-01Z_0400", TimeToDateId(time.Date(1998, time.January, 1, 0, 1, 1, 0, time.FixedZone("UTC", +4*60*60)), true))
	assert.Equal(t, "_date_2124-12-25-23-34-00Z_0000", TimeToDateId(time.Date(2124, time.December, 25, 23, 34, 0, 0, time.UTC), true))
	assert.Equal(t, "_date_2124-12-25-23-34-00Z-0200", TimeToDateId(time.Date(2124, time.December, 25, 23, 34, 0, 0, time.FixedZone("UTC", -2*60*60)), true))
}

func TestTimeToShortDateId(t *testing.T) {
	assert.Equal(t, "_date_2024-11-07", TimeToDateId(time.Date(2024, time.November, 7, 12, 25, 59, 0, time.UTC), false))
	assert.Equal(t, "_date_1998-01-01", TimeToDateId(time.Date(1998, time.January, 1, 0, 1, 1, 0, time.UTC), false))
	assert.Equal(t, "_date_1998-01-01", TimeToDateId(time.Date(1998, time.January, 1, 0, 1, 1, 0, time.FixedZone("UTC", +4*60*60)), false))
	assert.Equal(t, "_date_2124-12-25", TimeToDateId(time.Date(2124, time.December, 25, 23, 34, 0, 0, time.UTC), false))
	assert.Equal(t, "_date_2124-12-25", TimeToDateId(time.Date(2124, time.December, 25, 23, 34, 0, 0, time.FixedZone("UTC", -2*60*60)), false))
}

func TestTimeToDateName(t *testing.T) {
	assert.Equal(t, "07 Nov 2024", TimeToDateName(time.Date(2024, time.November, 7, 12, 25, 59, 0, time.UTC)))
	assert.Equal(t, "01 Jan 1998", TimeToDateName(time.Date(1998, time.January, 1, 0, 1, 1, 0, time.UTC)))
	assert.Equal(t, "01 Jan 1998", TimeToDateName(time.Date(1998, time.January, 1, 0, 1, 1, 0, time.FixedZone("UTC", +4*60*60))))
	assert.Equal(t, "25 Dec 2124", TimeToDateName(time.Date(2124, time.December, 25, 23, 34, 0, 0, time.UTC)))
	assert.Equal(t, "25 Dec 2124", TimeToDateName(time.Date(2124, time.December, 25, 23, 34, 0, 0, time.FixedZone("UTC", -2*60*60))))
	assert.Equal(t, "25 Dec 2124", TimeToDateName(time.Date(2124, time.December, 25, 23, 34, 0, 0, time.FixedZone("UTC", -2*60*60))))
}

func TestParseDateId(t *testing.T) {
	t.Run("long date format", func(t *testing.T) {
		for _, ts := range []time.Time{
			time.Date(2024, time.December, 7, 12, 25, 59, 0, time.UTC),
			time.Date(2024, time.November, 7, 12, 25, 59, 0, time.UTC),
			time.Date(1998, time.January, 1, 0, 1, 1, 0, time.UTC),
			time.Date(1998, time.January, 1, 0, 1, 1, 0, time.FixedZone("UTC", +4*60*60)),
			time.Date(2124, time.December, 25, 23, 34, 0, 0, time.UTC),
			time.Date(2124, time.December, 25, 23, 34, 0, 0, time.FixedZone("UTC", -2*60*60)),
		} {
			dateId := TimeToDateId(ts, true)
			ts2, err := ParseDateId(dateId)
			assert.NoError(t, err)
			assert.Equal(t, ts.Unix(), ts2.Unix())
		}
	})

	t.Run("short date format", func(t *testing.T) {
		for _, ts := range []time.Time{
			time.Date(2024, time.December, 7, 12, 25, 59, 0, time.UTC),
			time.Date(2024, time.November, 7, 12, 25, 59, 0, time.UTC),
			time.Date(1998, time.January, 1, 0, 1, 1, 0, time.UTC),
			time.Date(1998, time.January, 1, 0, 1, 1, 0, time.FixedZone("UTC", +4*60*60)),
			time.Date(2124, time.December, 25, 23, 34, 0, 0, time.UTC),
			time.Date(2124, time.December, 25, 23, 34, 0, 0, time.FixedZone("UTC", -2*60*60)),
		} {
			dateId := TimeToDateId(ts, false)
			ts2, err := ParseDateId(dateId)
			assert.NoError(t, err)
			assert.Equal(t, ts.Year(), ts2.Year())
			assert.Equal(t, ts.Month(), ts2.Month())
			assert.Equal(t, ts.Day(), ts2.Day())
			assert.Zero(t, ts2.Hour())
			assert.Zero(t, ts2.Minute())
			assert.Zero(t, ts2.Second())
			assert.Equal(t, time.Local, ts2.Location())
		}
	})

	t.Run("wrong format", func(t *testing.T) {
		_, err := ParseDateId("_date_2024")
		assert.Error(t, err)

		_, err = ParseDateId("object1")
		assert.Error(t, err)
	})
}
