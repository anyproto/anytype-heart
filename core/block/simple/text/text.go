package text

import (
	"fmt"
	"sort"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	textutil "github.com/anyproto/anytype-heart/util/text"
	"github.com/anyproto/anytype-heart/util/uri"
)

var (
	ErrOutOfRange = fmt.Errorf("out of range")
	log           = logging.Logger("anytype-text")
)

func init() {
	simple.RegisterCreator(func(m *model.Block) simple.Block {
		if _, err := toTextContent(m.Content); err != nil {
			return nil
		}
		if keysList := pbtypes.GetStringList(m.GetFields(), DetailsKeyFieldName); len(keysList) > 0 {
			keys := newDetailKeys(keysList)
			return NewDetails(m, keys)
		}
		return NewText(m)
	})
}

func NewText(block *model.Block) simple.Block {
	tc := mustTextContent(block.Content)
	t := &Text{Base: base.NewBase(block).(*base.Base), content: tc}
	return t
}

type Block interface {
	simple.Block
	SetText(text string, marks *model.BlockContentTextMarks)
	GetText() (text string)
	SetStyle(style model.BlockContentTextStyle)
	SetChecked(v bool)
	GetChecked() bool
	SetMarkForAllText(mark *model.BlockContentTextMark)
	RemoveMarkType(markType model.BlockContentTextMarkType)
	HasMarkForAllText(mark *model.BlockContentTextMark) bool
	SetTextColor(color string)
	SetIconEmoji(emoji string)
	SetIconImage(imageHash string)

	Split(pos int32) (simple.Block, error)
	RangeSplit(from int32, to int32, top bool) (newBlock simple.Block, err error)
	RangeTextPaste(rangeFrom int32, rangeTo int32, copiedBlock *model.Block, isPartOfBlock bool) (caretPosition int32, err error)
	RangeCut(from int32, to int32) (cutBlock *model.Block, initialBlock *model.Block, err error)
	Merge(b simple.Block, opts ...MergeOption) error
	SplitMarks(textRange *model.Range, newMarks []*model.BlockContentTextMark, newText string) (combinedMarks []*model.BlockContentTextMark)
	FillSmartIds(ids []string) []string
	HasSmartIds() bool
	ApplyEvent(e *pb.EventBlockSetText) error
	MigrateFile(migrateFunc func(oldHash string) (newHash string))

	IsEmpty() bool
}

type Text struct {
	*base.Base
	content *model.BlockContentText
}

func mustTextContent(content model.IsBlockContent) *model.BlockContentText {
	res, err := toTextContent(content)
	if err != nil {
		panic(err)
	}
	return res
}

func toTextContent(content model.IsBlockContent) (textContent *model.BlockContentText, err error) {
	if cot, ok := content.(*model.BlockContentOfText); ok {
		if cot.Text.Marks == nil {
			cot.Text.Marks = &model.BlockContentTextMarks{}
		}
		return cot.Text, nil
	}
	return nil, fmt.Errorf("unexpected content type: %T; want text", content)
}

func (t *Text) Copy() simple.Block {
	return NewText(pbtypes.CopyBlock(t.Model()))
}

// Validate TODO: add validation rules
func (t *Text) Validate() error {
	return nil
}

func (t *Text) Diff(spaceId string, b simple.Block) (msgs []simple.EventMessage, err error) {
	text, ok := b.(*Text)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = t.Base.Diff(spaceId, text); err != nil {
		return
	}
	changes := &pb.EventBlockSetText{
		Id: text.Id,
	}
	hasChanges := false

	if t.content.Text != text.content.Text {
		hasChanges = true
		changes.Text = &pb.EventBlockSetTextText{Value: text.content.Text}
	}
	if t.content.Style != text.content.Style {
		hasChanges = true
		changes.Style = &pb.EventBlockSetTextStyle{Value: text.content.Style}
	}
	if t.content.Checked != text.content.Checked {
		hasChanges = true
		changes.Checked = &pb.EventBlockSetTextChecked{Value: text.content.Checked}
	}
	if !marksEq(t.content.Marks, text.content.Marks) {
		hasChanges = true
		changes.Marks = &pb.EventBlockSetTextMarks{Value: text.content.Marks}
	}
	if t.content.Color != text.content.Color {
		hasChanges = true
		changes.Color = &pb.EventBlockSetTextColor{Value: text.content.Color}
	}
	if t.content.IconImage != text.content.IconImage {
		hasChanges = true
		changes.IconImage = &pb.EventBlockSetTextIconImage{Value: text.content.IconImage}
	}
	if t.content.IconEmoji != text.content.IconEmoji {
		hasChanges = true
		changes.IconEmoji = &pb.EventBlockSetTextIconEmoji{Value: text.content.IconEmoji}
	}
	if hasChanges {
		msgs = append(msgs, simple.EventMessage{Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockSetText{BlockSetText: changes})})
	}
	return
}

