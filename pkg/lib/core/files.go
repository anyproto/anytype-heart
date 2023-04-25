package core

import (
	"context"
	"fmt"

	files2 "github.com/anytypeio/go-anytype-middleware/core/files"
)

var ErrFileNotFound = fmt.Errorf("file not found")

func (a *Anytype) FileByHash(ctx context.Context, hash string) (File, error) {
	fileList, err := a.fileStore.ListByTarget(hash)
	if err != nil {
		return nil, err
	}

	if len(fileList) == 0 || fileList[0].MetaHash == "" {
		// info from ipfs
		fileList, err = a.files.FileIndexInfo(ctx, hash, false)
		if err != nil {
			log.With("cid", hash).Errorf("FileByHash: failed to retrieve from IPFS: %s", err.Error())
			return nil, ErrFileNotFound
		}
	}

	fileIndex := fileList[0]
	return &file{
		hash: hash,
		info: fileIndex,
		node: a.files,
	}, nil
}

// TODO: Touch the file to fire indexing
func (a *Anytype) FileAdd(ctx context.Context, options ...files2.AddOption) (File, error) {
	opts := files2.AddOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	err := a.files.NormalizeOptions(ctx, &opts)
	if err != nil {
		return nil, err
	}

	hash, info, err := a.files.FileAdd(ctx, opts)
	if err != nil {
		return nil, err
	}

	f := &file{
		hash: hash,
		info: info,
		node: a.files,
	}
	return f, nil
}
