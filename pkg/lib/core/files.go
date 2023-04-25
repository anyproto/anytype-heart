package core

import (
	"context"
	"fmt"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
)

var ErrFileNotFound = fmt.Errorf("file not found")

func (a *Anytype) FileGetKeys(hash string) (*files.FileKeys, error) {
	return a.files.FileGetKeys(hash)
}

func (a *Anytype) FileStoreKeys(fileKeys ...files.FileKeys) error {
	var fks []filestore.FileKeys

	for _, fk := range fileKeys {
		fks = append(fks, filestore.FileKeys{
			Hash: fk.Hash,
			Keys: fk.Keys,
		})
	}

	return a.fileStore.AddFileKeys(fks...)
}

func (a *Anytype) GetAllExistingFileBlocksCids(hash string) (totalSize uint64, cids []cid.Cid, err error) {
	var getCidsLinksRecursively func(c cid.Cid) (err error)

	var visitedMap = make(map[string]struct{})
	getCidsLinksRecursively = func(c cid.Cid) (err error) {
		if exists, err := a.commonFiles.HasCid(context.Background(), c); err != nil {
			return err
		} else if !exists {
			// double-check the blockstore, if we don't have the block - we have not yet downloaded it
			// otherwise format.GetLinks will do bitswap
			return nil
		}
		cids = append(cids, c)

		// here we can be sure that the block is loaded to the blockstore, so 1s should be more than enough
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		n, err := a.commonFiles.DAGService().Get(ctx, c)
		if err != nil {
			log.Errorf("GetAllExistingFileBlocksCids: failed to get links: %s", err.Error())
		}
		cancel()
		if n != nil {
			// use rawData because Size() includes size of inner links which may be not loaded
			totalSize += uint64(len(n.RawData()))
		}
		if n == nil || len(n.Links()) == 0 {
			return nil
		}
		for _, link := range n.Links() {
			if _, visited := visitedMap[link.Cid.String()]; visited {
				continue
			}
			visitedMap[link.Cid.String()] = struct{}{}
			err := getCidsLinksRecursively(link.Cid)
			if err != nil {
				return err
			}
		}

		return
	}

	c, err := cid.Parse(hash)
	if err != nil {
		return 0, nil, err
	}

	err = getCidsLinksRecursively(c)

	return
}

func (a *Anytype) FileOffload(hash string) (totalSize uint64, err error) {
	totalSize, cids, err := a.GetAllExistingFileBlocksCids(hash)
	if err != nil {
		return 0, err
	}

	for _, c := range cids {
		c, err := cid.Parse(c)
		if err != nil {
			return 0, err
		}

		err = a.commonFiles.DAGService().Remove(context.Background(), c)
		if err != nil {
			// no need to check for cid not exists
			return 0, err
		}
	}

	return totalSize, nil
}

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

	f := &file{
		hash: hash,
		info: info,
		node: a.files,
	}
	return f, nil
}