func (t *Text) SetStyle(style model.BlockContentTextStyle) {
	t.content.Style = style
}

func (t *Text) SetChecked(v bool) {
	t.content.Checked = v
}

func (t *Text) GetChecked() bool {
	return t.content.Checked
}

func (t *Text) SetTextColor(color string) {
	t.content.Color = color
}

func (t *Text) SetIconEmoji(emoji string) {
	t.content.IconEmoji = emoji
}

func (t *Text) SetIconImage(imageHash string) {
	t.content.IconImage = imageHash
}

func (t *Text) FillFileHashes(hashes []string) []string {
	if h := t.content.IconImage; h != "" {
		return append(hashes, h)
	}
	return hashes
}

func (t *Text) MigrateFile(migrateFunc func(oldHash string) (newHash string)) {
	if t.content.IconImage != "" {
		t.content.IconImage = migrateFunc(t.content.IconImage)
	}
	t.ReplaceLinkIds(migrateFunc)
}

func (t *Text) IterateLinkedFiles(iter func(id string)) {
	if h := t.content.IconImage; h != "" {
		iter(h)
	}
}

func (t *Text) SetMarkForAllText(mark *model.BlockContentTextMark) {
	mRange := &model.Range{
		To: int32(textutil.UTF16RuneCountString(t.content.Text)),
	}
	if t.content.Marks == nil {
		t.content.Marks = &model.BlockContentTextMarks{}
	}
	filteredMarks := t.content.Marks.Marks[:0]
	for _, m := range t.content.Marks.Marks {
		if m.Type != mark.Type && !isIncompatibleType(m.Type, mark.Type) {
			filteredMarks = append(filteredMarks, m)
		}
	}
	t.content.Marks.Marks = filteredMarks
	t.content.Marks.Marks = append(t.content.Marks.Marks, &model.BlockContentTextMark{
		Range: mRange,
		Type:  mark.Type,
		Param: mark.Param,
	})
	return
}

func (t *Text) RemoveMarkType(markType model.BlockContentTextMarkType) {
	if t.content.Marks == nil {
		t.content.Marks = &model.BlockContentTextMarks{}
	}
	filteredMarks := t.content.Marks.Marks[:0]
	for _, m := range t.content.Marks.Marks {
		if m.Type != markType {
			filteredMarks = append(filteredMarks, m)
		}
	}
	t.content.Marks.Marks = filteredMarks
	return
}

func (t *Text) HasMarkForAllText(mark *model.BlockContentTextMark) bool {
	mRange := &model.Range{
		To: int32(textutil.UTF16RuneCountString(t.content.Text)),
	}
	for _, m := range t.content.Marks.Marks {
		if m.Type == mark.Type && m.Param == mark.Param {
			if m.Range.From == mRange.From && m.Range.To >= mRange.To {
				return true
			}
		}
	}
	return false
}

func (t *Text) SetText(text string, marks *model.BlockContentTextMarks) {
	t.content.Text = text
	if marks == nil {
		marks = &model.BlockContentTextMarks{}
	} else {
		for mI, _ := range marks.Marks {
			if marks.Marks[mI].Type == model.BlockContentTextMark_Link {
				m, err := uri.NormalizeURI(marks.Marks[mI].Param)
				if err == nil {
					marks.Marks[mI].Param = m
				}
			}
		}
	}
	t.content.Marks = marks

	t.normalizeMarks()
	return
}

func (t *Text) GetText() (text string) {
	return t.content.Text
}

