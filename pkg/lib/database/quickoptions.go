package database

import (
	"time"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	timeutil "github.com/anyproto/anytype-heart/util/time"
)

func transformQuickOption(protoFilter FilterRequest) []FilterRequest {
	if protoFilter.QuickOption == 0 && protoFilter.Format != model.RelationFormat_date {
		return []FilterRequest{protoFilter}
	}

	from, to := getDateRange(protoFilter, time.Now())
	switch protoFilter.Condition {
	case model.BlockContentDataviewFilter_Equal, model.BlockContentDataviewFilter_In:
		return []FilterRequest{{
			RelationKey: protoFilter.RelationKey,
			Condition:   model.BlockContentDataviewFilter_LessOrEqual,
			Value:       domain.Int64(to.Unix()),
		}, {
			RelationKey: protoFilter.RelationKey,
			Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			Value:       domain.Int64(from.Unix()),
		}}
	case model.BlockContentDataviewFilter_Less:
		protoFilter.Value = domain.Int64(from.Unix())
	case model.BlockContentDataviewFilter_Greater:
		protoFilter.Value = domain.Int64(to.Unix())
	case model.BlockContentDataviewFilter_LessOrEqual:
		protoFilter.Value = domain.Int64(to.Unix())
	case model.BlockContentDataviewFilter_GreaterOrEqual:
		protoFilter.Value = domain.Int64(from.Unix())
	}

	return []FilterRequest{protoFilter}
}

func getDateRange(f FilterRequest, now time.Time) (from, to time.Time) {
	calendar := timeutil.NewCalendar(now, nil)
	switch f.QuickOption {
	case model.BlockContentDataviewFilter_Yesterday:
		return calendar.DayNumStart(-1), calendar.DayNumEnd(-1)
	case model.BlockContentDataviewFilter_Today:
		return calendar.DayNumStart(0), calendar.DayNumEnd(0)
	case model.BlockContentDataviewFilter_Tomorrow:
		return calendar.DayNumStart(1), calendar.DayNumEnd(1)
	case model.BlockContentDataviewFilter_LastWeek:
		return calendar.WeekNumStart(-1), calendar.WeekNumEnd(-1)
	case model.BlockContentDataviewFilter_CurrentWeek:
		return calendar.WeekNumStart(0), calendar.WeekNumEnd(0)
	case model.BlockContentDataviewFilter_NextWeek:
		return calendar.WeekNumStart(1), calendar.WeekNumEnd(1)
	case model.BlockContentDataviewFilter_LastMonth:
		return calendar.MonthNumStart(-1), calendar.MonthNumEnd(-1)
	case model.BlockContentDataviewFilter_CurrentMonth:
		return calendar.MonthNumStart(0), calendar.MonthNumEnd(0)
	case model.BlockContentDataviewFilter_NextMonth:
		return calendar.MonthNumStart(1), calendar.MonthNumEnd(1)
	case model.BlockContentDataviewFilter_NumberOfDaysAgo:
		daysCnt := f.Value.Int64()
		return calendar.DayNumStart(-int(daysCnt)), calendar.DayNumEnd(-int(daysCnt))
	case model.BlockContentDataviewFilter_NumberOfDaysNow:
		daysCnt := f.Value.Int64()
		return calendar.DayNumStart(int(daysCnt)), calendar.DayNumEnd(int(daysCnt))
	case model.BlockContentDataviewFilter_LastYear:
		return calendar.YearNumStart(-1), calendar.YearNumEnd(-1)
	case model.BlockContentDataviewFilter_CurrentYear:
		return calendar.YearNumStart(0), calendar.YearNumEnd(0)
	case model.BlockContentDataviewFilter_NextYear:
		return calendar.YearNumStart(1), calendar.YearNumEnd(1)
	default:
		timestamp := f.Value.Int64()
		t := time.Unix(timestamp, 0)
		calendar = timeutil.NewCalendar(t, nil)
		return calendar.DayNumStart(0), calendar.DayNumEnd(0)
	}
}
