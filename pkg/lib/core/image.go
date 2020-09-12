package core

import (
	"context"
	"fmt"
	"math"

	"github.com/anytypeio/go-anytype-library/files"
	"github.com/anytypeio/go-anytype-library/mill"
	"github.com/anytypeio/go-anytype-library/pb/storage"
)

type Image interface {
	Exif() (*mill.ImageExifSchema, error)
	Hash() string
	GetFileForWidth(ctx context.Context, wantWidth int) (File, error)
	GetFileForLargestWidth(ctx context.Context) (File, error)
}

type image struct {
	hash            string // directory hash
	variantsByWidth map[int]*storage.FileInfo
	service         *files.Service
}

func (i *image) GetFileForWidth(ctx context.Context, wantWidth int) (File, error) {
	if i.variantsByWidth != nil {
		return i.getFileForWidthFromCache(wantWidth)
	}

	sizeName := getSizeForWidth(wantWidth)
	fileIndex, err := i.service.FileGetInfoForPath("/ipfs/" + i.hash + "/" + sizeName)
	if err != nil {
		return nil, err
	}

	return &file{
		hash: fileIndex.Hash,
		info: fileIndex,
		node: i.service,
	}, nil
}

func (i *image) GetFileForLargestWidth(ctx context.Context) (File, error) {
	if i.variantsByWidth != nil {
		return i.getFileForWidthFromCache(math.MaxInt32)
	}

	sizeName := "large"
	fileIndex, err := i.service.FileGetInfoForPath("/ipfs/" + i.hash + "/" + sizeName)
	if err != nil {
		return nil, err
	}

	return &file{
		hash: fileIndex.Hash,
		info: fileIndex,
		node: i.service,
	}, nil
}

func (i *image) Hash() string {
	return i.hash
}

func (i *image) Exif() (*mill.ImageExifSchema, error) {
	return nil, fmt.Errorf("not implemented")
}

func (i *image) getFileForWidthFromCache(wantWidth int) (File, error) {
	var maxWidth int
	var maxWidthImage *storage.FileInfo

	var minWidthMatched int
	var minWidthMatchedImage *storage.FileInfo

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
			hash: minWidthMatchedImage.Hash,
			info: minWidthMatchedImage,
			node: i.service,
		}, nil
	} else if maxWidthImage != nil {
		return &file{
			hash: maxWidthImage.Hash,
			info: maxWidthImage,
			node: i.service,
		}, nil
	}

	return nil, ErrFileNotFound
}

var imageWidthByName = map[string]int{
	"thumb": 100,
	"small": 320,
	"large": 1920,
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
