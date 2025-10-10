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

func calculateYearStart(base time.Time, yearsOffset int) int64 {
	return time.Date(base.Year()+yearsOffset, 1, 1, 0, 0, 0, 0, base.Location()).Unix()
}

func calculateYearEnd(base time.Time, yearsOffset int) int64 {
	return time.Date(base.Year()+yearsOffset, 12, 31, 23, 59, 59, 0, base.Location()).Unix()
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
			name: "today",
			inputFilter: FilterRequest{
				Format:      model.RelationFormat_date,
				QuickOption: model.BlockContentDataviewFilter_Today,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
			},
			expectedFilters: []FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, 0))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, 0))},
			},
		}, {
			name: "strictly before today",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Today,
				Format:      model.RelationFormat_date,
				RelationKey: relationKey,
				Condition:   model.BlockContentDataviewFilter_Less,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(calculateDayStart(now, 0))}},
		}, {
			name: "today or before",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Today,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, 0))}},
		}, {
			name: "strictly after today",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Today,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(calculateDayEnd(now, 0))}},
		}, {
			name: "today or after",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Today,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, 0))}},
		},

		// yesterday
		{
			name: "yesterday",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Yesterday,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_In,
			},
			expectedFilters: []FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, -1))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, -1))},
			},
		}, {
			name: "yesterday or after",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Yesterday,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, -1))}},
		},

		// tomorrow
		{
			name: "tomorrow",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Tomorrow,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_In,
			},
			expectedFilters: []FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, 1))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, 1))},
			},
		}, {
			name: "strictly after tomorrow",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_Tomorrow,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(calculateDayEnd(now, 1))}},
		},

		// this week
		{
			name: "current week",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_CurrentWeek,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Equal,
			},
			expectedFilters: []FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateWeekEnd(now, 0))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateWeekStart(now, 0))},
			},
		}, {
			name: "strictly before this week",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_CurrentWeek,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Less,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(calculateWeekStart(now, 0))}},
		},

		// previous week
		{
			name: "previous week and before",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastWeek,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
			},
			expectedFilters: []FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateWeekEnd(now, -1))},
			},
		}, {
			name: "strictly after previous week",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastWeek,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(calculateWeekEnd(now, -1))}},
		}, {
			name: "strictly before previous week",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastWeek,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Less,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(calculateWeekStart(now, -1))}},
		},

		// next week
		{
			name: "next week",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NextWeek,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_In,
			},
			expectedFilters: []FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateWeekEnd(now, 1))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateWeekStart(now, 1))},
			},
		}, {
			name: "next week and after",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NextWeek,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateWeekStart(now, 1))}},
		},

		// this month
		{
			name: "current month",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_CurrentMonth,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Equal,
			},
			expectedFilters: []FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateMonthEnd(now, 0))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateMonthStart(now, 0))},
			},
		}, {
			name: "this month and later",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_CurrentMonth,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateMonthStart(now, 0))}},
		},

		// previous month
		{
			name: "previous month",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastMonth,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_In,
			},
			expectedFilters: []FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateMonthEnd(now, -1))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateMonthStart(now, -1))},
			},
		}, {
			name: "strictly before previous month",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastMonth,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Less,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(calculateMonthStart(now, -1))}},
		},

		// next month
		{
			name: "next month",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NextMonth,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Equal,
			},
			expectedFilters: []FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateMonthEnd(now, 1))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateMonthStart(now, 1))},
			},
		}, {
			name: "strictly after next month",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NextMonth,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(calculateMonthEnd(now, 1))}},
		},

		// last year
		{
			name: "last year",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastYear,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Equal,
			},
			expectedFilters: []FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateYearEnd(now, -1))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateYearStart(now, -1))},
			},
		}, {
			name: "strictly before last year",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastYear,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Less,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(calculateYearStart(now, -1))}},
		}, {
			name: "last year or before",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastYear,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateYearEnd(now, -1))}},
		}, {
			name: "strictly after last year",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastYear,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(calculateYearEnd(now, -1))}},
		}, {
			name: "last year or after",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_LastYear,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateYearStart(now, -1))}},
		},

		// current year
		{
			name: "current year",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_CurrentYear,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Equal,
			},
			expectedFilters: []FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateYearEnd(now, 0))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateYearStart(now, 0))},
			},
		}, {
			name: "strictly before current year",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_CurrentYear,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Less,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(calculateYearStart(now, 0))}},
		}, {
			name: "current year or before",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_CurrentYear,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateYearEnd(now, 0))}},
		}, {
			name: "strictly after current year",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_CurrentYear,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(calculateYearEnd(now, 0))}},
		}, {
			name: "current year or after",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_CurrentYear,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateYearStart(now, 0))}},
		},

		// next year
		{
			name: "next year",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NextYear,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Equal,
			},
			expectedFilters: []FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateYearEnd(now, 1))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateYearStart(now, 1))},
			},
		}, {
			name: "strictly before next year",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NextYear,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Less,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(calculateYearStart(now, 1))}},
		}, {
			name: "next year or before",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NextYear,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateYearEnd(now, 1))}},
		}, {
			name: "strictly after next year",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NextYear,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(calculateYearEnd(now, 1))}},
		}, {
			name: "next year or after",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NextYear,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateYearStart(now, 1))}},
		},

		// number of days ago
		{
			name: "6 days ago",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysAgo,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(6),
			},
			expectedFilters: []FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, -6))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, -6))},
			},
		}, {
			name: "3 days ago or more",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysAgo,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
				Value:       domain.Int64(3),
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, -3))}},
		}, {
			name: "more than 4 days ago",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysAgo,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Less,
				Value:       domain.Int64(4),
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Less, Value: domain.Int64(calculateDayStart(now, -4))}},
		}, {
			name: "10 days ago and after",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysAgo,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
				Value:       domain.Int64(10),
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, -10))}},
		},

		// number of days after
		{
			name: "in a day 3 days after",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysNow,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64(3),
			},
			expectedFilters: []FilterRequest{
				{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, 3))},
				{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, 3))},
			},
		}, {
			name: "in 12 days and after",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysNow,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
				Value:       domain.Int64(12),
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_GreaterOrEqual, Value: domain.Int64(calculateDayStart(now, 12))}},
		}, {
			name: "more than 100 days after",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysNow,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_Greater,
				Value:       domain.Int64(100),
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_Greater, Value: domain.Int64(calculateDayEnd(now, 100))}},
		}, {
			name: "before next 7 days",
			inputFilter: FilterRequest{
				QuickOption: model.BlockContentDataviewFilter_NumberOfDaysNow,
				RelationKey: relationKey,
				Format:      model.RelationFormat_date,
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
				Value:       domain.Int64(7),
			},
			expectedFilters: []FilterRequest{{Condition: model.BlockContentDataviewFilter_LessOrEqual, Value: domain.Int64(calculateDayEnd(now, 7))}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filters := transformDateFilter(tc.inputFilter)
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
