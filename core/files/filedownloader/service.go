package filedownloader

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filehelper"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const CName = "core.files.filedownloader"

var log = logging.Logger(CName).Desugar()

type Service interface {
	SetEnabled(enabled bool)
	DownloadToLocalStore(ctx context.Context, spaceId string, cid domain.FileId) error
	app.ComponentRunnable
}

type service struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	dagService           ipld.DAGService
	crossSpaceSubService crossspacesub.Service
	objectGetter         cache.ObjectGetter

	lock       sync.Mutex
	downloader *downloader
}

func New() Service {
	ctx, ctxCancel := context.WithCancel(context.Background())
	return &service{
		ctx:       ctx,
		ctxCancel: ctxCancel,
	}
}

func (s *service) SetEnabled(enabled bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if enabled {
		if s.downloader == nil {
			s.downloader = s.newDownloader()
			s.downloader.start()
		}
	} else {
		if s.downloader != nil {
			s.downloader.stop()
			s.downloader = nil
		}
	}
}

func (s *service) Init(a *app.App) error {
	s.crossSpaceSubService = app.MustComponent[crossspacesub.Service](a)
	s.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	commonFile := app.MustComponent[fileservice.FileService](a)
	s.dagService = commonFile.DAGService()
	return nil
}

func (s *service) Name() string {
	return CName
}

func (s *service) DownloadToLocalStore(ctx context.Context, spaceId string, fileCid domain.FileId) error {
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

func (s *service) Run(ctx context.Context) error {
	return nil
}

func (s *service) Close(ctx context.Context) error {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	return nil
}

func (s *service) newDownloader() *downloader {
	ctx, ctxCancel := context.WithCancel(s.ctx)
	return &downloader{
		ctx:                  ctx,
		ctxCancel:            ctxCancel,
		crossSpaceSubService: s.crossSpaceSubService,
		objectGetter:         s.objectGetter,
		handleTask: func(ctx context.Context, t downloadTask) error {
			return s.DownloadToLocalStore(ctx, t.spaceId, t.fileId)
		},
		requestTaskCh: make(chan chan downloadTask),
		addTaskCh:     make(chan downloadTask),
		removeTaskCh:  make(chan string),
		lock:          sync.Mutex{},
		tasks:         map[string]downloadTask{},
	}
}
