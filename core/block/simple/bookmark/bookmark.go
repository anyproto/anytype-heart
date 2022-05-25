package bookmark

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func init() {
	simple.RegisterCreator(NewBookmark)
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
	UpdateContent(func(content *model.BlockContentBookmark))
	ApplyEvent(e *pb.EventBlockSetBookmark) (err error)
}

type Bookmark struct {
	*base.Base
	content *model.BlockContentBookmark
}

func (b *Bookmark) GetContent() *model.BlockContentBookmark {
	return b.content
}

func (b *Bookmark) UpdateContent(updater func(bookmark *model.BlockContentBookmark)) {
	updater(b.content)
}

var _ Block = &Bookmark{}

type FetchParams struct {
	Url     string
	Updater Updater
	Sync    bool
}

type Updater func(id string, apply func(b Block) error) (err error)

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

func (b *Bookmark) Diff(other simple.Block) (msgs []simple.EventMessage, err error) {
	bookmark, ok := other.(*Bookmark)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = b.Base.Diff(bookmark); err != nil {
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

	if hasChanges {
		msgs = append(msgs, simple.EventMessage{Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetBookmark{BlockSetBookmark: changes}}})
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

	return
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

func (b *Bookmark) FillSmartIds(ids []string) []string {
	if b.content.TargetObjectId != "" {
		ids = append(ids, b.content.TargetObjectId)
	}
	return ids
}

func (b *Bookmark) HasSmartIds() bool {
	return b.content.TargetObjectId != ""
}
