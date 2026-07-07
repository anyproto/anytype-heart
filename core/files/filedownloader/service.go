package filedownloader

/*
AI generated

Name: File Auto-Download Manager
Scope: global

## Responsibility
- Auto-downloads files to local store based on criteria: not available offline, synced, <20MB
- Provides on-demand partial file caching (CacheFile) with block limits
- Respects network state for wifi-only download mode

## Background Tasks
- downloader (5 workers): subscribes to eligible files across spaces, downloads full content (runManager, runDownloadWorker)
- cacheWarmer (5 workers): processes on-demand cache requests with limited blocks (run, runWorker)

## Documentation
Two download subsystems:
- downloader: subscribes to files matching auto-download criteria, downloads entire file, marks FileAvailableOffline
- cacheWarmer: accepts individual file requests via CacheFile, downloads limited blocks (10) with timeout (2min)
*/

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/debugstat"
	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/device"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filehelper"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "core.files.filedownloader"

var log = logging.Logger(CName).Desugar()

type Service interface {
	SetEnabled(enabled bool, wifiOnly bool) error
	SetSizeLimit(sizeLimitMb int64) error
	CacheFile(spaceId string, fileId domain.FileId)
	CancelFileCaching(fileId domain.FileId)
	DownloadToLocalStore(ctx context.Context, spaceId string, cid domain.FileId, blocksLimit int) error
	app.ComponentRunnable
}

type service struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	dagService           ipld.DAGService
	commonFile           fileservice.FileService
	crossSpaceSubService crossspacesub.Service
	objectGetter         cache.ObjectGetter
	config               *config.Config
	networkState         device.NetworkState
	statService          debugstat.StatService
	cacheWarmer          *cacheWarmer

	// skippedFiles counts CacheFile requests skipped because the file's root block
	// was already cached locally. The enqueue/warm-up/cancel counters live on the
	// cacheWarmer, where those transitions happen.
	skippedFiles atomic.Int64

	lock        sync.Mutex
	isEnabled   bool
	wifiOnly    bool
	sizeLimitMb int64
	downloader  *downloader
}

func New() Service {
	ctx, ctxCancel := context.WithCancel(context.Background())
	return &service{
		ctx:       ctx,
		ctxCancel: ctxCancel,
	}
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) error {
	s.crossSpaceSubService = app.MustComponent[crossspacesub.Service](a)
	s.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	commonFile := app.MustComponent[fileservice.FileService](a)
	s.commonFile = commonFile
	s.dagService = commonFile.DAGService()
	s.config = app.MustComponent[*config.Config](a)
	s.networkState = app.MustComponent[device.NetworkState](a)
	s.networkState.RegisterHook(s.networkStateChanged)

	s.cacheWarmer = newCacheWarmer(s.ctx, 10, 20, 2*time.Minute, s.DownloadToLocalStore)

	if statService, _ := app.GetComponent[debugstat.StatService](a); statService != nil {
		s.statService = statService
		s.statService.AddProvider(s)
	}

	return nil
}

func (s *service) Run(ctx context.Context) error {
	enabled := s.config.AutoDownloadFiles()
	wifiOnly := s.config.AutoDownloadOnWifiOnly()
	sizeLimitMb := s.config.AutoDownloadSizeLimitMb()
	s.setDownloadState(enabled, wifiOnly, sizeLimitMb)
	for range 5 {
		go s.cacheWarmer.runWorker()
	}
	go s.cacheWarmer.run()
	return nil
}

func (s *service) Close(ctx context.Context) error {
	if s.statService != nil {
		s.statService.RemoveProvider(s)
	}
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	return nil
}

func (s *service) SetEnabled(enabled bool, wifiOnly bool) error {
	s.lock.Lock()
	sizeLimitMb := s.sizeLimitMb
	s.lock.Unlock()

	s.setDownloadState(enabled, wifiOnly, sizeLimitMb)
	return s.config.SetAutoDownloadSettings(enabled, wifiOnly)
}

func (s *service) SetSizeLimit(sizeLimitMb int64) error {
	s.lock.Lock()
	isEnabled := s.isEnabled
	wifiOnly := s.wifiOnly
	s.lock.Unlock()

	s.setDownloadState(isEnabled, wifiOnly, sizeLimitMb)
	return s.config.SetAutoDownloadSizeLimitMb(sizeLimitMb)
}

func (s *service) setDownloadState(enabled bool, wifiOnly bool, sizeLimitMb int64) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.isEnabled = enabled
	s.wifiOnly = wifiOnly
	oldLimit := s.sizeLimitMb
	s.sizeLimitMb = sizeLimitMb

	sizeLimitBytes := sizeLimitMb * 1024 * 1024

	if enabled {
		if s.downloader == nil {
			s.downloader = s.newDownloader(sizeLimitBytes)
			s.downloader.start()
		} else if oldLimit != sizeLimitMb {
			// Size limit changed: restart downloader to update subscription filters
			s.downloader.stop()
			s.downloader = s.newDownloader(sizeLimitBytes)
			s.downloader.start()
		}
	} else if s.downloader != nil {
		s.downloader.stop()
		s.downloader = nil
	}
}

