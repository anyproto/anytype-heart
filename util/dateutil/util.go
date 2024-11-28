package dateutil

import (
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

const (
	shortDateIdLayout   = "2006-01-02"
	dateIdLayout        = "2006-01-02-15-04-05Z-0700"
	shortDateNameLayout = "Mon, Jan 02, 2006"
	dateNameLayout      = "Mon, Jan 02, 2006 3:04 PM"
)

type DateObject interface {
	Id() string
	Name() string
	Time() time.Time
}

type dateObject struct {
	t           time.Time
	includeTime bool
}

func NewDateObject(t time.Time, includeTime bool) DateObject {
	return dateObject{t, includeTime}
}

func BuildDateObjectFromId(id string) (DateObject, error) {
	formatted := strings.TrimPrefix(id, addr.DatePrefix)
	formatted = strings.Replace(formatted, "_", "+", 1)
	t, err := time.Parse(dateIdLayout, formatted)
	if err == nil {
		return dateObject{t, true}, nil
	}
	t, err = time.ParseInLocation(shortDateIdLayout, formatted, time.Local)
	if err == nil {
		return dateObject{t, false}, nil
	}
	return dateObject{}, err
}

func (do dateObject) Id() string {
	if do.includeTime {
		formatted := do.t.Format(dateIdLayout)
		formatted = strings.Replace(formatted, "+", "_", 1)
		return addr.DatePrefix + formatted
	}
	return addr.DatePrefix + do.t.Format(shortDateIdLayout)
}

func (do dateObject) Name() string {
	if do.includeTime {
		return do.t.Format(dateNameLayout)
	}
	return do.t.Format(shortDateNameLayout)
}

func (do dateObject) Time() time.Time {
	return do.t
}
