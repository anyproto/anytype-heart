package link

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
	"github.com/anyproto/anytype-heart/util/text"
)

func init() {
	simple.RegisterCreator(NewLink)
}

func NewLink(m *model.Block) simple.Block {
	if link := m.GetLink(); link != nil {
		return &Link{
			Base:    base.NewBase(m).(*base.Base),
			content: link,
		}
	}
	return nil
}

type Block interface {
	simple.Block
	FillSmartIds(ids []string) []string
	HasSmartIds() bool
	ApplyEvent(e *pb.EventBlockSetLink) error
	ToText(targetDetails *domain.Details) simple.Block
	SetAppearance(content *model.BlockContentLink) error
}

type Link struct {
	*base.Base
	content *model.BlockContentLink
}

func (l *Link) Copy() simple.Block {
	copy := pbtypes.CopyBlock(l.Model())
	return &Link{
		Base:    base.NewBase(copy).(*base.Base),
		content: copy.GetLink(),
	}
}

func (l *Link) Validate() error {
	if l.content.TargetBlockId == "" {
		return fmt.Errorf("targetBlockId is empty")
	}
	return nil
}

func (l *Link) SetAppearance(content *model.BlockContentLink) error {
	l.content.IconSize = content.IconSize
	l.content.CardStyle = content.CardStyle
	l.content.Description = content.Description
	l.content.Relations = content.Relations
	return nil
}

func (l *Link) Diff(spaceId string, b simple.Block) (msgs []simple.EventMessage, err error) {
	link, ok := b.(*Link)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = l.Base.Diff(spaceId, link); err != nil {
		return
	}
	changes := &pb.EventBlockSetLink{
		Id: link.Id,
	}
	hasChanges := false

	if l.content.Style != link.content.Style {
		hasChanges = true
		changes.Style = &pb.EventBlockSetLinkStyle{Value: link.content.Style}
	}
	if l.content.TargetBlockId != link.content.TargetBlockId {
		hasChanges = true
		changes.TargetBlockId = &pb.EventBlockSetLinkTargetBlockId{Value: link.content.TargetBlockId}
	}

	if l.content.IconSize != link.content.IconSize {
		hasChanges = true
		changes.IconSize = &pb.EventBlockSetLinkIconSize{Value: link.content.IconSize}
	}

	if l.content.CardStyle != link.content.CardStyle {
		hasChanges = true
		changes.CardStyle = &pb.EventBlockSetLinkCardStyle{Value: link.content.CardStyle}
	}

	if l.content.Description != link.content.Description {
		hasChanges = true
		changes.Description = &pb.EventBlockSetLinkDescription{Value: link.content.Description}
	}

	if !slice.SortedEquals(l.content.Relations, link.content.Relations) {
		hasChanges = true
		changes.Relations = &pb.EventBlockSetLinkRelations{Value: link.content.Relations}
	}

	if hasChanges {
		msgs = append(msgs, simple.EventMessage{Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockSetLink{BlockSetLink: changes})})
	}
	return
}

func (l *Link) ReplaceLinkIds(replacer func(oldId string) (newId string)) {
	if l.content.TargetBlockId != "" {
		l.content.TargetBlockId = replacer(l.content.TargetBlockId)
	}
	return
}

func (l *Link) FillSmartIds(ids []string) []string {
	if l.content.TargetBlockId != "" {
		ids = append(ids, l.content.TargetBlockId)
	}
	return ids
}

func (l *Link) HasSmartIds() bool {
	return l.content.TargetBlockId != ""
}

func (l *Link) ApplyEvent(e *pb.EventBlockSetLink) error {
	if e.Style != nil {
		l.content.Style = e.Style.GetValue()
	}
	if e.TargetBlockId != nil {
		l.content.TargetBlockId = e.TargetBlockId.GetValue()
	}

	if e.IconSize != nil {
		l.content.IconSize = e.IconSize.GetValue()
	}

	if e.CardStyle != nil {
		l.content.CardStyle = e.CardStyle.GetValue()
	}

	if e.Description != nil {
		l.content.Description = e.Description.GetValue()
	}

	if e.Relations != nil {
		l.content.Relations = e.Relations.GetValue()
	}

	return nil
}

func (l *Link) ToText(targetDetails *domain.Details) simple.Block {
	tb := &model.BlockContentText{}
	if l.content.TargetBlockId != "" {
		name := targetDetails.GetString(bundle.RelationKeyName)
		if name == "" {
			name = "Untitled"
		}
		tb.Text = name
		tb.Marks = &model.BlockContentTextMarks{
			Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{0, int32(text.UTF16RuneCountString(name))},
					Type:  model.BlockContentTextMark_Mention,
					Param: l.content.TargetBlockId,
				},
			},
		}
	}
	m := pbtypes.CopyBlock(l.Model())
	m.Id = ""
	m.Content = &model.BlockContentOfText{
		Text: tb,
	}
	return simple.New(m)
}

func (l *Link) IsEmpty() bool {
	return l.content.TargetBlockId == ""
}
