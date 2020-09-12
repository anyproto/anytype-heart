package core

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/anytypeio/go-anytype-library/files"
	"github.com/anytypeio/go-anytype-library/localstore"
)

var ErrFileNotFound = fmt.Errorf("file not found")

func (a *Anytype) FileGetKeys(hash string) (*FileKeys, error) {
	m, err := a.localStore.Files.GetFileKeys(hash)
	if err != nil {
		if err != localstore.ErrNotFound {
			return nil, err
		}
	} else {
		return &FileKeys{
			Hash: hash,
			Keys: m,
		}, nil
	}

	// in case we don't have keys cached fot this file
	// we should have all the CIDs locally, so 5s is more than enough
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	fileKeysRestored, err := a.files.FileRestoreKeys(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to restore file keys: %w", err)
	}

	return &FileKeys{
		Hash: hash,
		Keys: fileKeysRestored,
	}, nil
}

func (a *Anytype) FileStoreKeys(fileKeys ...FileKeys) error {
	var fks []localstore.FileKeys

	for _, fk := range fileKeys {
		fks = append(fks, localstore.FileKeys{
			Hash: fk.Hash,
			Keys: fk.Keys,
		})
	}

	return a.localStore.Files.AddFileKeys(fks...)
}

func (a *Anytype) FileByHash(ctx context.Context, hash string) (File, error) {
	fileList, err := a.localStore.Files.ListByTarget(hash)
	if err != nil {
		return nil, err
	}

	if len(fileList) == 0 {
		// info from ipfs
		fileList, err = a.files.FileIndexInfo(ctx, hash)
		if err != nil {
			log.Errorf("FileByHash: failed to retrieve from IPFS: %s", err.Error())
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
