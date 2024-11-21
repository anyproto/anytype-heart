package dateutil

import (
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

const (
	shortDateIdLayout   = "2006-01-02"
	dateIdLayout        = "2006-01-02-15-04-05Z-0700"
	shortDateNameLayout = "02 Jan 2006"
	dateNameLayout      = "02 Jan 2006 15:04"
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

func ParseDateId(id string) (t time.Time, includeTime bool, err error) {
	formatted := strings.TrimPrefix(id, addr.DatePrefix)
	formatted = strings.Replace(formatted, "_", "+", 1)
	t, err = time.Parse(dateIdLayout, formatted)
	if err == nil {
		return t, true, nil
	}
	t, err = time.ParseInLocation(shortDateIdLayout, formatted, time.Local)
	return t, false, err
}

func TimeToDateName(t time.Time, includeTime bool) string {
	if includeTime {
		return t.Format(dateNameLayout)
	}
	return t.Format(shortDateNameLayout)
}

func DateNameToId(name string) (string, error) {
	t, err := time.Parse(dateNameLayout, name)
	if err != nil {
		return "", err
	}
	return TimeToDateId(t, false), nil
}