func (t *Text) Split(pos int32) (simple.Block, error) {
	if pos < 0 || int(pos) > textutil.UTF16RuneCountString(t.content.Text) {
		return nil, ErrOutOfRange
	}
	runes := textutil.StrToUTF16(t.content.Text)
	t.content.Text = textutil.UTF16ToStr(runes[pos:])
	if t.content.Marks == nil {
		t.content.Marks = &model.BlockContentTextMarks{}
	}
	newMarks := &model.BlockContentTextMarks{}
	oldMarks := &model.BlockContentTextMarks{}
	for _, mark := range t.content.Marks.Marks {
		if mark.Range.From >= pos {
			mark.Range.From -= pos
			mark.Range.To -= pos
			newMarks.Marks = append(newMarks.Marks, mark)
		} else if mark.Range.To <= pos {
			oldMarks.Marks = append(oldMarks.Marks, mark)
		} else {
			newMark := &model.BlockContentTextMark{
				Range: &model.Range{
					From: 0,
					To:   mark.Range.To - pos,
				},
				Type:  mark.Type,
				Param: mark.Param,
			}
			newMarks.Marks = append(newMarks.Marks, newMark)
			mark.Range.To = pos
			oldMarks.Marks = append(oldMarks.Marks, mark)
		}
	}
	t.content.Marks = newMarks
	newBlock := simple.New(&model.Block{
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:    textutil.UTF16ToStr(runes[:pos]),
			Style:   t.content.Style,
			Marks:   oldMarks,
			Checked: t.content.Checked,
			Color:   t.content.Color,
		}},
		BackgroundColor: t.BackgroundColor,
		Align:           t.Align,
	})
	return newBlock, nil
}

func (t *Text) RangeTextPaste(rangeFrom int32, rangeTo int32, copiedBlock *model.Block, copyStyle bool) (caretPosition int32, err error) {
	caretPosition = -1
	copiedText := copiedBlock.GetText()

	copyFrom := int32(0)
	copyTo := int32(textutil.UTF16RuneCountString(copiedText.Text))

	textLen := textutil.UTF16RuneCountString(t.content.Text)
	if rangeFrom < 0 || int(rangeFrom) > textLen {
		return caretPosition, fmt.Errorf("out of range: range.from is not correct: %d", rangeFrom)
	}
	if rangeTo < 0 || int(rangeTo) > textLen {
		return caretPosition, fmt.Errorf("out of range: range.to is not correct: %d", rangeTo)
	}
	if rangeFrom > rangeTo {
		return caretPosition, fmt.Errorf("out of range: range.from %d > range.to %d", rangeFrom, rangeTo)
	}

	ifFullTextCopied := rangeFrom == 0 && rangeTo == int32(textLen)
	isFullReplace := ifFullTextCopied && textLen > 0
	isPlaceInEmptyParagraph := ifFullTextCopied && textLen == 0 && (t.content.Style == model.BlockContentText_Paragraph)

	if isFullReplace || isPlaceInEmptyParagraph {
		if copyStyle {
			t.content.Style = copiedText.Style
			t.content.Color = copiedText.Color
			t.BackgroundColor = copiedBlock.BackgroundColor
		}
	}

	// 1. cut marks from 0 to TO
	copiedText.Marks.Marks, _ = t.splitMarks(copiedText.Marks.Marks, &model.Range{From: copyTo, To: copyTo}, 0)

	// 2. cut marks from FROM to TO
	_, copiedText.Marks.Marks = t.splitMarks(copiedText.Marks.Marks, &model.Range{From: copyFrom, To: copyFrom}, 0)
	for _, m := range copiedText.Marks.Marks {
		m.Range.From = m.Range.From - copyFrom
		m.Range.To = m.Range.To - copyFrom
	}

	// 3. combine
	contentText := textutil.StrToUTF16(t.content.Text)
	runesFirst := contentText[:rangeFrom]
	runesMiddle := textutil.StrToUTF16(copiedText.Text)[copyFrom:copyTo]
	runesLast := contentText[rangeTo:]

	combinedMarks := t.SplitMarks(&model.Range{From: rangeFrom, To: rangeTo}, copiedText.Marks.Marks, textutil.UTF16ToStr(runesMiddle))
	t.content.Marks.Marks = t.normalizeMarksPure(combinedMarks)

	t.content.Text = textutil.UTF16ToStr(runesFirst) + textutil.UTF16ToStr(runesMiddle) + textutil.UTF16ToStr(runesLast)

	caretPosition = rangeFrom + (copyTo - copyFrom)
	return caretPosition, nil
}

