package dateutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTimeToDateId(t *testing.T) {
	assert.Equal(t, "_date_2024-11-07", TimeToDateId(time.Date(2024, time.November, 7, 12, 25, 59, 0, time.UTC)))
	assert.Equal(t, "_date_1998-01-01", TimeToDateId(time.Date(1998, time.January, 1, 0, 1, 1, 0, time.UTC)))
	assert.Equal(t, "_date_2124-12-25", TimeToDateId(time.Date(2124, time.December, 25, 23, 34, 0, 0, time.UTC)))
}

func TestTimeToDateName(t *testing.T) {
	assert.Equal(t, "07 Nov 2024", TimeToDateName(time.Date(2024, time.November, 7, 12, 25, 59, 0, time.UTC)))
	assert.Equal(t, "01 Jan 1998", TimeToDateName(time.Date(1998, time.January, 1, 0, 1, 1, 0, time.UTC)))
	assert.Equal(t, "25 Dec 2124", TimeToDateName(time.Date(2124, time.December, 25, 23, 34, 0, 0, time.UTC)))
}

func TestParseDateId(t *testing.T) {
	t.Run("date format", func(t *testing.T) {
		for _, ts := range []time.Time{
			time.Date(2024, time.December, 7, 12, 25, 59, 0, time.UTC),
			time.Date(2024, time.November, 7, 12, 25, 59, 0, time.UTC),
			time.Date(1998, time.January, 1, 0, 1, 1, 0, time.UTC),
			time.Date(2124, time.December, 25, 23, 34, 0, 0, time.UTC),
		} {
			dateId := TimeToDateId(ts)
			ts2, err := ParseDateId(dateId)
			assert.NoError(t, err)
			assert.Equal(t, ts.Year(), ts2.Year())
			assert.Equal(t, ts.Month(), ts2.Month())
			assert.Equal(t, ts.Day(), ts2.Day())
			assert.Zero(t, ts2.Hour())
			assert.Zero(t, ts2.Minute())
			assert.Zero(t, ts2.Second())
		}
	})

	t.Run("wrong format", func(t *testing.T) {
		_, err := ParseDateId("_date_2024")
		assert.Error(t, err)

		_, err = ParseDateId("object1")
		assert.Error(t, err)
	})
}