func (s *service) CacheFile(spaceId string, fileId domain.FileId) {
	if s.hasRootBlock(spaceId, fileId) {
		// The file's root block is already present in the local flatfs, so the
		// file has (almost certainly) been cached before. Skip the warm-up to
		// avoid a redundant DAG walk. Missing child blocks are a tolerated edge
		// case: the proxy store refetches them on demand when the file is read.
		s.skippedFiles.Add(1)
		return
	}
	s.cacheWarmer.enqueue(spaceId, fileId)
}

type stat struct {
	// SkippedFiles is the number of CacheFile requests skipped because the file's
	// root block was already cached locally.
	SkippedFiles int64 `json:"skippedFiles"`
	// EnqueuedFiles is the number of files accepted into the warm-up queue.
	EnqueuedFiles int64 `json:"enqueuedFiles"`
	// WarmedUpFiles is the number of files whose warm-up finished successfully.
	WarmedUpFiles int64 `json:"warmedUpFiles"`
	// CancelledBeforeStart is the number of queued warm-ups cancelled before they started.
	CancelledBeforeStart int64 `json:"cancelledBeforeStart"`
}

func (s *service) ProvideStat() any {
	st := stat{SkippedFiles: s.skippedFiles.Load()}
	if s.cacheWarmer != nil {
		st.EnqueuedFiles = s.cacheWarmer.enqueued.Load()
		st.WarmedUpFiles = s.cacheWarmer.warmedUp.Load()
		st.CancelledBeforeStart = s.cacheWarmer.cancelledBeforeStart.Load()
	}
	return st
}

func (s *service) StatId() string {
	return "cache"
}

func (s *service) StatType() string {
	return CName
}

// hasRootBlock reports whether the file's root block already exists in the local
// blockstore. It is a pure local key-existence check (flatfs stat) — no block is
// read, decrypted or fetched from a remote peer.
func (s *service) hasRootBlock(spaceId string, fileId domain.FileId) bool {
	rootCid, err := cid.Parse(fileId.String())
	if err != nil {
		return false
	}
	ctx := fileblockstore.CtxWithSpaceId(s.ctx, spaceId)
	exists, err := s.commonFile.HasCid(ctx, rootCid)
	if err != nil {
		log.Warn("cache file: check root block existence", zap.Error(err))
		return false
	}
	return exists
}

func (s *service) CancelFileCaching(fileId domain.FileId) {
	s.cacheWarmer.cancelTask(fileId)
}

func (s *service) networkStateChanged(networkState model.DeviceNetworkType) {
	s.lock.Lock()
	isEnabled := s.isEnabled
	wifiOnly := s.wifiOnly
	sizeLimitMb := s.sizeLimitMb
	s.lock.Unlock()

	if isEnabled {
		if wifiOnly {
			if networkState == model.DeviceNetworkType_WIFI {
				s.setDownloadState(true, wifiOnly, sizeLimitMb)
			} else {
				s.setDownloadState(false, wifiOnly, sizeLimitMb)
			}
		} else {
			s.setDownloadState(true, wifiOnly, sizeLimitMb)
		}
	}
}

func (s *service) DownloadToLocalStore(ctx context.Context, spaceId string, fileCid domain.FileId, blocksLimit int) error {
	ctx = rpcstore.ContextWithWaitAvailable(ctx)

	dagService := s.dagServiceForSpace(spaceId)

	rootCid, err := cid.Parse(fileCid.String())
	if err != nil {
		return fmt.Errorf("parse cid: %w", err)
	}

	rootNode, err := dagService.Get(ctx, rootCid)
	if err != nil {
		return fmt.Errorf("get root node: %w", err)
	}

	visited := map[cid.Cid]struct{}{}
	walker := ipld.NewWalker(ctx, ipld.NewNavigableIPLDNode(rootNode, dagService))
	err = walker.Iterate(func(navNode ipld.NavigableNode) error {
		node := navNode.GetIPLDNode()
		if _, ok := visited[node.Cid()]; !ok {
			visited[node.Cid()] = struct{}{}

			if blocksLimit > 0 && len(visited) >= blocksLimit {
				// Stop iterating
				return ipld.EndOfDag
			}
		}
		return nil
	})
	if errors.Is(err, ipld.EndOfDag) {
		return nil
	}
	return nil
}

func (s *service) dagServiceForSpace(spaceID string) ipld.DAGService {
	return filehelper.NewDAGServiceWithSpaceID(spaceID, s.dagService)
}
