package space

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/headsync"
	"github.com/anyproto/any-sync/commonspace/object/acl/syncacl"
	"github.com/anyproto/any-sync/commonspace/object/treesyncer"
	"github.com/anyproto/any-sync/commonspace/objectsync"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/anyproto/any-sync/net/peer"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

type marketplaceSpace struct {
	*space
}

type builtinTemplateService interface {
	app.Component
	RegisterBuiltinTemplates(space Space) error
}

func (s *service) initMarketplaceSpace() error {
	coreSpace := newMarketplaceCommon()
	spc := &marketplaceSpace{
		space: &space{
			service:                s,
			Space:                  coreSpace,
			installer:              s.bundledObjectsInstaller,
			loadMandatoryObjectsCh: make(chan struct{}),
		},
	}
	spc.Cache = objectcache.New(s.accountService, s.objectFactory, s.personalSpaceID, spc)

	s.preLoad(spc)

	err := s.builtinTemplateService.RegisterBuiltinTemplates(spc)
	if err != nil {
		return fmt.Errorf("register builtin templates: %w", err)
	}
	err = s.indexer.ReindexMarketplaceSpace(spc)
	if err != nil {
		return fmt.Errorf("reindex marketplace space: %w", err)
	}
	s.marketplaceSpace = spc
	return nil
}

func (s *marketplaceSpace) GetRelationIdByKey(ctx context.Context, key domain.RelationKey) (id string, err error) {
	return addr.BundledRelationURLPrefix + key.String(), nil
}

func (s *marketplaceSpace) GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error) {
	return addr.BundledObjectTypeURLPrefix + key.String(), nil
}

func newMarketplaceCommon() commonspace.Space {
	return &marketplaceCommonSpace{}
}

type marketplaceCommonSpace struct {
}

func (m *marketplaceCommonSpace) Id() string {
	return addr.AnytypeMarketplaceWorkspace
}

func (m *marketplaceCommonSpace) Init(ctx context.Context) error {
	return nil
}

func (m *marketplaceCommonSpace) Acl() syncacl.SyncAcl {
	return nil
}

func (m *marketplaceCommonSpace) StoredIds() []string {
	return nil
}

func (m *marketplaceCommonSpace) DebugAllHeads() []headsync.TreeHeads {
	return nil
}

func (m *marketplaceCommonSpace) Description() (desc commonspace.SpaceDescription, err error) {
	return
}

func (m *marketplaceCommonSpace) TreeBuilder() objecttreebuilder.TreeBuilder {
	return nil
}

func (m *marketplaceCommonSpace) TreeSyncer() treesyncer.TreeSyncer {
	return nil
}

func (m *marketplaceCommonSpace) SyncStatus() syncstatus.StatusUpdater {
	return nil
}

func (m *marketplaceCommonSpace) Storage() spacestorage.SpaceStorage {
	return nil
}

func (m *marketplaceCommonSpace) DeleteTree(ctx context.Context, id string) (err error) {
	return nil
}

func (m *marketplaceCommonSpace) GetNodePeers(ctx context.Context) (peer []peer.Peer, err error) {
	return
}

func (m *marketplaceCommonSpace) HandleMessage(ctx context.Context, msg objectsync.HandleMessage) (err error) {
	return
}

func (m *marketplaceCommonSpace) HandleSyncRequest(ctx context.Context, req *spacesyncproto.ObjectSyncMessage) (resp *spacesyncproto.ObjectSyncMessage, err error) {
	return
}

func (m *marketplaceCommonSpace) HandleRangeRequest(ctx context.Context, req *spacesyncproto.HeadSyncRequest) (resp *spacesyncproto.HeadSyncResponse, err error) {
	return
}

func (m *marketplaceCommonSpace) TryClose(objectTTL time.Duration) (close bool, err error) {
	return
}

func (m *marketplaceCommonSpace) Close() error {
	return nil
}
