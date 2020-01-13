package core

import (
	"bytes"
	"fmt"
	"io"
	"math"

	"github.com/anytypeio/go-anytype-library/schema"
	"github.com/textileio/go-textile/mill"
	tpb "github.com/textileio/go-textile/pb"
)

type Image interface {
	Exif() (*mill.ImageExifSchema, error)
	Hash() string
	GetFileForWidth(wantWidth int) (File, error)
	GetFileForLargestWidth() (File, error)
}

type image struct {
	hash            string // directory hash
	variantsByWidth map[int]tpb.FileIndex
	node            *Anytype
}

func (i *image) GetFileForWidth(wantWidth int) (File, error) {
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

func (i *image) GetFileForLargestWidth() (File, error) {
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

func (a *Anytype) ImageAddWithBytes(content []byte, filename string) (Image, error) {
	reader := bytes.NewReader(content)
	return a.ImageAddWithReader(reader, filename)
}

func (a *Anytype) ImageAddWithReader(content io.Reader, filename string) (Image, error) {
	dir, err := a.buildDirectory(content, filename, schema.ImageNode())
	if err != nil {
		return nil, err
	}

	node, keys, err := a.textile().AddNodeFromDirs(&tpb.DirectoryList{Items: []*tpb.Directory{dir}})
	if err != nil {
		return nil, err
	}

	nodeHash := node.Cid().Hash().B58String()

	filesKeysCacheMutex.Lock()
	defer filesKeysCacheMutex.Unlock()
	filesKeysCache[nodeHash] = keys.Files

	err = a.indexFileData(node, nodeHash)
	if err != nil {
		return nil, err
	}

	var variantsByWidth = make(map[int]tpb.FileIndex, len(dir.Files))
	for _, f := range dir.Files {
		if v, exists := f.Meta.Fields["width"]; exists {
			variantsByWidth[int(v.GetNumberValue())] = *f
		}
	}

	return &image{
		hash:            nodeHash,
		variantsByWidth: variantsByWidth,
		node:            a,
	}, nil
}

func (i *image) getFileForWidthFromCache(wantWidth int) (File, error) {
	var maxWidth int
	var maxWidthImage tpb.FileIndex

	var minWidthMatched int
	var minWidthMatchedImage tpb.FileIndex

	for width, fileIndex := range i.variantsByWidth {
		if width >= maxWidth {
			maxWidth = width
			maxWidthImage = fileIndex
		}

		if width > wantWidth &&
			(minWidthMatchedImage.Hash == "" || minWidthMatched > width) {
			minWidthMatchedImage = fileIndex
			minWidthMatched = width
		}
	}

	if minWidthMatchedImage.Hash != "" {
		return &file{
			hash:  minWidthMatchedImage.Hash,
			index: &minWidthMatchedImage,
			node:  i.node,
		}, nil
	} else if maxWidthImage.Hash != "" {
		return &file{
			hash:  maxWidthImage.Hash,
			index: &maxWidthImage,
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
