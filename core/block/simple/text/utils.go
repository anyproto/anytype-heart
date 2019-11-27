package text

import "github.com/anytypeio/go-anytype-library/pb/model"

type ranges []*model.BlockContentTextMark

func (a ranges) Len() int           { return len(a) }
func (a ranges) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ranges) Less(i, j int) bool { return a[i].Range.From < a[j].Range.From }

const (
	// a equal b
	equal int = iota
	// b inside a
	outer
	// a inside b
	inner
	// a inside b, left side eq
	innerLeft
	// a inside b, right side eq
	innerRight
	// a-b
	left
	// b-a
	right
	// a ... b
	before
	// b ... a
	after
)

func overlap(a, b *model.Range) int {
	switch {
	case *a == *b:
		return equal
	case a.To < b.From:
		return before
	case a.From > b.To:
		return after
	case a.From <= b.From && a.To >= b.To:
		return outer
	case a.From > b.From && a.To < b.To:
		return inner
	case a.From == b.From && a.To < b.To:
		return innerLeft
	case a.From > b.From && a.To == b.To:
		return innerRight
	case a.From < b.From && b.From <= a.To:
		return left
	default:
		return right
	}
}

func inInt(s []int, i int) bool {
	for _, si := range s {
		if si == i {
			return true
		}
	}
	return false
}

func marksByTypesEq(m1, m2 map[model.BlockContentTextMarkType]ranges) bool {
	for k, v := range m1 {
		if len(v) == 0 {
			delete(m1, k)
		}
	}
	for k, v := range m2 {
		if len(v) == 0 {
			delete(m2, k)
		}
	}
	if len(m1) != len(m2) {
		return false
	}
	for k, v := range m1 {
		if v2, ok := m2[k]; ok {
			if !rangesEq(v, v2) {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

func rangesEq(s1, s2 ranges) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i, v := range s1 {
		if !markEq(v, s2[i]) {
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
