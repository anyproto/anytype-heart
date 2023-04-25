package files

import (
	"context"
	"fmt"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/samber/lo"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/storage"
)

func (s *service) FileOffload(fileID string, includeNotPinned bool) (totalSize uint64, err error) {
	if err := s.checkIfPinned(fileID, includeNotPinned); err != nil {
		return 0, err
	}

	return s.fileOffload(fileID)
}

func (s *service) checkIfPinned(fileID string, includeNotPinned bool) error {
	if includeNotPinned {
		return nil
	}

	isPinned, err := s.isFilePinned(fileID)
	if err != nil {
		return fmt.Errorf("check if file is pinned: %w", err)
	}
	if !isPinned {
		return fmt.Errorf("file %s is not pinned yet", fileID)
	}
	return nil
}

func (s *service) isFilePinned(fileID string) (bool, error) {
	stat, err := s.fileSync.FileStat(context.Background(), s.spaceService.AccountId(), fileID)
	if err != nil {
		return false, fmt.Errorf("file stat %s: %w", fileID, err)
	}

	return stat.UploadedChunksCount == stat.TotalChunksCount, nil
}

func (s *service) fileOffload(hash string) (totalSize uint64, err error) {
	log.With("fileID", hash).Info("offload file")
	totalSize, cids, err := s.getAllExistingFileBlocksCids(hash)
	if err != nil {
		return 0, err
	}

	for _, c := range cids {
		err = s.commonFile.DAGService().Remove(context.Background(), c)
		if err != nil {
			// no need to check for cid not exists
			return 0, err
		}
	}

	return totalSize, nil
}

func (s *service) FileListOffload(fileIDs []string, includeNotPinned bool) (totalBytesOffloaded uint64, totalFilesOffloaded uint64, err error) {
	if len(fileIDs) == 0 {
		allFiles, err := s.fileStore.List()
		if err != nil {
			return 0, 0, fmt.Errorf("list all files: %w", err)
		}

		allTargets := lo.Map(allFiles, func(file *storage.FileInfo, _ int) []string {
			return file.Targets
		})
		fileIDs = lo.Uniq(lo.Flatten(allTargets))
	}

	if !includeNotPinned {
		fileIDs, err = s.keepOnlyPinnedOrDeleted(fileIDs)
		if err != nil {
			return 0, 0, fmt.Errorf("keep only pinned: %w", err)
		}
	}

	for _, fileID := range fileIDs {
		bytesRemoved, err := s.fileOffload(fileID)
		if err != nil {
			log.Errorf("failed to offload file %s: %s", fileID, err.Error())
			continue
		}
		if bytesRemoved > 0 {
			totalBytesOffloaded += bytesRemoved
			totalFilesOffloaded++
		}
	}
	return
}

func (s *service) isFileDeleted(fileID string) (bool, error) {
	roots, err := s.fileStore.ListByTarget(fileID)
	if err == localstore.ErrNotFound {
		return true, nil
	}
	return len(roots) == 0, err
}

func (s *service) keepOnlyPinnedOrDeleted(fileIDs []string) ([]string, error) {
	fileStats, err := s.fileSync.FileListStats(context.Background(), s.spaceService.AccountId(), fileIDs)
	if err != nil {
		return nil, fmt.Errorf("files stat: %w", err)
	}

	fileIDs = fileIDs[:0]
	for _, fileStat := range fileStats {
		if fileStat.IsPinned() {
			fileIDs = append(fileIDs, fileStat.FileId)
			continue
		}
		isDeleted, err := s.isFileDeleted(fileStat.FileId)
		if err != nil {
			log.With("fileID", fileStat.FileId).Errorf("failed to check if file is deleted: %s", err)
			continue
		}
		if isDeleted {
			fileIDs = append(fileIDs, fileStat.FileId)
		}
	}
	return fileIDs, nil
}

func (s *service) getAllExistingFileBlocksCids(hash string) (totalSize uint64, cids []cid.Cid, err error) {
	var getCidsLinksRecursively func(c cid.Cid) (err error)

	var visitedMap = make(map[string]struct{})
	getCidsLinksRecursively = func(c cid.Cid) (err error) {
		if exists, err := s.commonFile.HasCid(context.Background(), c); err != nil {
			return err
		} else if !exists {
			// double-check the blockstore, if we don't have the block - we have not yet downloaded it
			// otherwise format.GetLinks will do bitswap
			return nil
		}
		cids = append(cids, c)

		// here we can be sure that the block is loaded to the blockstore, so 1s should be more than enough
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		n, err := s.commonFile.DAGService().Get(ctx, c)
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
