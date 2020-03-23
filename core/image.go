package core

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math"

	"github.com/anytypeio/go-anytype-library/mill"
	"github.com/anytypeio/go-anytype-library/pb/lsmodel"
	"github.com/anytypeio/go-anytype-library/schema"
)

type Image interface {
	Exif() (*mill.ImageExifSchema, error)
	Hash() string
	GetFileForWidth(ctx context.Context, wantWidth int) (File, error)
	GetFileForLargestWidth(ctx context.Context) (File, error)
}

type image struct {
	hash            string // directory hash
	variantsByWidth map[int]*lsmodel.FileInfo
	node            *Anytype
}

func (i *image) GetFileForWidth(ctx context.Context, wantWidth int) (File, error) {
	if i.variantsByWidth != nil {
		return i.getFileForWidthFromCache(wantWidth)
	}

	sizeName := getSizeForWidth(wantWidth)
	fileIndex, err := i.node.getFileIndexForPath("/ipfs/" + i.hash + "/" + sizeName)
	if err != nil {
		return nil, err
	}

	return &file{
		hash:  fileIndex.Hash,
		index: fileIndex,
		node:  i.node,
	}, nil
}

func (i *image) GetFileForLargestWidth(ctx context.Context) (File, error) {
	if i.variantsByWidth != nil {
		return i.getFileForWidthFromCache(math.MaxInt32)
	}

	sizeName := "large"
	fileIndex, err := i.node.getFileIndexForPath("/ipfs/" + i.hash + "/" + sizeName)
	if err != nil {
		return nil, err
	}

	return &file{
		hash:  fileIndex.Hash,
		index: fileIndex,
		node:  i.node,
	}, nil
}

func (i *image) Hash() string {
	return i.hash
}

func (i *image) Exif() (*mill.ImageExifSchema, error) {
	return nil, fmt.Errorf("not implemented")
}

func (a *Anytype) ImageAddWithBytes(ctx context.Context, content []byte, filename string) (Image, error) {
	dir, err := a.buildDirectory(ctx, content, filename, schema.ImageNode())
	if err != nil {
		return nil, err
	}

	node, keys, err := a.AddNodeFromDirs(ctx, &lsmodel.DirectoryList{Items: []*lsmodel.Directory{dir}})
	if err != nil {
		return nil, err
	}

	nodeHash := node.Cid().String()

	filesKeysCacheMutex.Lock()
	defer filesKeysCacheMutex.Unlock()
	filesKeysCache[nodeHash] = keys.KeysByPath

	err = a.indexFileData(ctx, node, nodeHash)
	if err != nil {
		return nil, err
	}

	var variantsByWidth = make(map[int]*lsmodel.FileInfo, len(dir.Files))
	for _, f := range dir.Files {
		if f.Mill != "/image/resize" {
			continue
		}
		if v, exists := f.Meta.Fields["width"]; exists {
			variantsByWidth[int(v.GetNumberValue())] = f
		}
	}

	return &image{
		hash:            nodeHash,
		variantsByWidth: variantsByWidth,
		node:            a,
	}, nil
}

func (a *Anytype) ImageAddWithReader(ctx context.Context, content io.Reader, filename string) (Image, error) {
	b, err := ioutil.ReadAll(content)
	if err != nil {
		return nil, err
	}

	// use ImageAddWithBytes because we need seeker underlying
	// todo: rewrite when all stack including mill and aes will use reader
	return a.ImageAddWithBytes(ctx, b, filename)
}

func (i *image) getFileForWidthFromCache(wantWidth int) (File, error) {
	var maxWidth int
	var maxWidthImage *lsmodel.FileInfo

	var minWidthMatched int
	var minWidthMatchedImage *lsmodel.FileInfo

	for width, fileIndex := range i.variantsByWidth {
		if width >= maxWidth {
			maxWidth = width
			maxWidthImage = fileIndex
		}

		if width > wantWidth &&
			(minWidthMatchedImage == nil || minWidthMatched > width) {
			minWidthMatchedImage = fileIndex
			minWidthMatched = width
		}
	}

	if minWidthMatchedImage != nil {
		return &file{
			hash:  minWidthMatchedImage.Hash,
			index: minWidthMatchedImage,
			node:  i.node,
		}, nil
	} else if maxWidthImage != nil {
		return &file{
			hash:  maxWidthImage.Hash,
			index: maxWidthImage,
			node:  i.node,
		}, nil
	}

	return nil, ErrFileNotFound
}

var imageWidthByName = map[string]int{
	"thumb": 100,
	"small": 320,
	"large": 1280,
}

func getSizeForWidth(wantWidth int) string {
	var maxWidth int
	var maxWidthSize string
	for sizeName, width := range imageWidthByName {
		if width >= wantWidth {
			return sizeName
		}
		if width > maxWidth {
			maxWidthSize = sizeName
			maxWidth = width
		}
	}
	return maxWidthSize
}
