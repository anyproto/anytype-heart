package bookmark

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FetchParams struct {
	Url         string
	Anytype     core.Service
	Updater     Updater
	LinkPreview linkpreview.LinkPreview
	Sync        bool
}

type Updater func(id string, apply func(b bookmark.Block) error) (err error)

func Fetch(id string, params FetchParams) (err error) {
	// TODO: is it needed?
	// b.Content.Url = params.Url
	if !params.Sync {
		go func() {
			fetcher(id, params)
		}()
	} else {
		fetcher(id, params)
	}
	return
}

func ContentFetcher(url string, linkPreview linkpreview.LinkPreview, svc core.Service) (chan func(blockContent bookmark.BlockContent) error, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	data, err := linkPreview.Fetch(ctx, url)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("bookmark: can't fetch link %s: %w", url, err)
	}

	var wg sync.WaitGroup
	updaters := make(chan func(blockContent bookmark.BlockContent) error)
	wg.Add(1)
	go func() {
		defer wg.Done()
		updaters <- func(bm bookmark.BlockContent) error {
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
			updaters <- func(bm bookmark.BlockContent) error {
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
			updaters <- func(bm bookmark.BlockContent) error {
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
	var upds []func(bm bookmark.BlockContent) error
	for u := range updaters {
		upds = append(upds, u)
	}

	err = params.Updater(id, func(bm bookmark.Block) error {
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