func (t *Text) RangeCut(from int32, to int32) (cutBlock *model.Block, initialBlock *model.Block, err error) {
	if from < 0 || int(from) > textutil.UTF16RuneCountString(t.content.Text) {
		return nil, nil, ErrOutOfRange
	}
	if to < 0 || int(to) > textutil.UTF16RuneCountString(t.content.Text) {
		return nil, nil, ErrOutOfRange
	}
	if from > to {
		return nil, nil, ErrOutOfRange
	}

	contentText := textutil.StrToUTF16(t.content.Text)
	runesFirst := contentText[:from]
	runesMiddle := contentText[from:to]
	runesLast := contentText[to:]

	// make a copy of the block
	cutBlock = t.Copy().Model()
	// set text, marks to the cutBlock

	// 1. cut marks from 0 to TO
	cutBlock.GetText().Marks.Marks, _ = t.splitMarks(t.content.Marks.Marks, &model.Range{From: to, To: to}, 0)
	// 2. cut marks from FROM to TO
	_, cutBlock.GetText().Marks.Marks = t.splitMarks(cutBlock.GetText().Marks.Marks, &model.Range{From: from, To: from}, 0)

	initialBlock = t.Copy().Model()
	initialBlock.GetText().Text = textutil.UTF16ToStr(runesFirst) + textutil.UTF16ToStr(runesLast)
	initialBlock.GetText().Marks.Marks = t.SplitMarks(&model.Range{From: from, To: to}, []*model.BlockContentTextMark{}, "")

	cutBlock.GetText().Text = textutil.UTF16ToStr(runesMiddle)

	return cutBlock, initialBlock, nil
}

func (t *Text) RangeSplit(from int32, to int32, top bool) (newBlock simple.Block, err error) {
	if from < 0 || int(from) > textutil.UTF16RuneCountString(t.content.Text) {
		return nil, ErrOutOfRange
	}
	if to < 0 || int(to) > textutil.UTF16RuneCountString(t.content.Text) {
		return nil, ErrOutOfRange
	}
	if from > to {
		return nil, ErrOutOfRange
	}

	runes := textutil.StrToUTF16(t.content.Text)
	if t.content.Marks == nil {
		t.content.Marks = &model.BlockContentTextMarks{}
	}

	newMarks := &model.BlockContentTextMarks{}
	oldMarks := &model.BlockContentTextMarks{}
	r := model.Range{From: from, To: to}
	oldMarks.Marks, newMarks.Marks = t.splitMarks(t.content.Marks.Marks, &r, 0)
	oldMarks.Marks = t.normalizeMarksPure(oldMarks.Marks)
	newMarks.Marks = t.normalizeMarksPure(newMarks.Marks)

	for _, m := range newMarks.Marks {
		m.Range.From = m.Range.From - r.From
		m.Range.To = m.Range.To - r.From
	}

	tBackgroundColor := t.BackgroundColor
	if t.content.Style == model.BlockContentText_Code {
		tBackgroundColor = ""
	}
	if top {
		newBlock = simple.New(&model.Block{
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text:  textutil.UTF16ToStr(runes[:from]),
				Style: t.content.Style,
				Marks: oldMarks,
				Color: t.content.Color,
			}},
			BackgroundColor: tBackgroundColor,
			Align:           t.Align,
		})

		t.content.Text = textutil.UTF16ToStr(runes[to:])
		t.content.Marks = newMarks
		t.content.Checked = false
	} else {
		newBlock = simple.New(&model.Block{
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text:    textutil.UTF16ToStr(runes[to:]),
				Style:   t.content.Style,
				Marks:   newMarks,
				Checked: false,
				Color:   t.content.Color,
			}},
			BackgroundColor: tBackgroundColor,
			Align:           t.Align,
		})
		t.content.Text = textutil.UTF16ToStr(runes[:from])
		t.content.Marks = oldMarks
	}
	return newBlock, nil
}

