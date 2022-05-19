package bookmark

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
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
	Fetch(params FetchParams) (err error)
	ApplyEvent(e *pb.EventBlockSetBookmark) (err error)
}

type FetchParams struct {
	Url         string
	Anytype     core.Service
	Updater     Updater
	LinkPreview linkpreview.LinkPreview
	Sync        bool
}

type Updater func(id string, apply func(b Block) error) (err error)

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

func (b *Bookmark) Fetch(params FetchParams) (err error) {
	b.Content.Url = params.Url
	if !params.Sync {
		go func() {
			fetcher(b.Id, params)
		}()
	} else {
		fetcher(b.Id, params)
	}
	return
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

// TODO: move to bookmark service?
func ContentFetcher(url string, linkPreview linkpreview.LinkPreview, svc core.Service) (chan func(blockContent BlockContent) error, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	data, err := linkPreview.Fetch(ctx, url)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("bookmark: can't fetch link %s: %w", url, err)
	}

	var wg sync.WaitGroup
	updaters := make(chan func(blockContent BlockContent) error)
	wg.Add(1)
	go func() {
		defer wg.Done()
		updaters <- func(bm BlockContent) error {
			bm.SetLinkPreview(data)
			return nil
		}
	}()

	if data.ImageUrl != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hash, err := loadImage(svc, data.ImageUrl)
			if err != nil {
				fmt.Println("can't load image url:", data.ImageUrl, err)
				return
			}
			updaters <- func(bm BlockContent) error {
				bm.SetImageHash(hash)
				return nil
			}
		}()
	}
	if data.FaviconUrl != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hash, err := loadImage(svc, data.FaviconUrl)
			if err != nil {
				fmt.Println("can't load favicon url:", data.FaviconUrl, err)
				return
			}
			updaters <- func(bm BlockContent) error {
				bm.SetFaviconHash(hash)
				return nil
			}
		}()
	}

	go func() {
		wg.Wait()
		close(updaters)
	}()

	return updaters, nil
}

func fetcher(id string, params FetchParams) {
	updaters, err := ContentFetcher(params.Url, params.LinkPreview, params.Anytype)
	if err != nil {
		fmt.Println("can't get updates:", id, err)
		return
	}
	var upds []func(bm BlockContent) error
	for u := range updaters {
		upds = append(upds, u)
	}

	err = params.Updater(id, func(bm Block) error {
		for _, u := range upds {
			if err := u(bm); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		fmt.Println("can't update bookmark data:", id, err)
		return
	}
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

func loadImage(stor core.Service, url string) (hash string, err error) {
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

	tmpFile, err := ioutil.TempFile(stor.TempDir(), "anytype_downloaded_file_*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpFile.Name())

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return "", err
	}

	_, err = tmpFile.Seek(0, io.SeekStart)
	if err != nil {
		return "", err
	}

	im, err := stor.ImageAdd(context.TODO(), files.WithReader(tmpFile), files.WithName(filepath.Base(url)))
	if err != nil {
		return
	}
	return im.Hash(), nil
}
