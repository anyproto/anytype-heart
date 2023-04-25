package filesync

import (
	"context"
	"fmt"

	"github.com/anytypeio/any-sync/commonfile/fileproto"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/samber/lo"

	"github.com/anytypeio/go-anytype-middleware/util/conc"
)

type SpaceStat struct {
	SpaceId    string
	FileCount  int
	CidsCount  int
	BytesUsage int
	BytesLimit int
}

type FileStat struct {
	SpaceId             string
	FileId              string
	TotalChunksCount    int
	UploadedChunksCount int
	BytesUsage          int
}

func (s FileStat) IsPinned() bool {
	return s.UploadedChunksCount == s.TotalChunksCount
}

func (s FileStat) IsDeleted() bool {
	return s.UploadedChunksCount == 0
}

func (f *fileSync) SpaceStat(ctx context.Context, spaceId string) (ss SpaceStat, err error) {
	info, err := f.rpcStore.SpaceInfo(ctx, spaceId)
	if err != nil {
		return
	}
	return SpaceStat{
		SpaceId:    spaceId,
		FileCount:  int(info.FilesCount),
		CidsCount:  int(info.CidsCount),
		BytesUsage: int(info.UsageBytes),
		// TODO Remove after test
		BytesLimit: 52_428_800,
		// BytesLimit: int(info.LimitBytes),
	}, nil
}

func (f *fileSync) FileListStats(ctx context.Context, spaceID string, fileIDs []string) ([]FileStat, error) {
	filesInfo, err := f.fetchFilesInfo(ctx, spaceID, fileIDs)
	if err != nil {
		return nil, err
	}
	return conc.MapErr(filesInfo, func(fileInfo *fileproto.FileInfo) (FileStat, error) {
		return f.fileInfoToStat(ctx, spaceID, fileInfo)
	})
}

func (f *fileSync) fetchFilesInfo(ctx context.Context, spaceId string, fileIDs []string) ([]*fileproto.FileInfo, error) {
	requests := lo.Chunk(fileIDs, 50)
	responses, err := conc.MapErr(requests, func(chunk []string) ([]*fileproto.FileInfo, error) {
		return f.rpcStore.FilesInfo(ctx, spaceId, chunk...)
	})
	if err != nil {
		return nil, err
	}
	return lo.Flatten(responses), nil
}

func (f *fileSync) FileStat(ctx context.Context, spaceId, fileId string) (fs FileStat, err error) {
	fi, err := f.rpcStore.FilesInfo(ctx, spaceId, fileId)
	if err != nil {
		return
	}
	if len(fi) == 0 {
		return FileStat{}, fmt.Errorf("file not found")
	}
	file := fi[0]

	return f.fileInfoToStat(ctx, spaceId, file)
}

func (f *fileSync) fileInfoToStat(ctx context.Context, spaceId string, file *fileproto.FileInfo) (FileStat, error) {
	totalChunks, err := f.countChunks(ctx, file.FileId)
	if err != nil {
		return FileStat{}, fmt.Errorf("count chunks: %w", err)
	}

	return FileStat{
		SpaceId:             spaceId,
		FileId:              file.FileId,
		TotalChunksCount:    totalChunks,
		UploadedChunksCount: int(file.CidsCount),
		BytesUsage:          int(file.UsageBytes),
	}, nil
}

func (f *fileSync) countChunks(ctx context.Context, fileID string) (int, error) {
	chunksCount, err := f.fileStore.GetChunksCount(fileID)
	if err == nil {
		return chunksCount, nil
	}

	chunksCount, err = f.fetchChunksCount(ctx, fileID)
	if err != nil {
		return -1, fmt.Errorf("count chunks in IPFS: %w", err)
	}

	err = f.fileStore.SetChunksCount(fileID, chunksCount)

	return chunksCount, err
}

func (f *fileSync) fetchChunksCount(ctx context.Context, fileID string) (int, error) {
	fileCid, err := cid.Parse(fileID)
	if err != nil {
		return -1, err
	}
	node, err := f.dagService.Get(ctx, fileCid)
	if err != nil {
		return -1, err
	}
	return f.FetchChunksCount(ctx, node)
}

func (f *fileSync) FetchChunksCount(ctx context.Context, node ipld.Node) (int, error) {
	var count int
	visited := map[string]struct{}{}
	walker := ipld.NewWalker(ctx, ipld.NewNavigableIPLDNode(node, f.dagService))
	err := walker.Iterate(func(node ipld.NavigableNode) error {
		id := node.GetIPLDNode().Cid().String()
		if _, ok := visited[id]; !ok {
			visited[id] = struct{}{}
			count++
		}
		return nil
	})
	if err == ipld.EndOfDag {
		err = nil
	}
	return count, err
}
