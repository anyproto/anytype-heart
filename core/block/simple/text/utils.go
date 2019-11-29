package text

import (
	"sort"

	"github.com/anytypeio/go-anytype-library/pb/model"
)

type sortedMarks []*model.BlockContentTextMark

func (s sortedMarks) Len() int {
	return len(s)
}

func (s sortedMarks) Less(i, j int) bool {
	if s[i].Type == s[j].Type {
		return getSafeRangeFrom(s[i].Range) < getSafeRangeFrom(s[j].Range)
	}
	return s[i].Type < s[j].Type
}

func (s sortedMarks) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func getSafeRangeFrom(r *model.Range) int32 {
	if r == nil {
		return 0
	}
	return r.From
}

func marksEq(s1, s2 *model.BlockContentTextMarks) bool {
	if s1 == nil {
		s1 = &model.BlockContentTextMarks{}
	}
	if s2 == nil {
		s2 = &model.BlockContentTextMarks{}
	}
	if len(s1.Marks) != len(s2.Marks) {
		return false
	}
	sort.Sort(sortedMarks(s1.Marks))
	sort.Sort(sortedMarks(s2.Marks))
	for i, v := range s1.Marks {
		if !markEq(v, s2.Marks[i]) {
			return false
		}
	}
	return true
}

func markEq(m1, m2 *model.BlockContentTextMark) bool {
	if m1.Type != m2.Type {
		return false
	}
	if m1.Param != m2.Param {
		return false
	}
	if *m1.Range != *m2.Range {
		return false
	}
	return true
}
