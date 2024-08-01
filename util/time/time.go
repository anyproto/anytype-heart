package time

import (
	"time"
)

func CutToDay(t time.Time) time.Time {
	roundTime := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return roundTime
}
