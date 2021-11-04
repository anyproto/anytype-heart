package text

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const DetailsKeyFieldName = "_detailsKey"

func newDetailKeys(keysList []string) DetailsKeys {
	keys := DetailsKeys{}
	if len(keysList) > 0 {
		keys.Text = keysList[0]
		if len(keysList) > 1 {
			keys.Checked = keysList[1]
		}
	}
	return keys
}

type DetailsKeys struct {
	Text    string
	Checked string
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
	td.keys = newDetailKeys(pbtypes.GetStringList(td.Fields, DetailsKeyFieldName))
	if td.keys.Text != "" {
		td.SetText(pbtypes.GetString(s.Details(), td.keys.Text), nil)
	}
	if td.keys.Checked != "" {
		checked := pbtypes.GetBool(s.Details(), td.keys.Checked)
		td.SetChecked(checked)
	}
	return
}

func (td *textDetails) ApplyToDetails(prevBlock simple.Block, s simple.DetailsService) (ok bool, err error) {
	var prev Block
	prev, _ = prevBlock.(Block)

	if td.keys.Text != "" {
		if prev == nil || prev.GetText() != td.GetText() {
			s.SetDetail(td.keys.Text, pbtypes.String(td.GetText()))
			ok = true
		}
	}
	if td.keys.Checked != "" {
		if prev == nil && td.GetChecked() || prev != nil && prev.GetChecked() != td.GetChecked() {
			s.SetDetail(td.keys.Checked, pbtypes.Bool(td.GetChecked()))
			ok = true
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
	var virtEvent = simple.EventMessage{
		Virtual: true,
		Msg: &pb.EventMessage{
			Value: &pb.EventMessageValueOfBlockSetText{
				BlockSetText: &pb.EventBlockSetText{
					Id: td.Id,
				},
			},
		},
	}
	var (
		toRemove   = -1
		virtActive bool
	)
	for i, msg := range msgs {
		if td.keys.Text != "" {
			if st := msg.Msg.GetBlockSetText(); st != nil {
				if st.Text != nil {
					virtEvent.Msg.GetBlockSetText().Text = st.Text
					st.Text = nil
					virtActive = true
				}
			}
		}
		if td.keys.Checked != "" {
			if st := msg.Msg.GetBlockSetText(); st != nil {
				if st.Checked != nil {
					virtEvent.Msg.GetBlockSetText().Checked = st.Checked
					st.Checked = nil
					virtActive = true
				}
			}
		}
		if st := msg.Msg.GetBlockSetText(); st != nil {
			if st.Text == nil && st.Checked == nil && st.Marks == nil && st.Style == nil && st.Color == nil {
				toRemove = i
			}
		}
	}
	if toRemove != -1 {
		if virtActive {
			msgs[toRemove] = virtEvent
		} else {
			copy(msgs[toRemove:], msgs[toRemove+1:])
			msgs[len(msgs)-1].Msg = nil
			msgs = msgs[:len(msgs)-1]
		}
		return
	}
	if virtActive {
		msgs = append(msgs, virtEvent)
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
		td.Text.SetStyle(style)
	}
}

func (td *textDetails) RangeCut(from int32, to int32) (cutBlock *model.Block, initialBlock *model.Block, err error) {
	if td.keys.Text == "" {
		return td.Text.RangeCut(from, to)
	}
	if cutBlock, initialBlock, err = td.Text.RangeCut(from, to); err != nil {
		return nil, nil, err
	}
	if pbtypes.Exists(cutBlock.GetFields(), DetailsKeyFieldName) {
		delete(cutBlock.GetFields().Fields, DetailsKeyFieldName)
	}
	cutBlock.GetText().Style = model.BlockContentText_Paragraph
	return
}
