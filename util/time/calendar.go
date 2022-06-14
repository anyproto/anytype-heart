package time

//TODO wrap all to structure and make common locale

import "time"

var Day = time.Hour * 24
var Week = Day * 7

func DayNumStart(dayNum int) time.Time {
	t := time.Now()
	year, month, day := t.Date()
	day = day + dayNum
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func DayNumEnd(dayNum int) time.Time {
	t := time.Now()
	year, month, day := t.Date()
	day = day + dayNum
	return time.Date(year, month, day, 23, 59, 59, 0, time.UTC)
}

func DayStart(t time.Time) time.Time{
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func DayEnd(needDate time.Time) time.Time {
	year, month, day := needDate.Date()
	return time.Date(year, month, day, 23, 59, 59, 0, time.UTC)
}

func WeekNumStart(weekNum int) time.Time {
	year, week := time.Now().ISOWeek()
	week = week + weekNum
	// Start from the middle of the year:
	t := time.Date(year, 7, 1, 0, 0, 0, 0, time.UTC)

	// Roll back to Monday:
	if wd := t.Weekday(); wd == time.Sunday {
		t = t.AddDate(0, 0, -6)
	} else {
		t = t.AddDate(0, 0, -int(wd)+1)
	}

	// Difference in weeks:
	_, w := t.ISOWeek()
	t = t.AddDate(0, 0, (week-w)*7)

	return t
}

func WeekNumEnd(weekNum int) time.Time {
	return WeekNumStart(weekNum).Add(Week).Add(time.Nanosecond * -1)
}

func WeekStart(needDate time.Time) time.Time {
	year, week := needDate.ISOWeek()
	// Start from the middle of the year:
	t := time.Date(year, 7, 1, 0, 0, 0, 0, time.UTC)

	// Roll back to Monday:
	if wd := t.Weekday(); wd == time.Sunday {
		t = t.AddDate(0, 0, -6)
	} else {
		t = t.AddDate(0, 0, -int(wd)+1)
	}

	// Difference in weeks:
	_, w := t.ISOWeek()
	t = t.AddDate(0, 0, (week-w)*7)

	return t
}

func WeekEnd(needDate time.Time) time.Time {
	return WeekStart(needDate).Add(Week).Add(time.Nanosecond * -1)
}

func MonthNumStart(monthNum int) time.Time {
	t := time.Now()
	needMonth := t.Month() + time.Month(monthNum)
	return time.Date(t.Year(), needMonth, 1, 0, 0, 0, 0, time.UTC)
}

func MonthNumEnd(monthNum int) time.Time {
	firstDay := MonthNumStart(monthNum)
	return firstDay.AddDate(0, 1, 0).Add(time.Nanosecond * -1)
}

func MonthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func MonthEnd(t time.Time) time.Time {
	firstDay := MonthStart(t)
	return firstDay.AddDate(0, 1, 0).Add(time.Nanosecond * -1)
}