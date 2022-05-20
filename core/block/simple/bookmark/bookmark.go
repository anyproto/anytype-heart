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
			Content: (*Content)(bookmark),
		}
	}
	return nil
}

type BlockContent interface {
	GetContent() *model.BlockContentBookmark
	SetLinkPreview(data model.LinkPreview)
	SetImageHash(hash string)
	SetFaviconHash(hash string)
	SetTargetObjectId(pageId string)
}

type Block interface {
	simple.Block
	simple.FileHashes
	BlockContent
	ApplyEvent(e *pb.EventBlockSetBookmark) (err error)
}

type Bookmark struct {
	*base.Base
	*Content
}

var _ Block = &Bookmark{}

type Content model.BlockContentBookmark

func (f *Content) GetContent() *model.BlockContentBookmark {
	return (*model.BlockContentBookmark)(f)
}

func (f *Content) SetLinkPreview(data model.LinkPreview) {
	// TODO: don't reset url
	f.Url = data.Url
	f.Title = data.Title
	f.Description = data.Description
	f.Type = data.Type
}

func (f *Content) SetImageHash(hash string) {
	f.ImageHash = hash
}

func (f *Content) SetFaviconHash(hash string) {
	f.FaviconHash = hash
}

func (f *Content) SetTargetObjectId(pageId string) {
	f.TargetObjectId = pageId
}

func (b *Bookmark) Copy() simple.Block {
	copy := pbtypes.CopyBlock(b.Model())
	return &Bookmark{
		Base:    base.NewBase(copy).(*base.Base),
		Content: (*Content)(copy.GetBookmark()),
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

	if b.Content.Type != bookmark.Content.Type {
		hasChanges = true
		changes.Type = &pb.EventBlockSetBookmarkType{Value: bookmark.Content.Type}
	}
	if b.Content.Url != bookmark.Content.Url {
		hasChanges = true
		changes.Url = &pb.EventBlockSetBookmarkUrl{Value: bookmark.Content.Url}
	}
	if b.Content.Title != bookmark.Content.Title {
		hasChanges = true
		changes.Title = &pb.EventBlockSetBookmarkTitle{Value: bookmark.Content.Title}
	}
	if b.Content.Description != bookmark.Content.Description {
		hasChanges = true
		changes.Description = &pb.EventBlockSetBookmarkDescription{Value: bookmark.Content.Description}
	}
	if b.Content.ImageHash != bookmark.Content.ImageHash {
		hasChanges = true
		changes.ImageHash = &pb.EventBlockSetBookmarkImageHash{Value: bookmark.Content.ImageHash}
	}
	if b.Content.FaviconHash != bookmark.Content.FaviconHash {
		hasChanges = true
		changes.FaviconHash = &pb.EventBlockSetBookmarkFaviconHash{Value: bookmark.Content.FaviconHash}
	}
	if b.Content.TargetObjectId != bookmark.Content.TargetObjectId {
		hasChanges = true
		changes.TargetObjectId = &pb.EventBlockSetBookmarkTargetObjectId{Value: bookmark.Content.TargetObjectId}
	}

	if hasChanges {
		msgs = append(msgs, simple.EventMessage{Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetBookmark{BlockSetBookmark: changes}}})
	}
	return
}

func (b *Bookmark) ApplyEvent(e *pb.EventBlockSetBookmark) (err error) {
	if e.Type != nil {
		b.Content.Type = e.Type.GetValue()
	}
	if e.Description != nil {
		b.Content.Description = e.Description.GetValue()
	}
	if e.FaviconHash != nil {
		b.Content.FaviconHash = e.FaviconHash.GetValue()
	}
	if e.ImageHash != nil {
		b.Content.ImageHash = e.ImageHash.GetValue()
	}
	if e.Title != nil {
		b.Content.Title = e.Title.GetValue()
	}
	if e.Url != nil {
		b.Content.Url = e.Url.GetValue()
	}
	if e.TargetObjectId != nil {
		b.Content.TargetObjectId = e.TargetObjectId.GetValue()
	}

	return
}

func (b *Bookmark) FillFileHashes(hashes []string) []string {
	if b.Content.ImageHash != "" {
		hashes = append(hashes, b.Content.ImageHash)
	}
	if b.Content.FaviconHash != "" {
		hashes = append(hashes, b.Content.FaviconHash)
	}
	return hashes
}
