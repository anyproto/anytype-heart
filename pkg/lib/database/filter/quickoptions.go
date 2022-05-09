package filter

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	timeutil "github.com/anytypeio/go-anytype-middleware/util/time"
	"time"
)

var day = time.Hour * 24
var week = day * 7
var month = week * 30

func TransformQuickOption(reqFilters *[]*model.BlockContentDataviewFilter) {
	if reqFilters == nil {
		return
	}

	for _, f := range *reqFilters {
		if f.QuickOption > model.BlockContentDataviewFilter_DateNone {

			d1, d2 := getNeedDate(f)
			switch f.Condition {
			case model.BlockContentDataviewFilter_Equal:
				f.Condition = model.BlockContentDataviewFilter_Greater
				f.Value = pb.ToValue(timeutil.DayStart(d1).Unix())

				*reqFilters = append(*reqFilters, &model.BlockContentDataviewFilter{
					RelationKey: f.RelationKey,
					Condition:   model.BlockContentDataviewFilter_Less,
					Value:       pb.ToValue(timeutil.DayEnd(d2).Unix()),
				})
			case model.BlockContentDataviewFilter_Less:
				f.Value = pb.ToValue(timeutil.DayStart(d1).Unix())
			case model.BlockContentDataviewFilter_Greater:
				f.Value = pb.ToValue(timeutil.DayEnd(d2).Unix())
			case model.BlockContentDataviewFilter_LessOrEqual:
				f.Value = pb.ToValue(timeutil.DayEnd(d2).Unix())
			case model.BlockContentDataviewFilter_GreaterOrEqual:
				f.Value = pb.ToValue(timeutil.DayStart(d1).Unix())
			case model.BlockContentDataviewFilter_In:
				f.Condition = model.BlockContentDataviewFilter_Greater
				f.Value = pb.ToValue(timeutil.DayStart(d1).Unix())

				*reqFilters = append(*reqFilters, &model.BlockContentDataviewFilter{
					RelationKey: f.RelationKey,
					Condition:   model.BlockContentDataviewFilter_Less,
					Value:       pb.ToValue(timeutil.DayEnd(d2).Unix()),
				})
			}
			f.QuickOption = 0
		}
	}
}

func getNeedDate(f *model.BlockContentDataviewFilter) (time.Time, time.Time) {
	var d1, d2 time.Time
	switch f.QuickOption {
	case model.BlockContentDataviewFilter_Today:
		d1 = time.Now()
		d2 = d1
	case model.BlockContentDataviewFilter_Tomorrow:
		d1 = time.Now().Add(day)
		d2 = d2
	case model.BlockContentDataviewFilter_Yesterday:
		d1 = time.Now().Add(-day)
		d2 = d1
	case model.BlockContentDataviewFilter_OneWeekAgo:
		d1 = time.Now().Add(-week)
		d2 = time.Now()
	case model.BlockContentDataviewFilter_OneWeekFromNow:
		d1 = time.Now()
		d2 = time.Now().Add(week)
	case model.BlockContentDataviewFilter_OneMonthAgo:
		d1 = time.Now().Add(-month)
		d2 = time.Now()
	case model.BlockContentDataviewFilter_OneMonthFromNow:
		d1 = time.Now()
		d2 = time.Now().Add(month)
	case model.BlockContentDataviewFilter_NumberOfDaysAgo:
		daysCnt := f.Value.GetNumberValue()
		d1 = time.Now().Add(-(day * time.Duration(daysCnt)))
		d2 = time.Now()
	case model.BlockContentDataviewFilter_NumberOfDaysNow:
		daysCnt := f.Value.GetNumberValue()
		d1 = time.Now()
		d2 = time.Now().Add(day * time.Duration(daysCnt))
	case model.BlockContentDataviewFilter_ExactDate:
		timestamp := f.GetValue().GetNumberValue()
		d1 = time.Unix(int64(timestamp), 0)
		d2 = time.Unix(int64(timestamp), 0)
	}

	return d1, d2
}
