package filedownloader

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
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
	CacheFile(spaceId string, fileId domain.FileId)
	CancelFileCaching(fileId domain.FileId)
	DownloadToLocalStore(ctx context.Context, spaceId string, cid domain.FileId, blocksLimit int) error
	app.ComponentRunnable
}

type service struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	dagService           ipld.DAGService
	crossSpaceSubService crossspacesub.Service
	objectGetter         cache.ObjectGetter
	config               *config.Config
	networkState         device.NetworkState
	cacheWarmer          *cacheWarmer

	lock       sync.Mutex
	isEnabled  bool
	wifiOnly   bool
	downloader *downloader
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
	s.dagService = commonFile.DAGService()
	s.config = app.MustComponent[*config.Config](a)
	s.networkState = app.MustComponent[device.NetworkState](a)
	s.networkState.RegisterHook(s.networkStateChanged)

	s.cacheWarmer = newCacheWarmer(s.ctx, 10, 20, 2*time.Minute, s.DownloadToLocalStore)

	return nil
}

func (s *service) Run(ctx context.Context) error {
	err := s.SetEnabled(s.config.AutoDownloadFiles, s.config.AutoDownloadFiles)
	if err != nil {
		log.Error("set enabled", zap.Error(err))
	}
	for range 5 {
		go s.cacheWarmer.runWorker()
	}
	go s.cacheWarmer.run()
	return nil
}

func (s *service) Close(ctx context.Context) error {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	return nil
}

func (s *service) SetEnabled(enabled bool, wifiOnly bool) error {
	s.setEnabled(enabled, wifiOnly)

	// Write to the config file only if it's changed
	if s.config.AutoDownloadFiles != enabled || s.config.AutoDownloadOnWifiOnly != wifiOnly {
		cfgPart := config.ConfigAutoDownloadFiles{}
		cfgPart.AutoDownloadFiles = enabled
		cfgPart.AutoDownloadOnWifiOnly = wifiOnly
		return config.WriteJsonConfig(s.config.GetConfigPath(), cfgPart)
	}
	return nil
}

func (s *service) setEnabled(enabled bool, wifiOnly bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.isEnabled = enabled
	s.wifiOnly = wifiOnly
	if enabled {
		if s.downloader == nil {
			s.downloader = s.newDownloader()
			s.downloader.start()
		}
	} else if s.downloader != nil {
		s.downloader.stop()
		s.downloader = nil
	}
}

func (s *service) CacheFile(spaceId string, fileId domain.FileId) {
	s.cacheWarmer.enqueue(spaceId, fileId)
}

func (s *service) CancelFileCaching(fileId domain.FileId) {
	s.cacheWarmer.cancelTask(fileId)
}

func (s *service) networkStateChanged(networkState model.DeviceNetworkType) {
	s.lock.Lock()
	isEnabled := s.isEnabled
	wifiOnly := s.wifiOnly
	s.lock.Unlock()

	if isEnabled {
		if wifiOnly {
			if networkState == model.DeviceNetworkType_WIFI {
				s.setEnabled(true, wifiOnly)
			} else {
				s.setEnabled(false, wifiOnly)
			}
		} else {
			s.setEnabled(true, wifiOnly)
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
