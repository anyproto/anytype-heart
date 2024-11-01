package date

import (
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

const (
	dateIdLayout   = "2006-01-02"
	dateNameLayout = "02 Jan 2006"
)

func TimeToDateId(t time.Time) string {
	return addr.DatePrefix + t.Format(dateIdLayout)
}

func ParseDateId(id string) (time.Time, error) {
	return time.Parse(dateIdLayout, strings.TrimPrefix(id, addr.DatePrefix))
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
