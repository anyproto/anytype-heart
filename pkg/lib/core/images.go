package core

import (
	"context"
	"fmt"
	"io"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/storage"
)

var ErrImageNotFound = fmt.Errorf("image not found")

func (a *Anytype) ImageByHash(ctx context.Context, hash string) (Image, error) {
	files, err := a.fileStore.ListByTarget(hash)
	if err != nil {
		return nil, err
	}

	// check the image files count explicitly because we have a bug when the info can be cached not fully(only for some files)
	if len(files) < 4 || files[0].MetaHash == "" {
		// index image files info from ipfs
		files, err = a.files.FileIndexInfo(ctx, hash, true)
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

	img := &image{
		hash:            hash,
		variantsByWidth: variants,
		service:         a.files,
	}

	details, err := img.Details()
	if err != nil {
		return nil, err
	}

	err = a.objectStore.UpdateObjectDetails(img.hash, details, &model.Relations{Relations: bundle.MustGetType(bundle.TypeKeyImage).Relations}, false)
	if err != nil {
		return nil, err
	}

	err = a.objectStore.AddToIndexQueue(img.hash)
	if err != nil {
		return nil, err
	}

	return img, nil
}

func (a *Anytype) ImageAddWithBytes(ctx context.Context, content []byte, filename string) (Image, error) {
	return a.ImageAdd(ctx, files.WithBytes(content), files.WithName(filename))
}

func (a *Anytype) ImageAddWithReader(ctx context.Context, content io.ReadSeeker, filename string) (Image, error) {
	return a.ImageAdd(ctx, files.WithReader(content), files.WithName(filename))
}
