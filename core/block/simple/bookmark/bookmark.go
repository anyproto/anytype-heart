package bookmark

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"github.com/mohae/deepcopy"
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
	Fetch(params FetchParams) (err error)
	SetLinkPreview(data model.LinkPreview)
	SetImageHash(hash string)
	SetFaviconHash(hash string)
}

type FetchParams struct {
	Url         string
	Anytype     anytype.Service
	Updater     Updater
	LinkPreview linkpreview.LinkPreview
}

type Updater func(ids []string, hist bool, apply func(b simple.Block) error) (err error)

type Bookmark struct {
	*base.Base
	content *model.BlockContentBookmark
}

func (f *Bookmark) SetLinkPreview(data model.LinkPreview) {
	f.content.Url = data.Url
	f.content.Title = data.Title
	f.content.Description = data.Description
	f.content.Type = data.Type
}

func (f *Bookmark) SetImageHash(hash string) {
	f.content.ImageHash = hash
}

func (f *Bookmark) SetFaviconHash(hash string) {
	f.content.FaviconHash = hash
}

func (f *Bookmark) Fetch(params FetchParams) (err error) {
	f.content.Url = params.Url
	go fetcher(f.Id, params)
	return
}

func (f *Bookmark) Copy() simple.Block {
	copy := deepcopy.Copy(f.Model()).(*model.Block)
	return &Bookmark{
		Base:    base.NewBase(copy).(*base.Base),
		content: copy.GetBookmark(),
	}
}

func (f *Bookmark) Diff(b simple.Block) (msgs []*pb.EventMessage, err error) {
	bookmark, ok := b.(*Bookmark)
	if ! ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = f.Base.Diff(bookmark); err != nil {
		return
	}
	changes := &pb.EventBlockSetBookmark{
		Id: bookmark.Id,
	}
	hasChanges := false

	if f.content.Type != bookmark.content.Type {
		hasChanges = true
		changes.Type = &pb.EventBlockSetBookmarkType{Value: bookmark.content.Type}
	}
	if f.content.Url != bookmark.content.Url {
		hasChanges = true
		changes.Url = &pb.EventBlockSetBookmarkUrl{Value: bookmark.content.Url}
	}
	if f.content.Title != bookmark.content.Title {
		hasChanges = true
		changes.Title = &pb.EventBlockSetBookmarkTitle{Value: bookmark.content.Title}
	}
	if f.content.Description != bookmark.content.Description {
		hasChanges = true
		changes.Description = &pb.EventBlockSetBookmarkDescription{Value: bookmark.content.Description}
	}
	if f.content.ImageHash != bookmark.content.ImageHash {
		hasChanges = true
		changes.ImageHash = &pb.EventBlockSetBookmarkImageHash{Value: bookmark.content.ImageHash}
	}
	if f.content.FaviconHash != bookmark.content.FaviconHash {
		hasChanges = true
		changes.FaviconHash = &pb.EventBlockSetBookmarkFaviconHash{Value: bookmark.content.FaviconHash}
	}

	if hasChanges {
		msgs = append(msgs, &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetBookmark{BlockSetBookmark: changes}})
	}
	return
}

func fetcher(id string, params FetchParams) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	data, err := params.LinkPreview.Fetch(ctx, params.Url)
	cancel()
	if err != nil {
		fmt.Println("bookmark: can't fetch link:", params.Url, err)
		return
	}

	if data.ImageUrl != "" {
		go func() {
			hash, err := loadImage(params.Anytype, data.ImageUrl)
			if err != nil {
				fmt.Println("can't load image url:", data.ImageUrl, err)
				return
			}
			err = params.Updater([]string{id}, false, func(b simple.Block) error {
				if bm, ok := b.(Block); ok {
					bm.SetImageHash(hash)
					return nil
				}
				return fmt.Errorf("unexpected block type (want bookmark, have %T)", b)
			})
			if err != nil {
				fmt.Println("can't set image hash:", id, err)
				return
			}
		}()
	}
	if data.FaviconUrl != "" {
		go func() {
			hash, err := loadImage(params.Anytype, data.FaviconUrl)
			if err != nil {
				fmt.Println("can't load favicon url:", data.FaviconUrl, err)
				return
			}
			err = params.Updater([]string{id}, false, func(b simple.Block) error {
				if bm, ok := b.(Block); ok {
					bm.SetFaviconHash(hash)
					return nil
				}
				return fmt.Errorf("unexpected block type (want bookmark, have %T)", b)
			})
			if err != nil {
				fmt.Println("can't set favicon hash:", id, err)
				return
			}
		}()
	}

	err = params.Updater([]string{id}, false, func(b simple.Block) error {
		if bm, ok := b.(Block); ok {
			bm.SetLinkPreview(data)
			return nil
		}
		return fmt.Errorf("unexpected block type (want bookmark, have %T)", b)
	})
	if err != nil {
		fmt.Println("can't set linkpreview data:", id, err)
		return
	}
}

func loadImage(stor anytype.Service, url string) (hash string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("can't download '%s': %s", url, resp.Status)
	}

	im, err := stor.ImageAddWithReader(resp.Body, filepath.Base(url))
	if err != nil {
		return
	}
	return im.Hash(), nil
}
