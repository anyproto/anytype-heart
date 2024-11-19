package dateutil

import (
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

const (
	shortDateIdLayout = "2006-01-02"
	dateIdLayout      = "2006-01-02-15-04-05Z-0700"
	dateNameLayout    = "02 Jan 2006"
)

// TimeToDateId returns date object id. We substitute + with _ in time zone, as + is not supported in object ids on clients
// Format with time is _date_YYYY-MM-DD-hh-mm-ssZ-zzzz. Format without time _date_YYYY-MM-DD
func TimeToDateId(t time.Time, includeTime bool) string {
	if includeTime {
		formatted := t.Format(dateIdLayout)
		formatted = strings.Replace(formatted, "+", "_", 1)
		return addr.DatePrefix + formatted
	}
	return addr.DatePrefix + t.Format(shortDateIdLayout)
}

func ParseDateId(id string) (time.Time, error) {
	formatted := strings.TrimPrefix(id, addr.DatePrefix)
	formatted = strings.Replace(formatted, "_", "+", 1)
	t, err := time.Parse(dateIdLayout, formatted)
	if err == nil {
		return t, nil
	}
	return time.ParseInLocation(shortDateIdLayout, formatted, time.Local)
}

func TimeToDateName(t time.Time) string {
	return t.Format(dateNameLayout)
}

func DateNameToId(name string) (string, error) {
	t, err := time.Parse(dateNameLayout, name)
	if err != nil {
		return "", err
	}
	return TimeToDateId(t, false), nil
}
