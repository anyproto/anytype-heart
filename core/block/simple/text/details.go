package text

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const DetailsKeyFieldName = "_detailsKey"

type DetailsKeys struct {
	Text    string
	Checked string
	Align   string
}

func NewDetails(block *model.Block, keys DetailsKeys) simple.Block {
	t := NewText(block)
	if t == nil {
		return nil
	}
	return &textDetails{
		Text: t.(*Text),
		keys: keys,
	}
}

type DetailsBlock interface {
	simple.DetailsHandler
	Block
}

type textDetails struct {
	*Text
	keys DetailsKeys
}

func (td *textDetails) DetailsInit(s simple.DetailsService) {
	if td.keys.Text != "" {
		td.Text.SetText(pbtypes.GetString(s.Details(), td.keys.Text), nil)
	}
	if td.keys.Checked != "" {
		td.Text.SetChecked(pbtypes.GetBool(s.Details(), td.keys.Checked))
	}
	if td.keys.Align != "" {
		td.Align = model.BlockAlign(pbtypes.GetInt64(s.Details(), td.keys.Align))
	}
	return
}

func (td *textDetails) OnDetailsChange(prevBlock simple.Block, s simple.DetailsService) (msgs []simple.EventMessage, err error) {
	var prev Block
	prev, _ = prevBlock.(Block)
	setTextEvent := &pb.EventBlockSetText{
		Id: td.Id,
	}
	if td.keys.Text != "" {
		if err = td.onDetailsChangeText(prev, pbtypes.GetString(s.Details(), td.keys.Text), setTextEvent); err != nil {
			return
		}
	}
	if td.keys.Checked != "" {
		if err = td.onDetailsChangeChecked(prev, pbtypes.GetBool(s.Details(), td.keys.Checked), setTextEvent); err != nil {
			return
		}
	}
	if setTextEvent.Text != nil || setTextEvent.Checked != nil {
		msgs = append(msgs, simple.EventMessage{
			Msg: &pb.EventMessage{
				Value: &pb.EventMessageValueOfBlockSetText{
					BlockSetText: setTextEvent,
				},
			},
			Virtual: true,
		})
	}
	if td.keys.Align != "" {
		msgsAlign, e := td.onDetailsChangeAlign(prev, model.BlockAlign(pbtypes.GetInt64(s.Details(), td.keys.Align)))
		if e != nil {
			return nil, e
		}
		if len(msgsAlign) > 0 {
			msgs = append(msgs, msgsAlign...)
		}
	}
	return
}

func (td *textDetails) onDetailsChangeText(prev Block, newValue string, event *pb.EventBlockSetText) (err error) {
	oldValue := ""
	if prev != nil {
		oldValue = prev.GetText()
	}
	if oldValue != newValue {
		td.Text.SetText(newValue, nil)
		event.Text = &pb.EventBlockSetTextText{
			Value: newValue,
		}
	}
	return
}

func (td *textDetails) onDetailsChangeChecked(prev Block, newValue bool, event *pb.EventBlockSetText) (err error) {
	if td.keys.Checked == "" {
		return
	}
	oldValue := false
	if prev != nil {
		oldValue = prev.GetChecked()
	}
	if oldValue != newValue {
		td.Text.SetChecked(newValue)
		event.Checked = &pb.EventBlockSetTextChecked{
			Value: newValue,
		}
	}
	return
}

func (td *textDetails) onDetailsChangeAlign(prev simple.Block, newValue model.BlockAlign) (msgs []simple.EventMessage, err error) {
	if td.keys.Align == "" {
		return
	}
	oldValue := model.BlockAlign(0)
	if prev != nil {
		oldValue = prev.Model().Align
	}
	if oldValue != newValue {
		td.Align = newValue
		msgs = append(msgs, alignEvent(td.Id, newValue))
	}
	return
}

