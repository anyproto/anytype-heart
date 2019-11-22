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

	// find intersected marks
	var intersectIdx []int
	for i, e := range marks {
		if e.Range.From <= m.Range.To && e.Range.To >= m.Range.From {
			intersectIdx = append(intersectIdx, i)
		}
		if m.Range.To < e.Range.From {
			break
		}
	}

	// not intersection - just add new one
	if len(intersectIdx) == 0 {
		marks = append(marks, m)
		return
	}

	var (
		toDeleteIdx []int
		solved      bool
	)

	// one intersection - toggle cases
	if len(intersectIdx) == 1 && m.Param == "" {
		e := marks[intersectIdx[0]]
		switch {
		// toggle existing - just delete mark
		case *e.Range == *m.Range:
			toDeleteIdx = intersectIdx
			solved = true
		// toggle part
		case e.Range.From <= m.Range.From && e.Range.To >= m.Range.To:
			if e.Range.From == m.Range.From {
				// cut left
				e.Range.From = m.Range.To
			} else if e.Range.To == m.Range.To {
				// cut right
				e.Range.To = m.Range.From
			} else {
				// toggle center - split for two marks
				marks = append(marks, &model.BlockContentTextMark{
					Range: &model.Range{
						From: m.Range.To,
						To:   e.Range.To,
					},
					Type:  e.Type,
					Param: e.Param,
				})
				e.Range.To = m.Range.From
			}
			solved = true
		}
	}

	if ! solved {

	}

	// delete
	for _, idx := range toDeleteIdx {
		marks[idx] = nil
		marks = append(marks[:idx], marks[idx+1:]...)
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