func (t *Text) splitMarks(marks []*model.BlockContentTextMark, r *model.Range, newTextLen int32) (topMarks []*model.BlockContentTextMark, botMarks []*model.BlockContentTextMark) {
	for i := 0; i < len(marks); i++ {
		m := marks[i]

		// <b>lorem</b> lorem (**********)  :--->   <b>lorem</b> lorem __PASTE__
		if (m.Range.From < r.From) && (m.Range.To <= r.From) {
			topMarks = append(topMarks, &model.BlockContentTextMark{
				Range: &model.Range{
					From: m.Range.From,
					To:   m.Range.To,
				},
				Type:  m.Type,
				Param: m.Param,
			})
		} else

		// <b>lorem lorem(******</b>******)  :--->   <b>lorem lorem</b> __PASTE__
		if (m.Range.From < r.From) && (m.Range.To > r.From) && (m.Range.To < r.To) {
			topMarks = append(topMarks, &model.BlockContentTextMark{
				Range: &model.Range{
					From: m.Range.From,
					To:   r.From,
				},
				Type:  m.Type,
				Param: m.Param,
			})

		} else

		// (**<b>******</b>******)  :--->     __PASTE__
		if (m.Range.From >= r.From) && (m.Range.To <= r.To) {
			continue
		} else

		// <b>lorem (*********) lorem</b>  :--->   <b>lorem</b> __PASTE__ <b>lorem</b>
		if (m.Range.From < r.From) && (m.Range.To > r.To) {
			topMarks = append(topMarks, &model.BlockContentTextMark{
				Range: &model.Range{
					From: m.Range.From,
					To:   r.From,
				},
				Type:  m.Type,
				Param: m.Param,
			})

			botMarks = append(botMarks, &model.BlockContentTextMark{
				Range: &model.Range{
					From: r.From + newTextLen,
					To:   m.Range.To - (r.To - r.From) + newTextLen,
				},
				Type:  m.Type,
				Param: m.Param,
			})

		} else
		// (*********) <b>lorem lorem</b>  :--->   __PASTE__ <b>lorem lorem</b>
		if m.Range.From >= r.To {
			botMarks = append(botMarks, &model.BlockContentTextMark{
				Range: &model.Range{
					From: m.Range.From - (r.To - r.From) + newTextLen,
					To:   m.Range.To - (r.To - r.From) + newTextLen,
				},
				Type:  m.Type,
				Param: m.Param,
			})
		} else if (m.Range.From >= r.From) && (m.Range.To >= r.To) {
			botMarks = append(botMarks, &model.BlockContentTextMark{
				Range: &model.Range{
					From: r.From + newTextLen,
					To:   m.Range.To - (r.To - r.From) + newTextLen,
				},
				Type:  m.Type,
				Param: m.Param,
			})
		} else if (m.Range.From < r.From) && (m.Range.To > r.From) && (m.Range.To <= r.To) {
			topMarks = append(topMarks, &model.BlockContentTextMark{
				Range: &model.Range{
					From: m.Range.From,
					To:   r.From,
				},
				Type:  m.Type,
				Param: m.Param,
			})
		} else
		//  (*******<b>**)rem lorem</b>  :--->   __PASTE__ <b>em lorem</b>
		if m.Range.From < r.To {
			topMarks = append(topMarks, &model.BlockContentTextMark{
				Range: &model.Range{
					From: r.From + newTextLen,
					To:   m.Range.To - (r.To - r.From) + newTextLen,
				},
				Type:  m.Type,
				Param: m.Param,
			})
		}
	}
	return topMarks, botMarks
}

func (t *Text) SplitMarks(textRange *model.Range, newMarks []*model.BlockContentTextMark, newText string) (combinedMarks []*model.BlockContentTextMark) {
	addLen := int32(textutil.UTF16RuneCountString(newText))

	leftMarks, rightMarks := t.splitMarks(t.content.Marks.Marks, textRange, addLen)

	for _, mark := range newMarks {
		mark.Range.From = mark.Range.From + textRange.From
		mark.Range.To = mark.Range.To + textRange.From

		combinedMarks = append(combinedMarks, mark)
	}

	for _, mark := range leftMarks {
		combinedMarks = append(combinedMarks, mark)
	}

	for _, mark := range rightMarks {
		combinedMarks = append(combinedMarks, mark)
	}

	return combinedMarks
}

type mergeOpts struct {
	withStyle bool
	style     model.BlockContentTextStyle
}

type MergeOption func(opts *mergeOpts)

func WithForcedStyle(style model.BlockContentTextStyle) MergeOption {
	return func(opts *mergeOpts) {
		opts.withStyle = true
		opts.style = style
	}
}

