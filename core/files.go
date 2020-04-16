package core

import (
	"context"
	"io"

	"github.com/anytypeio/go-anytype-library/files"
)

func (a *Anytype) FileByHash(ctx context.Context, hash string) (File, error) {
	fileList, err := a.localStore.Files.ListByTarget(hash)
	if err != nil {
		return nil, err
	}

	if len(fileList) == 0 {
		a.files.KeysCacheMutex.RLock()
		defer a.files.KeysCacheMutex.RUnlock()
		// info from ipfs
		fileList, err = a.files.FileIndexInfo(ctx, hash, a.files.KeysCache[hash])
		if err != nil {
			log.Errorf("FileByHash: failed to retrieve from IPFS: %s", err.Error())
			return nil, files.ErrFileNotFound
		}
	}

	fileIndex := fileList[0]
	return &file{
		hash: hash,
		info: fileIndex,
		node: a.files,
	}, nil
}

func (a *Anytype) FileAdd(ctx context.Context, options ...files.AddOption) (File, error) {
	opts := files.AddOptions{}
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

	return &file{
		hash: hash,
		info: info,
		node: a.files,
	}, nil
}

func (a *Anytype) FileAddWithReader(ctx context.Context, content io.Reader, filename string) (File, error) {
	return a.FileAdd(ctx, files.WithReader(content), files.WithName(filename))
}

func (a *Anytype) FileAddWithBytes(ctx context.Context, content []byte, filename string) (File, error) {
	return a.FileAdd(ctx, files.WithBytes(content), files.WithName(filename))
}
