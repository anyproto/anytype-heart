package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const dayShift = 24 * 60 * 60

func TestQuickOption(t *testing.T) {
	var (
		relationKey = bundle.RelationKeyCreatedDate
		now         = time.Now()
		todayStart  = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
		todayEnd    = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location()).Unix()

		monthStart     = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
		monthEnd       = time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location()).Add(-1 * time.Nanosecond).Unix()
		prevMonthStart = time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location()).Unix()
		prevMonthEnd   = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Add(-1 * time.Nanosecond).Unix()
		nextMonthStart = time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location()).Unix()
		nextMonthEnd   = time.Date(now.Year(), now.Month()+2, 1, 0, 0, 0, 0, now.Location()).Add(-1 * time.Nanosecond).Unix()
	)

	var (
		mondayStart     = lastMondayStart(now)
		sundayEnd       = mondayStart + 7*dayShift - 1
		prevMondayStart = mondayStart - 7*dayShift
		prevSundayEnd   = sundayEnd - 7*dayShift
		nextMondayStart = mondayStart + 7*dayShift
		nextSundayEnd   = sundayEnd + 7*dayShift
	)

	for _, tc := range []struct {
		name            string
		inputFilter     FilterRequest
		expectedFilters []FilterRequest
	}{
		// today
		{
			"today",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Today,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
			},
			[]FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(todayEnd)},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(todayStart)},
			},
		}, {
			"strictly before today",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Today,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Less,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(todayStart)}},
		}, {
			"today or before",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Today,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(todayEnd)}},
		}, {
			"strictly after today",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Today,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(todayEnd)}},
		}, {
			"today or after",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Today,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(todayStart)}},
		},

		// yesterday
		{
			"yesterday",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Yesterday,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_In,
			},
			[]FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(todayEnd - dayShift)},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(todayStart - dayShift)},
			},
		}, {
			"yesterday or after",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Yesterday,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(todayStart - dayShift)}},
		},

		// tomorrow
		{
			"tomorrow",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Tomorrow,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_In,
			},
			[]FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(todayEnd + dayShift)},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(todayStart + dayShift)},
			},
		}, {
			"strictly after tomorrow",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Tomorrow,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(todayEnd + dayShift)}},
		},

		// this week
		{
			"current week",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_CurrentWeek,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
			},
			[]FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(sundayEnd)},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(mondayStart)},
			},
		}, {
			"strictly before this week",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_CurrentWeek,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Less,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(mondayStart)}},
		},

		// previous week
		{
			"previous week and before",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastWeek,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
			},
			[]FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(prevSundayEnd)},
			},
		}, {
			"strictly after previous week",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastWeek,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(prevSundayEnd)}},
		}, {
			"strictly before previous week",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastWeek,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Less,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(prevMondayStart)}},
		},

		// next week
		{
			"next week",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NextWeek,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_In,
			},
			[]FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(nextSundayEnd)},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(nextMondayStart)},
			},
		}, {
			"next week and after",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NextWeek,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(nextMondayStart)}},
		},

		// this month
		{
			"current month",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_CurrentMonth,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
			},
			[]FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(monthEnd)},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(monthStart)},
			},
		}, {
			"this month and later",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_CurrentMonth,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(monthStart)}},
		},

		// previous month
		{
			"previous month",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastMonth,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_In,
			},
			[]FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(prevMonthEnd)},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(prevMonthStart)},
			},
		}, {
			"strictly before previous month",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastMonth,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Less,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(prevMonthStart)}},
		},

		// next month
		{
			"next month",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NextMonth,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
			},
			[]FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(nextMonthEnd)},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(nextMonthStart)},
			},
		}, {
			"strictly after next month",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NextMonth,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(nextMonthEnd)}},
		},

		// number of days ago
		{
			"6 days ago",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysAgo,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(6),
			},
			[]FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(todayEnd - 6*dayShift)},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(todayStart - 6*dayShift)},
			},
		}, {
			"3 days ago or more",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysAgo,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
				Value:       domain.Int64(3),
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(todayEnd - 3*dayShift)}},
		}, {
			"more than 4 days ago",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysAgo,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Less,
				Value:       domain.Int64(4),
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(todayStart - 4*dayShift)}},
		}, {
			"10 days ago and after",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysAgo,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
				Value:       domain.Int64(10),
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(todayStart - 10*dayShift)}},
		},

		// number of days after
		{
			"in a day 3 days after",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysNow,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64(3),
			},
			[]FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(todayEnd + 3*dayShift)},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(todayStart + 3*dayShift)},
			},
		}, {
			"in 12 days and after",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysNow,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
				Value:       domain.Int64(12),
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(todayStart + 12*dayShift)}},
		}, {
			"more than 100 days after",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysNow,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Greater,
				Value:       domain.Int64(100),
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(todayEnd + 100*dayShift)}},
		}, {
			"before next 7 days",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysNow,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
				Value:       domain.Int64(7),
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(todayEnd + 7*dayShift)}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filters := transformQuickOption(tc.inputFilter)
			assert.Len(t, filters, len(tc.expectedFilters))
			for i, f := range filters {
				assert.Equal(t, tc.expectedFilters[i].Condition, f.Condition)
				assert.Equal(t, tc.expectedFilters[i].Value.Int64(), f.Value.Int64())
				assert.Equal(t, relationKey, f.RelationKey)
			}
		})
	}
}

func lastMondayStart(t time.Time) int64 {
	shift := -1 * ((t.Weekday() + 6) % 7)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).Add(24 * time.Duration(shift) * time.Hour).Unix()
}
