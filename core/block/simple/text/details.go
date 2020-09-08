package text

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const DetailsKeyFieldName = "_detailsKey"

func NewDetails(block *model.Block, key string) simple.Block {
	t := NewText(block)
	if t == nil {
		return nil
	}
	return &textDetails{
		Text: t.(*Text),
		key:  key,
	}
}

type DetailsBlock interface {
	simple.DetailsHandler
	Block
}

type textDetails struct {
	*Text
	key     string
	changed bool
	text    string
}

func (td *textDetails) DetailsInit(s simple.DetailsService) {
	td.Text.SetText(pbtypes.GetString(s.Details(), td.key), nil)
	return
}

func (td *textDetails) OnDetailsChange(s simple.DetailsService) (msgs []simple.EventMessage, err error) {
	newValue := pbtypes.GetString(s.Details(), td.key)
	if old := td.GetText(); old != newValue {
		td.Text.SetText(newValue, nil)
		msgs = append(msgs, simple.EventMessage{
			Msg: &pb.EventMessage{
				Value: &pb.EventMessageValueOfBlockSetText{
					BlockSetText: &pb.EventBlockSetText{
						Id: td.Id,
						Text: &pb.EventBlockSetTextText{
							Value: newValue,
						},
					},
				},
			},
			Virtual: true,
		})
	}
	return
}

func (td *textDetails) DetailsApply(s simple.DetailsService) (msgs []simple.EventMessage, err error) {
	if !td.changed {
		return
	}
	value := pbtypes.String(td.GetText())
	s.SetDetail(td.key, value)
	msgs = append(msgs, simple.EventMessage{
		Msg: &pb.EventMessage{
			Value: &pb.EventMessageValueOfBlockSetText{
				BlockSetText: &pb.EventBlockSetText{
					Id: td.Id,
					Text: &pb.EventBlockSetTextText{
						Value: value.GetStringValue(),
					},
				},
			},
		},
		Virtual: true,
	})
	td.changed = false
	return
}

func (td *textDetails) Copy() simple.Block {
	return &textDetails{
		Text:    td.Text.Copy().(*Text),
		key:     td.key,
		changed: td.changed,
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
	for _, msg := range msgs {
		if st := msg.Msg.GetBlockSetText(); st != nil {
			if st.Text != nil {
				st.Text = nil
			}
		}
	}
	return
}

func (td *textDetails) SetText(text string, _ *model.BlockContentTextMarks) (err error) {
	td.changed = text != td.GetText()
	return td.Text.SetText(text, nil)
}

func (td *textDetails) ModelToSave() *model.Block {
	b := pbtypes.CopyBlock(td.Model())
	b.Content.(*model.BlockContentOfText).Text.Text = ""
	return b
}
