package files

import (
	"context"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/filestorage"
)

func (s *service) FileOffload(ctx context.Context, id domain.FullFileId) (totalSize uint64, err error) {
	log.With("fileID", id.FileId).Info("offload file")
	totalSize, cids, err := s.getAllExistingFileBlocksCids(ctx, id)
	if err != nil {
		return 0, err
	}

	dagService := s.dagServiceForSpace(id.SpaceId)
	for _, c := range cids {
		err = dagService.Remove(context.Background(), c)
		if err != nil {
			// no need to check for cid not exists
			return 0, err
		}
	}

	return totalSize, nil
}

func (s *service) getAllExistingFileBlocksCids(ctx context.Context, id domain.FullFileId) (totalSize uint64, cids []cid.Cid, err error) {
	var getCidsLinksRecursively func(c cid.Cid) (err error)
	dagService := s.dagServiceForSpace(id.SpaceId)

	var visitedMap = make(map[string]struct{})
	getCidsLinksRecursively = func(c cid.Cid) (err error) {
		if exists, err := s.hasCid(ctx, id.SpaceId, c); err != nil {
			return err
		} else if !exists {
			// double-check the blockstore, if we don't have the block - we have not yet downloaded it
			// otherwise format.GetLinks will do bitswap
			return nil
		}
		cids = append(cids, c)

		// here we can be sure that the block is loaded to the blockstore, so 1s should be more than enough
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		ctx = context.WithValue(ctx, filestorage.CtxKeyRemoteLoadDisabled, true)
		n, err := dagService.Get(ctx, c)
		if err != nil {
			log.Errorf("GetAllExistingFileBlocksCids: failed to get links: %s", err)
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

	c, err := cid.Parse(id.FileId.String())
	if err != nil {
		return 0, nil, err
	}

	err = getCidsLinksRecursively(c)

	return
}
