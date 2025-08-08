package bookmark

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func init() {
	simple.RegisterCreator(NewBookmark)
}

type ObjectContent struct {
	BookmarkContent *model.BlockContentBookmark
	Blocks          []*model.Block
}

func NewBookmark(m *model.Block) simple.Block {
	if bookmark := m.GetBookmark(); bookmark != nil {
		return &Bookmark{
			Base:    base.NewBase(m).(*base.Base),
			content: bookmark,
		}
	}
	return nil
}

type Block interface {
	simple.Block
	simple.FileHashes
	GetContent() *model.BlockContentBookmark
	ToDetails(origin objectorigin.ObjectOrigin) *domain.Details
	SetState(s model.BlockContentBookmarkState)
	UpdateContent(func(content *ObjectContent))
	ApplyEvent(e *pb.EventBlockSetBookmark) (err error)
}

type Bookmark struct {
	*base.Base
	content *model.BlockContentBookmark
}

func (b *Bookmark) GetContent() *model.BlockContentBookmark {
	return b.content
}

func (b *Bookmark) ToDetails(origin objectorigin.ObjectOrigin) *domain.Details {
	details := domain.NewDetails()
	details.SetString(bundle.RelationKeySource, b.content.Url)
	origin.AddToDetails(details)
	return details
}

func (b *Bookmark) UpdateContent(updater func(content *ObjectContent)) {
	updater(&ObjectContent{BookmarkContent: b.content})
}

func (b *Bookmark) SetState(s model.BlockContentBookmarkState) {
	b.content.State = s
}

var _ Block = &Bookmark{}

type FetchParams struct {
	Url     string
	Updater Updater
}

type Updater func(blockID string, apply func(b Block) error) (err error)

func (b *Bookmark) Copy() simple.Block {
	copy := pbtypes.CopyBlock(b.Model())
	return &Bookmark{
		Base:    base.NewBase(copy).(*base.Base),
		content: copy.GetBookmark(),
	}
}

// Validate TODO: add validation rules
func (b *Bookmark) Validate() error {
	return nil
}

func (b *Bookmark) Diff(spaceId string, other simple.Block) (msgs []simple.EventMessage, err error) {
	bookmark, ok := other.(*Bookmark)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = b.Base.Diff(spaceId, bookmark); err != nil {
		return
	}
	changes := &pb.EventBlockSetBookmark{
		Id: bookmark.Id,
	}
	hasChanges := false

	if b.content.Type != bookmark.content.Type {
		hasChanges = true
		changes.Type = &pb.EventBlockSetBookmarkType{Value: bookmark.content.Type}
	}
	if b.content.Url != bookmark.content.Url {
		hasChanges = true
		changes.Url = &pb.EventBlockSetBookmarkUrl{Value: bookmark.content.Url}
	}
	if b.content.Title != bookmark.content.Title {
		hasChanges = true
		changes.Title = &pb.EventBlockSetBookmarkTitle{Value: bookmark.content.Title}
	}
	if b.content.Description != bookmark.content.Description {
		hasChanges = true
		changes.Description = &pb.EventBlockSetBookmarkDescription{Value: bookmark.content.Description}
	}
	if b.content.ImageHash != bookmark.content.ImageHash {
		hasChanges = true
		changes.ImageHash = &pb.EventBlockSetBookmarkImageHash{Value: bookmark.content.ImageHash}
	}
	if b.content.FaviconHash != bookmark.content.FaviconHash {
		hasChanges = true
		changes.FaviconHash = &pb.EventBlockSetBookmarkFaviconHash{Value: bookmark.content.FaviconHash}
	}
	if b.content.TargetObjectId != bookmark.content.TargetObjectId {
		hasChanges = true
		changes.TargetObjectId = &pb.EventBlockSetBookmarkTargetObjectId{Value: bookmark.content.TargetObjectId}
	}
	if b.content.State != bookmark.content.State {
		hasChanges = true
		changes.State = &pb.EventBlockSetBookmarkState{Value: bookmark.content.State}
	}

	if hasChanges {
		msgs = append(msgs, simple.EventMessage{Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockSetBookmark{BlockSetBookmark: changes})})
	}
	return
}

func (b *Bookmark) ApplyEvent(e *pb.EventBlockSetBookmark) (err error) {
	if e.Type != nil {
		b.content.Type = e.Type.GetValue()
	}
	if e.Description != nil {
		b.content.Description = e.Description.GetValue()
	}
	if e.FaviconHash != nil {
		b.content.FaviconHash = e.FaviconHash.GetValue()
	}
	if e.ImageHash != nil {
		b.content.ImageHash = e.ImageHash.GetValue()
	}
	if e.Title != nil {
		b.content.Title = e.Title.GetValue()
	}
	if e.Url != nil {
		b.content.Url = e.Url.GetValue()
	}
	if e.TargetObjectId != nil {
		b.content.TargetObjectId = e.TargetObjectId.GetValue()
	}
	if e.State != nil {
		b.content.State = e.State.GetValue()
	}

	return
}

func (b *Bookmark) IterateLinkedFiles(iter func(id string)) {
	if b.content.ImageHash != "" {
		iter(b.content.ImageHash)
	}
	if b.content.FaviconHash != "" {
		iter(b.content.FaviconHash)
	}
}

func (b *Bookmark) FillFileHashes(hashes []string) []string {
	if b.content.ImageHash != "" {
		hashes = append(hashes, b.content.ImageHash)
	}
	if b.content.FaviconHash != "" {
		hashes = append(hashes, b.content.FaviconHash)
	}
	return hashes
}

func (l *Bookmark) ReplaceLinkIds(replacer func(oldId string) (newId string)) {
	if l.content.TargetObjectId != "" {
		l.content.TargetObjectId = replacer(l.content.TargetObjectId)
	}
	return
}

func (b *Bookmark) FillSmartIds(ids []string) []string {
	if b.content.TargetObjectId != "" {
		ids = append(ids, b.content.TargetObjectId)
	}
	return ids
}

func (b *Bookmark) HasSmartIds() bool {
	return b.content.TargetObjectId != ""
}

func (l *Bookmark) IsEmpty() bool {
	return l.content.TargetObjectId == ""
}
