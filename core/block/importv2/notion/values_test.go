package notion

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNotionDate(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)

	t.Run("RFC3339 with offset", func(t *testing.T) {
		got, includeTime, err := parseNotionDate("2020-12-08T12:00:00.000+01:00", "")
		require.NoError(t, err)
		assert.True(t, includeTime)
		assert.Equal(t, time.Date(2020, 12, 8, 11, 0, 0, 0, time.UTC).Unix(), got)
	})

	t.Run("offset-less datetime honors the IANA time zone", func(t *testing.T) {
		// The API contract: with a time_zone set, start/end carry NO offset.
		got, includeTime, err := parseNotionDate("2020-12-08T12:00:00", "Europe/Berlin")
		require.NoError(t, err)
		assert.True(t, includeTime)
		assert.Equal(t, time.Date(2020, 12, 8, 12, 0, 0, 0, berlin).Unix(), got)
	})

	t.Run("offset-less datetime with fractional seconds", func(t *testing.T) {
		got, includeTime, err := parseNotionDate("2020-12-08T12:00:00.123", "Europe/Berlin")
		require.NoError(t, err)
		assert.True(t, includeTime)
		assert.Equal(t, time.Date(2020, 12, 8, 12, 0, 0, 123000000, berlin).Unix(), got)
	})

	t.Run("date-only", func(t *testing.T) {
		got, includeTime, err := parseNotionDate("2024-03-05", "")
		require.NoError(t, err)
		assert.False(t, includeTime, "date-only values must not claim a time of day")
		assert.Equal(t, time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC).Unix(), got)
	})

	t.Run("date-only in a zone", func(t *testing.T) {
		got, _, err := parseNotionDate("2024-03-05", "Europe/Berlin")
		require.NoError(t, err)
		assert.Equal(t, time.Date(2024, 3, 5, 0, 0, 0, 0, berlin).Unix(), got)
	})

	t.Run("malformed dates error instead of epoch 0", func(t *testing.T) {
		for _, value := range []string{"garbage", "", "12:00:00"} {
			_, _, err := parseNotionDate(value, "")
			assert.Error(t, err, "value %q", value)
		}
	})
}

func TestPlaceValueText(t *testing.T) {
	// Notion's place property carries what the user picked on a map. Anytype
	// has no place format, but the text is real data.
	cases := []struct {
		name  string
		place *placeValue
		want  string
	}{
		{"name and a fuller address read as one line", &placeValue{Name: "Golden Gate Bridge", Address: "Golden Gate Brg, San Francisco, CA"}, "Golden Gate Bridge, Golden Gate Brg, San Francisco, CA"},
		{"an address that repeats the name is not said twice", &placeValue{Name: "1 Apple Park Way, Cupertino", Address: "1 Apple Park Way, Cupertino"}, "1 Apple Park Way, Cupertino"},
		{"an address that starts with the name is not said twice", &placeValue{Name: "The Kremlin", Address: "The Kremlin, Nizhny Novgorod"}, "The Kremlin"},
		{"a place with only an address keeps it", &placeValue{Address: "Church St, New York"}, "Church St, New York"},
		{"an empty place is no value at all", &placeValue{}, ""},
		{"no place at all", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.place.text())
		})
	}
}
