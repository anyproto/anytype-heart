package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func calculateDayEnd(base time.Time, daysOffset int) int64 {
	t := time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location())
	t = t.AddDate(0, 0, daysOffset)
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location()).Unix()
}

func calculateDayStart(base time.Time, daysOffset int) int64 {
	t := time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location())
	t = t.AddDate(0, 0, daysOffset)
	return t.Unix()
}

func calculateMonthStart(base time.Time, monthsOffset int) int64 {
	return time.Date(base.Year(), base.Month()+time.Month(monthsOffset), 1, 0, 0, 0, 0, base.Location()).Unix()
}

func calculateMonthEnd(base time.Time, monthsOffset int) int64 {
	firstDayOfMonth := time.Date(base.Year(), base.Month()+time.Month(monthsOffset), 1, 0, 0, 0, 0, base.Location())
	return firstDayOfMonth.AddDate(0, 1, 0).Add(-1 * time.Nanosecond).Unix()
}

func calculateWeekStartTime(base time.Time, weeksOffset int) time.Time {
	shift := -1 * ((base.Weekday() + 6) % 7)
	monday := time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location()).AddDate(0, 0, int(shift))
	return monday.AddDate(0, 0, weeksOffset*7)
}

func calculateWeekStart(base time.Time, weeksOffset int) int64 {
	return calculateWeekStartTime(base, weeksOffset).Unix()
}

func calculateWeekEnd(base time.Time, weeksOffset int) int64 {
	sunday := calculateWeekStartTime(base, weeksOffset).AddDate(0, 0, 6)
	return time.Date(sunday.Year(), sunday.Month(), sunday.Day(), 23, 59, 59, 0, sunday.Location()).Unix()
}

func TestQuickOption(t *testing.T) {
	var (
		relationKey = bundle.RelationKeyCreatedDate
		now         = time.Now()
	)

	// Test uses methods which account for daylight saving time
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
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, 0))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, 0))},
			},
		}, {
			"strictly before today",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Today,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Less,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(calculateDayStart(now, 0))}},
		}, {
			"today or before",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Today,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, 0))}},
		}, {
			"strictly after today",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Today,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(calculateDayEnd(now, 0))}},
		}, {
			"today or after",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Today,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, 0))}},
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
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, -1))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, -1))},
			},
		}, {
			"yesterday or after",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Yesterday,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, -1))}},
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
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, 1))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, 1))},
			},
		}, {
			"strictly after tomorrow",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Tomorrow,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(calculateDayEnd(now, 1))}},
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
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateWeekEnd(now, 0))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateWeekStart(now, 0))},
			},
		}, {
			"strictly before this week",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_CurrentWeek,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Less,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(calculateWeekStart(now, 0))}},
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
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateWeekEnd(now, -1))},
			},
		}, {
			"strictly after previous week",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastWeek,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(calculateWeekEnd(now, -1))}},
		}, {
			"strictly before previous week",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastWeek,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Less,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(calculateWeekStart(now, -1))}},
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
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateWeekEnd(now, 1))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateWeekStart(now, 1))},
			},
		}, {
			"next week and after",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NextWeek,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateWeekStart(now, 1))}},
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
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateMonthEnd(now, 0))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateMonthStart(now, 0))},
			},
		}, {
			"this month and later",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_CurrentMonth,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateMonthStart(now, 0))}},
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
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateMonthEnd(now, -1))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateMonthStart(now, -1))},
			},
		}, {
			"strictly before previous month",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastMonth,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Less,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(calculateMonthStart(now, -1))}},
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
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateMonthEnd(now, 1))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateMonthStart(now, 1))},
			},
		}, {
			"strictly after next month",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NextMonth,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(calculateMonthEnd(now, 1))}},
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
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, -6))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, -6))},
			},
		}, {
			"3 days ago or more",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysAgo,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
				Value:       domain.Int64(3),
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, -3))}},
		}, {
			"more than 4 days ago",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysAgo,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Less,
				Value:       domain.Int64(4),
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(calculateDayStart(now, -4))}},
		}, {
			"10 days ago and after",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysAgo,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
				Value:       domain.Int64(10),
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, -10))}},
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
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, 3))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, 3))},
			},
		}, {
			"in 12 days and after",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysNow,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
				Value:       domain.Int64(12),
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, 12))}},
		}, {
			"more than 100 days after",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysNow,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Greater,
				Value:       domain.Int64(100),
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(calculateDayEnd(now, 100))}},
		}, {
			"before next 7 days",
			FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysNow,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
				Value:       domain.Int64(7),
			},
			[]FilterRequest{{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, 7))}},
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
