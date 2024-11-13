package dateutil

import (
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

const (
	shortDateIdLayout = "2006-01-02"
	dateIdLayout      = "2006-01-02-15-04-05"
	dateNameLayout    = "02 Jan 2006"
)

func TimeToDateId(t time.Time) string {
	return addr.DatePrefix + t.Format(dateIdLayout)
}

// TimeToShortDateId should not be used to generate Date object id. Use TimeToDateId instead
func TimeToShortDateId(t time.Time) string {
	return addr.DatePrefix + t.Format(shortDateIdLayout)
}

func ParseDateId(id string) (time.Time, error) {
	t, err := time.Parse(dateIdLayout, strings.TrimPrefix(id, addr.DatePrefix))
	if err == nil {
		return t, nil
	}
	return time.Parse(shortDateIdLayout, strings.TrimPrefix(id, addr.DatePrefix))
}

func TimeToDateName(t time.Time) string {
	return t.Format(dateNameLayout)
}

func DateNameToId(name string) (string, error) {
	t, err := time.Parse(dateNameLayout, name)
	if err != nil {
		return "", err
	}
	return TimeToDateId(t), nil
}
