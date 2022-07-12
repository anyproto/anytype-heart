package filter

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	timeutil "github.com/anytypeio/go-anytype-middleware/util/time"
	"time"
)

func TransformQuickOption(reqFilters []*model.BlockContentDataviewFilter, loc *time.Location) []*model.BlockContentDataviewFilter {
	if reqFilters == nil {
		return nil
	}

	for _, f := range reqFilters {
		if f.QuickOption > model.BlockContentDataviewFilter_ExactDate {
			d1, d2 := getRange(f, loc)
			switch f.Condition {
			case model.BlockContentDataviewFilter_Equal:
				f.Condition = model.BlockContentDataviewFilter_GreaterOrEqual
				f.Value = pb.ToValue(d1)

				reqFilters = append(reqFilters, &model.BlockContentDataviewFilter{
					RelationKey: f.RelationKey,
					Condition:   model.BlockContentDataviewFilter_LessOrEqual,
					Value:       pb.ToValue(d2),
				})
			case model.BlockContentDataviewFilter_Less:
				f.Value = pb.ToValue(d1)
			case model.BlockContentDataviewFilter_Greater:
				f.Value = pb.ToValue(d2)
			case model.BlockContentDataviewFilter_LessOrEqual:
				f.Value = pb.ToValue(d2)
			case model.BlockContentDataviewFilter_GreaterOrEqual:
				f.Value = pb.ToValue(d1)
			case model.BlockContentDataviewFilter_In:
				f.Condition = model.BlockContentDataviewFilter_GreaterOrEqual
				f.Value = pb.ToValue(d1)

				reqFilters = append(reqFilters, &model.BlockContentDataviewFilter{
					RelationKey: f.RelationKey,
					Condition:   model.BlockContentDataviewFilter_LessOrEqual,
					Value:       pb.ToValue(d2),
				})
			}
			f.QuickOption = 0
		}
	}

	return reqFilters
}

func getRange(f *model.BlockContentDataviewFilter, loc *time.Location) (int64, int64) {
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
		daysCnt := f.Value.GetNumberValue()
		d1 = calendar.DayNumStart(-int(daysCnt))
		d2 = calendar.DayNumEnd(-1)
	case model.BlockContentDataviewFilter_NumberOfDaysNow:
		daysCnt := f.Value.GetNumberValue()
		d1 = calendar.DayNumStart(0)
		d2 = calendar.DayNumEnd(int(daysCnt))
	//case model.BlockContentDataviewFilter_ExactDate:
	//	timestamp := f.GetValue().GetNumberValue()
	//	t := time.Unix(int64(timestamp), 0)
	//	d1 = timeutil.DayStart(t)
	//	d2 = timeutil.DayEnd(t)
	}

	return d1.Unix(), d2.Unix()
}
