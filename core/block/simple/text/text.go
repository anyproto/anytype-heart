package text

import (
	"fmt"
	"sort"
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
)

var (
	ErrOutOfRange = fmt.Errorf("out of range")
)

func NewText(block *model.Block) *Text {
	tc := mustTextContent(block.Content.Content)
	t := &Text{Base: base.NewBase(block), content: tc}
	t.initMarks()
	return t
}

type Text struct {
	*base.Base
	content   *model.BlockContentText
	markTypes map[model.BlockContentTextMarkType]ranges
}

func mustTextContent(content model.IsBlockCoreContent) *model.BlockContentText {
	res, err := toTextContent(content)
	if err != nil {
		panic(err)
	}
	return res
}

func toTextContent(content model.IsBlockCoreContent) (textContent *model.BlockContentText, err error) {
	if cot, ok := content.(*model.BlockCoreContentOfText); ok {
		return cot.Text, nil
	}
	return nil, fmt.Errorf("unexpected content type: %T; want text", content)
}

func (t *Text) ApplyContentChanges(content model.IsBlockCoreContent) (err error) {
	tc, err := toTextContent(content)
	if err != nil {
		return
	}
	t.content = tc
	t.Model().Content = &model.BlockCore{Content: &model.BlockCoreContentOfText{Text: tc}}
	t.initMarks()
	return nil
}

func (t *Text) initMarks() {
	t.markTypes = make(map[model.BlockContentTextMarkType]ranges)
	if t.content.Marks == nil {
		t.content.Marks = &model.BlockContentTextMarks{}
	}
	for _, m := range t.content.Marks.Marks {
		if m != nil && m.Range != nil {
			ranges := t.markTypes[m.Type]
			ranges = append(ranges, m)
			t.markTypes[m.Type] = ranges
		}
	}
	for _, v := range t.markTypes {
		sort.Sort(v)
	}

	// TODO: group validate and join here
}

func (t *Text) AddMark(m *model.BlockContentTextMark) (err error) {
	// validate range
	if m.Range == nil || m.Range.From < 0 || m.Range.To <= 0 || m.Range.To <= m.Range.From {
		return ErrOutOfRange
	}
	if int(m.Range.To) > utf8.RuneCountInString(t.content.Text) {
		return ErrOutOfRange
	}

	marks := t.markTypes[m.Type]

	defer func() {
		if err == nil {
			sort.Sort(marks)
			t.markTypes[m.Type] = marks
			t.makeMarks()
		}
	}()

	const (
		notOverlap int = iota
		equal          // a equal b
		outer          // b inside a
		inner          // a inside b
		innerLeft      // a inside b, left side eq
		innerRight     // a inside b, right side eq
		left           // a-b
		right          // b-a
		stop
	)

	overlap := func(a, b *model.BlockContentTextMark) int {
		switch {
		case *a.Range == *b.Range:
			return equal
		case a.Range.From <= b.Range.From && a.Range.To >= b.Range.To:
			return outer
		case a.Range.From > b.Range.From && a.Range.To < b.Range.To:
			return inner
		case a.Range.From == b.Range.From && a.Range.To < b.Range.To:
			return innerLeft
		case a.Range.From > b.Range.From && a.Range.To == b.Range.To:
			return innerRight
		case a.Range.From < b.Range.From && b.Range.From <= a.Range.To:
			return left
		case a.Range.From > b.Range.From && b.Range.To >= a.Range.From:
			return right
		case a.Range.To < b.Range.From:
			return stop
		}
		return notOverlap
	}

	addM := true

	for i := 0; i < len(marks); i++ {
		var (
			delete bool
			e      = marks[i]
		)
		switch overlap(m, e) {
		case equal:
			if m.Param == "" {
				delete = true
			} else {
				e.Param = m.Param
			}
			addM = false
		case outer:
			delete = true
		case innerLeft:
			e.Range.From = m.Range.To
			if m.Param == "" {
				addM = false
			}
		case innerRight:
			e.Range.To = m.Range.From
			if m.Param == "" {
				addM = false
			}
		case inner:
			marks = append(marks, &model.BlockContentTextMark{
				Range: &model.Range{From: m.Range.To, To: e.Range.To},
				Type:  e.Type,
				Param: e.Param,
			})
			e.Range.To = m.Range.From
			if m.Param == "" {
				addM = false
			}
			i = len(marks)
		case left:
			if m.Param == e.Param {
				e.Range.From = m.Range.From
				addM = false
			} else {
				e.Range.From = m.Range.To
			}
		case right:
			if m.Param == e.Param {
				e.Range.To = m.Range.To
				m = e
				addM = false
			} else {
				e.Range.To = m.Range.From
			}
		case stop:
			i = len(marks)
		}
		if delete {
			marks[i] = nil
			marks = append(marks[:i], marks[i+1:]...)
			i = -1
		}
	}

	if addM {
		marks = append(marks, m)
	}
	return
}

func (t *Text) makeMarks() {
	var total int
	for _, ms := range t.markTypes {
		total += len(ms)
	}
	t.content.Marks = &model.BlockContentTextMarks{
		Marks: make([]*model.BlockContentTextMark, 0, total),
	}
	for _, ms := range t.markTypes {
		t.content.Marks.Marks = append(t.content.Marks.Marks, ms...)
	}
}

type ranges []*model.BlockContentTextMark

func (a ranges) Len() int           { return len(a) }
func (a ranges) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ranges) Less(i, j int) bool { return a[i].Range.From < a[j].Range.From }
