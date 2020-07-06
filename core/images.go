package core

import (
	"context"
	"fmt"
	"io"

	"github.com/anytypeio/go-anytype-library/files"
	"github.com/anytypeio/go-anytype-library/pb/storage"
)

var ErrImageNotFound = fmt.Errorf("image not found")

func (a *Anytype) ImageByHash(ctx context.Context, hash string) (Image, error) {
	files, err := a.localStore.Files.ListByTarget(hash)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		// info from ipfs
		files, err = a.files.FileIndexInfo(ctx, hash)
		if err != nil {
			log.Errorf("ImageByHash: failed to retrieve from IPFS: %s", err.Error())
			return nil, ErrImageNotFound
		}
	}

	var variantsByWidth = make(map[int]*storage.FileInfo, len(files))
	for _, f := range files {
		if f.Mill != "/image/resize" {
			continue
		}

		if v, exists := f.Meta.Fields["width"]; exists {
			variantsByWidth[int(v.GetNumberValue())] = f
		}
	}

	return &image{
		hash:            files[0].Targets[0],
		variantsByWidth: variantsByWidth,
		service:         a.files,
	}, nil
}

func (a *Anytype) ImageAdd(ctx context.Context, options ...files.AddOption) (Image, error) {
	opts := files.AddOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	err := a.files.NormalizeOptions(ctx, &opts)
	if err != nil {
		return nil, err
	}

	hash, variants, err := a.files.ImageAdd(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &image{
		hash:            hash,
		variantsByWidth: variants,
		service:         a.files,
	}, nil
}

func (a *Anytype) ImageAddWithBytes(ctx context.Context, content []byte, filename string) (Image, error) {
	return a.ImageAdd(ctx, files.WithBytes(content), files.WithName(filename))
}

func (a *Anytype) ImageAddWithReader(ctx context.Context, content io.Reader, filename string) (Image, error) {
	return a.ImageAdd(ctx, files.WithReader(content), files.WithName(filename))
}
