package database

import (
	"time"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	timeutil "github.com/anyproto/anytype-heart/util/time"
)

func transformQuickOption(protoFilter FilterRequest, loc *time.Location) []FilterRequest {
	var filters []FilterRequest

	if protoFilter.QuickOption > model.BlockContentDataviewFilter_ExactDate || protoFilter.Format == model.RelationFormat_date {
		d1, d2 := getRange(protoFilter, loc)
		switch protoFilter.Condition {
		case model.BlockContentDataviewFilter_Equal:
			protoFilter.Condition = model.BlockContentDataviewFilter_GreaterOrEqual
			protoFilter.Value = domain.Int64(d1)

			filters = append(filters, FilterRequest{
				RelationKey: protoFilter.RelationKey,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
				Value:       domain.Int64(d2),
			})
		case model.BlockContentDataviewFilter_Less:
			protoFilter.Value = domain.Int64(d1)
		case model.BlockContentDataviewFilter_Greater:
			protoFilter.Value = domain.Int64(d2)
		case model.BlockContentDataviewFilter_LessOrEqual:
			protoFilter.Value = domain.Int64(d2)
		case model.BlockContentDataviewFilter_GreaterOrEqual:
			protoFilter.Value = domain.Int64(d1)
		case model.BlockContentDataviewFilter_In:
			protoFilter.Condition = model.BlockContentDataviewFilter_GreaterOrEqual
			protoFilter.Value = domain.Int64(d1)

			filters = append(filters, FilterRequest{
				RelationKey: protoFilter.RelationKey,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
				Value:       domain.Int64(d2),
			})
		}
	}

	filters = append(filters, protoFilter)
	return filters
}

func getRange(f FilterRequest, loc *time.Location) (int64, int64) {
	var d1, d2 time.Time
	calendar := timeutil.NewCalendar(time.Now(), loc)
	switch f.QuickOption {
	case model.BlockContentDataviewFilter_Yesterday:
		d1 = calendar.DayNumStart(-1)
		d2 = calendar.DayNumEnd(-1)
	case model.BlockContentDataviewFilter_Today:
		d1 = calendar.DayNumStart(0)
		d2 = calendar.DayNumEnd(0)
	case model.BlockContentDataviewFilter_Tomorrow:
		d1 = calendar.DayNumStart(1)
		d2 = calendar.DayNumEnd(1)
	case model.BlockContentDataviewFilter_LastWeek:
		d1 = calendar.WeekNumStart(-1)
		d2 = calendar.WeekNumEnd(-1)
	case model.BlockContentDataviewFilter_CurrentWeek:
		d1 = calendar.WeekNumStart(0)
		d2 = calendar.WeekNumEnd(0)
	case model.BlockContentDataviewFilter_NextWeek:
		d1 = calendar.WeekNumStart(1)
		d2 = calendar.WeekNumEnd(1)
	case model.BlockContentDataviewFilter_LastMonth:
		d1 = calendar.MonthNumStart(-1)
		d2 = calendar.MonthNumEnd(-1)
	case model.BlockContentDataviewFilter_CurrentMonth:
		d1 = calendar.MonthNumStart(0)
		d2 = calendar.MonthNumEnd(0)
	case model.BlockContentDataviewFilter_NextMonth:
		d1 = calendar.MonthNumStart(1)
		d2 = calendar.MonthNumEnd(1)
	case model.BlockContentDataviewFilter_NumberOfDaysAgo:
		daysCnt := f.Value.Int64()
		d1 = calendar.DayNumStart(-int(daysCnt))
		d2 = calendar.DayNumEnd(-1)
	case model.BlockContentDataviewFilter_NumberOfDaysNow:
		daysCnt := f.Value.Int64()
		d1 = calendar.DayNumStart(0)
		d2 = calendar.DayNumEnd(int(daysCnt))
	case model.BlockContentDataviewFilter_ExactDate:
		timestamp := f.Value.Int64()
		t := time.Unix(int64(timestamp), 0)
		calendar2 := timeutil.NewCalendar(t, loc)
		d1 = calendar2.DayNumStart(0)
		d2 = calendar2.DayNumEnd(0)
	}

	return d1.Unix(), d2.Unix()
}
