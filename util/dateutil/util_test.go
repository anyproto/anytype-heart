package dateutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

func TestTimeToDateId(t *testing.T) {
	assert.Equal(t, "_date_2024-11-07-12-25-59", TimeToDateId(time.Date(2024, time.November, 7, 12, 25, 59, 0, time.UTC)))
	assert.Equal(t, "_date_1998-01-01-00-01-01", TimeToDateId(time.Date(1998, time.January, 1, 0, 1, 1, 0, time.UTC)))
	assert.Equal(t, "_date_2124-12-25-23-34-00", TimeToDateId(time.Date(2124, time.December, 25, 23, 34, 0, 0, time.UTC)))
}

func TestTimeToShortDateId(t *testing.T) {
	assert.Equal(t, "_date_2024-11-07", TimeToShortDateId(time.Date(2024, time.November, 7, 12, 25, 59, 0, time.UTC)))
	assert.Equal(t, "_date_1998-01-01", TimeToShortDateId(time.Date(1998, time.January, 1, 0, 1, 1, 0, time.UTC)))
	assert.Equal(t, "_date_2124-12-25", TimeToShortDateId(time.Date(2124, time.December, 25, 23, 34, 0, 0, time.UTC)))
}

func TestTimeToDateName(t *testing.T) {
	assert.Equal(t, "07 Nov 2024", TimeToDateName(time.Date(2024, time.November, 7, 12, 25, 59, 0, time.UTC)))
	assert.Equal(t, "01 Jan 1998", TimeToDateName(time.Date(1998, time.January, 1, 0, 1, 1, 0, time.UTC)))
	assert.Equal(t, "25 Dec 2124", TimeToDateName(time.Date(2124, time.December, 25, 23, 34, 0, 0, time.UTC)))
}

func TestParseDateId(t *testing.T) {
	t.Run("current date format", func(t *testing.T) {
		for _, ts := range []time.Time{
			time.Date(2024, time.December, 7, 12, 25, 59, 0, time.UTC),
			time.Date(2024, time.November, 7, 12, 25, 59, 0, time.UTC),
			time.Date(1998, time.January, 1, 0, 1, 1, 0, time.UTC),
			time.Date(2124, time.December, 25, 23, 34, 0, 0, time.UTC),
		} {
			ts2, err := ParseDateId(TimeToDateId(ts))
			assert.NoError(t, err)
			assert.Equal(t, ts, ts2)
		}
	})

	t.Run("old date format", func(t *testing.T) {
		ts := time.Date(2025, time.June, 8, 2, 25, 39, 0, time.UTC)
		ts2, err := ParseDateId(addr.DatePrefix + ts.Format(shortDateIdLayout))
		ts = ts.Truncate(24 * time.Hour)
		assert.NoError(t, err)
		assert.Equal(t, ts, ts2)
	})

	t.Run("wrong format", func(t *testing.T) {
		_, err := ParseDateId("_date_2024")
		assert.Error(t, err)

		_, err = ParseDateId("object1")
		assert.Error(t, err)
	})
}
