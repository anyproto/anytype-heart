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