func (td *textDetails) ApplyToDetails(prevBlock simple.Block, s simple.DetailsService) (msgs []simple.EventMessage, err error) {
	var prev Block
	prev, _ = prevBlock.(Block)
	setTextEvent := &pb.EventBlockSetText{
		Id: td.Id,
	}
	if td.keys.Text != "" {
		if err = td.onDetailsChangeText(prev, td.GetText(), setTextEvent); err != nil {
			return
		}
		if setTextEvent.Text != nil {
			s.SetDetail(td.keys.Text, pbtypes.String(setTextEvent.Text.Value))
		}
	}
	if td.keys.Checked != "" {
		if err = td.onDetailsChangeChecked(prev, td.GetChecked(), setTextEvent); err != nil {
			return
		}
		if setTextEvent.Checked != nil {
			s.SetDetail(td.keys.Checked, pbtypes.Bool(setTextEvent.Checked.Value))
		}
	}
	if setTextEvent.Text != nil || setTextEvent.Checked != nil {
		msgs = append(msgs, simple.EventMessage{
			Msg: &pb.EventMessage{
				Value: &pb.EventMessageValueOfBlockSetText{
					BlockSetText: setTextEvent,
				},
			},
			Virtual: true,
		})
	}
	if td.keys.Align != "" {
		msgsAlign, e := td.onDetailsChangeAlign(prev, td.Align)
		if e != nil {
			return nil, e
		}
		if len(msgsAlign) > 0 {
			s.SetDetail(td.keys.Align, pbtypes.Int64(int64(td.Align)))
			msgs = append(msgs, msgsAlign...)
		}
	}
	return
}

func (td *textDetails) Copy() simple.Block {
	return &textDetails{
		Text: td.Text.Copy().(*Text),
		keys: td.keys,
	}
}

func (td *textDetails) Diff(s simple.Block) (msgs []simple.EventMessage, err error) {
	sd, ok := s.(*textDetails)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = td.Text.Diff(sd.Text); err != nil {
		return
	}
	toRemove := -1
	for i, msg := range msgs {
		if td.keys.Text != "" {
			if st := msg.Msg.GetBlockSetText(); st != nil {
				if st.Text != nil {
					st.Text = nil
				}
			}
		}
		if td.keys.Checked != "" {
			if st := msg.Msg.GetBlockSetText(); st != nil {
				if st.Checked != nil {
					st.Checked = nil
				}
			}
		}
		if td.keys.Align != "" {
			if st := msg.Msg.GetBlockSetAlign(); st != nil {
				toRemove = i
			}
		}
	}
	if toRemove != -1 {
		msgs = append(msgs[:toRemove], msgs[toRemove+1:]...)
	}
	return
}

func (td *textDetails) SetText(text string, marks *model.BlockContentTextMarks) (err error) {
	if td.keys.Text != "" {
		marks = nil
	}
	return td.Text.SetText(text, nil)
}

func (td *textDetails) ModelToSave() *model.Block {
	b := pbtypes.CopyBlock(td.Model())
	if td.keys.Text != "" {
		b.Content.(*model.BlockContentOfText).Text.Text = ""
	}
	if td.keys.Checked != "" {
		b.Content.(*model.BlockContentOfText).Text.Checked = false
	}
	if td.keys.Align != "" {
		b.Align = 0
	}
	return b
}

func (td *textDetails) RangeTextPaste(rangeFrom int32, rangeTo int32, copiedBlock *model.Block, isPartOfBlock bool) (caretPosition int32, err error) {
	if caretPosition, err = td.Text.RangeTextPaste(rangeFrom, rangeTo, copiedBlock, isPartOfBlock); err != nil {
		return
	}
	if td.keys.Text != "" {
		td.Text.content.Marks = &model.BlockContentTextMarks{}
	}
	return
}

func (td *textDetails) Merge(b simple.Block) (err error) {
	if err = td.Text.Merge(b); err != nil {
		return
	}
	if td.keys.Text != "" {
		td.Text.content.Marks = &model.BlockContentTextMarks{}
	}
	return
}

func (td *textDetails) SetStyle(style model.BlockContentTextStyle) {
	if td.keys.Text == "" {
		td.SetStyle(style)
	}
}

func (td *textDetails) RangeCut(from int32, to int32) (cutBlock *model.Block, initialBlock *model.Block, err error) {
	if td.keys.Text == "" {
		return td.RangeCut(from, to)
	}
	if cutBlock, initialBlock, err = td.Text.RangeCut(from, to); err != nil {
		return nil, nil, err
	}
	if pbtypes.GetString(cutBlock.GetFields(), DetailsKeyFieldName) != "" {
		delete(cutBlock.GetFields().Fields, DetailsKeyFieldName)
	}
	cutBlock.GetText().Style = model.BlockContentText_Paragraph
	return
}

func alignEvent(id string, value model.BlockAlign) simple.EventMessage {
	return simple.EventMessage{
		Msg: &pb.EventMessage{
			Value: &pb.EventMessageValueOfBlockSetAlign{
				BlockSetAlign: &pb.EventBlockSetAlign{
					Id:    id,
					Align: value,
				},
			},
		},
		Virtual: true,
	}
}
