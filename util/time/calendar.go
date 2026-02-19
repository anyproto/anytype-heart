package time

import "time"

var Day = time.Hour * 24
var Week = Day * 7

func NewCalendar(t time.Time, loc *time.Location) Calendar {
	if loc == nil {
		loc = time.Now().Location()
	}
	return Calendar{t: t, loc: loc}
}

type Calendar struct {
	t   time.Time
	loc *time.Location
}

func (c *Calendar) DayNumStart(dayNum int) time.Time {
	year, month, day := c.t.Date()
	day = day + dayNum
	return time.Date(year, month, day, 0, 0, 0, 0, c.loc)
}

func (c *Calendar) DayNumEnd(dayNum int) time.Time {
	year, month, day := c.t.Date()
	day = day + dayNum
	return time.Date(year, month, day, 23, 59, 59, 0, c.loc)
}

func (c *Calendar) WeekNumStart(weekNum int) time.Time {
	year, week := c.t.ISOWeek()
	week = week + weekNum
	// Start from the middle of the year:
	t := time.Date(year, 7, 1, 0, 0, 0, 0, c.loc)

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

func (c *Calendar) WeekNumEnd(weekNum int) time.Time {
	return c.WeekNumStart(weekNum).Add(Week).Add(time.Nanosecond * -1)
}

func (c *Calendar) MonthNumStart(monthNum int) time.Time {
	needMonth := c.t.Month() + time.Month(monthNum)
	return time.Date(c.t.Year(), needMonth, 1, 0, 0, 0, 0, c.loc)
}

func (c *Calendar) MonthNumEnd(monthNum int) time.Time {
	firstDay := c.MonthNumStart(monthNum)
	return firstDay.AddDate(0, 1, 0).Add(time.Nanosecond * -1)
}

func (c *Calendar) YearNumStart(yearDelta int) time.Time {
	needYear := c.t.Year() + yearDelta
	return time.Date(needYear, time.January, 1, 0, 0, 0, 0, c.loc)
}

func (c *Calendar) YearNumEnd(yearDelta int) time.Time {
	firstDay := c.YearNumStart(yearDelta)
	return firstDay.AddDate(1, 0, 0).Add(time.Nanosecond * -1)
}