func (t *Text) Merge(b simple.Block, opts ...MergeOption) error {
	o := mergeOpts{}
	for _, apply := range opts {
		apply(&o)
	}

	text, ok := b.(*Text)
	if !ok {
		return fmt.Errorf("unexpected block type for merge: %T", b)
	}

	style := text.content.Style
	if o.withStyle {
		style = o.style
	}
	t.SetStyle(style)
	t.BackgroundColor = text.BackgroundColor

	curLen := int32(textutil.UTF16RuneCountString(t.content.Text))
	t.content.Text += text.content.Text
	for _, m := range text.content.Marks.Marks {
		t.content.Marks.Marks = append(t.content.Marks.Marks, &model.BlockContentTextMark{
			Range: &model.Range{
				From: m.Range.From + curLen,
				To:   m.Range.To + curLen,
			},
			Type:  m.Type,
			Param: m.Param,
		})
	}
	t.normalizeMarks()
	return nil
}

func (t *Text) normalizeMarks() {
	t.content.Marks.Marks = t.normalizeMarksPure(t.content.Marks.Marks)
}

func (t *Text) normalizeMarksPure(marks []*model.BlockContentTextMark) (outputMarks []*model.BlockContentTextMark) {
	outputMarks = marks
	sort.Sort(sortedMarks(outputMarks))
	for i := 0; i < len(outputMarks); i++ {
		if i+1 == len(outputMarks) {
			break
		}
		m := outputMarks[i]
		sm := outputMarks[i+1]
		if m.Type == sm.Type && m.Param == sm.Param && m.Range.To >= sm.Range.From {
			m.Range.To = sm.Range.To
			outputMarks[i+1] = nil
			outputMarks = append(outputMarks[:i+1], outputMarks[i+2:]...)
			i = -1
		}
	}

	return outputMarks
}

func (t *Text) String() string {
	return fmt.Sprintf("%s: text:%s", t.Id, t.content.Style.String())
}

func (t *Text) FillSmartIds(ids []string) []string {
	if t.content.Marks != nil {
		for _, m := range t.content.Marks.Marks {
			if (m.Type == model.BlockContentTextMark_Mention ||
				m.Type == model.BlockContentTextMark_Object) && m.Param != "" {
				ids = append(ids, m.Param)
			}
		}
	}
	return ids
}

func (t *Text) ReplaceLinkIds(replacer func(oldId string) (newId string)) {
	if t.content.Marks != nil {
		for _, m := range t.content.Marks.Marks {
			if (m.Type == model.BlockContentTextMark_Mention ||
				m.Type == model.BlockContentTextMark_Object) && m.Param != "" {

				m.Param = replacer(m.Param)
			}
		}
	}
	return
}

func (t *Text) HasSmartIds() bool {
	if t.content.Marks != nil {
		for _, m := range t.content.Marks.Marks {
			if m.Type == model.BlockContentTextMark_Mention || m.Type == model.BlockContentTextMark_Object {
				return true
			}
		}
	}
	return false
}

func (t *Text) ApplyEvent(e *pb.EventBlockSetText) error {
	if e.Style != nil {
		t.content.Style = e.Style.GetValue()
	}
	if e.Text != nil {
		t.content.Text = e.Text.GetValue()
	}
	if e.Marks != nil {
		t.content.Marks = e.Marks.GetValue()
	}
	if e.Checked != nil {
		t.content.Checked = e.Checked.GetValue()
	}
	if e.Color != nil {
		t.content.Color = e.Color.GetValue()
	}
	if e.IconImage != nil {
		t.content.IconImage = e.IconImage.GetValue()
	}
	if e.IconEmoji != nil {
		t.content.IconEmoji = e.IconEmoji.GetValue()
	}
	return nil
}

func (t *Text) IsEmpty() bool {
	if t.content.Text == "" &&
		!t.content.Checked &&
		t.content.Color == "" &&
		t.content.Style == 0 &&
		t.content.IconEmoji == "" &&
		t.content.IconImage == "" &&
		len(t.content.GetMarks().GetMarks()) == 0 &&
		t.Model().BackgroundColor == "" &&
		t.Model().Align == 0 &&
		t.Model().VerticalAlign == 0 {
		return true
	}
	return false
}

func isIncompatibleType(firstType, secondType model.BlockContentTextMarkType) bool {
	if (firstType == model.BlockContentTextMark_Link && secondType == model.BlockContentTextMark_Object) ||
		(secondType == model.BlockContentTextMark_Link && firstType == model.BlockContentTextMark_Object) {
		return true
	}
	return false
}

func (t *Text) CanInheritChildrenOnReplace() {}
