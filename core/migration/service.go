package migration

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/app"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

const CName = "migration"

const (
	// idleCheckInterval is how often to check if the indexer is idle
	idleCheckInterval = 10 * time.Second
	// minIdleDelay is the minimum time the indexer must be idle before running migrations
	minIdleDelay = 30 * time.Second
)

var log = logging.LoggerNotSugared(CName)

// Indexer provides access to index timing information
type Indexer interface {
	GetLastIndexTime(spaceId string) time.Time
}

// NetworkConfig provides access to network mode configuration
type NetworkConfig interface {
	app.Component
	GetNetworkMode() pb.RpcAccountNetworkMode
}

// Service runs migrations when the indexer is idle
type Service interface {
	app.Component
	RunMigrationsWhenIdle(spaceId string, derivedIDs threads.DerivedSmartblockIds)
}

type service struct {
	objectStore    objectstore.ObjectStore
	detailsService detailservice.Service
	indexer        Indexer
	chatRepository chatrepository.Service
	networkConfig  NetworkConfig
	nodeStatus     nodestatus.NodeStatus
	compCtx        context.Context
	compCancel     context.CancelFunc
}

func New() Service {
	return &service{}
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) error {
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.detailsService = app.MustComponent[detailservice.Service](a)
	s.indexer = app.MustComponent[Indexer](a)
	s.chatRepository = app.MustComponent[chatrepository.Service](a)
	s.networkConfig = app.MustComponent[NetworkConfig](a)
	s.nodeStatus = app.MustComponent[nodestatus.NodeStatus](a)
	s.compCtx, s.compCancel = context.WithCancel(context.Background())
	return nil
}

func (s *service) Run(ctx context.Context) error {
	return nil
}

func (s *service) Close() error {
	s.compCancel()
	return nil
}

// RunMigrationsWhenIdle waits for the indexer to become idle and then runs all migrations.
// In network mode (not local-only), it also waits for the node to be connected to the network
// to ensure remote changes have been received before running migrations.
func (s *service) RunMigrationsWhenIdle(spaceId string, derivedIDs threads.DerivedSmartblockIds) {
	isLocalOnly := s.networkConfig.GetNetworkMode() == pb.RpcAccount_LocalOnly

	for {
		select {
		case <-time.After(idleCheckInterval):
			lastIndex := s.indexer.GetLastIndexTime(spaceId)
			if lastIndex.IsZero() || time.Since(lastIndex) <= minIdleDelay {
				continue
			}

			// Indexer is idle - now check network connection if not local-only
			if !isLocalOnly {
				status, ok := s.nodeStatus.TryGetNodeStatus(spaceId)
				if !ok || status != nodestatus.Online {
					continue // Status not set yet, or not online
				}
			}

			s.runAllMigrations(s.compCtx, spaceId, derivedIDs)
			return
		case <-s.compCtx.Done():
			return
		}
	}
}

func (s *service) runAllMigrations(ctx context.Context, spaceId string, derivedIDs threads.DerivedSmartblockIds) {
	workspaceId := derivedIDs.Workspace
	if err := s.runObjectContextMigration(ctx, spaceId, workspaceId); err != nil {
		log.Error("object context migration failed",
			zap.Int("version", currentObjectContextMigrationVersion),
			zap.String("spaceId", spaceId),
			zap.Error(err))
	}
	// Future migrations can be added here
}
