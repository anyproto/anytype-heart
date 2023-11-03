package space

import (
	"context"
	"time"

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
)

type VirtualSpace struct {
	*space
}

func NewVirtualSpace(s *service, spaceID string) *VirtualSpace {
	coreSpace := newCommonSpace(spaceID)
	vs := &VirtualSpace{
		space: &space{
			service:                s,
			Space:                  coreSpace,
			installer:              s.bundledObjectsInstaller,
			loadMandatoryObjectsCh: make(chan struct{}),
		},
	}
	return vs
}

func (vs *VirtualSpace) Close(_ context.Context) error {
	staticObjects := vs.service.sourceService.GetStaticObjectsBySpaceID(vs.Id())
	for _, id := range staticObjects {
		vs.service.sourceService.RemoveStaticSource(id)
	}
	return nil
}

func newCommonSpace(spaceID string) commonspace.Space {
	return &commonSpace{spaceID: spaceID}
}

type commonSpace struct {
	spaceID string
}

func (c *commonSpace) Id() string {
	return c.spaceID
}

func (c *commonSpace) Init(ctx context.Context) error {
	return nil
}

func (c *commonSpace) Acl() syncacl.SyncAcl {
	return nil
}

func (c *commonSpace) StoredIds() []string {
	return nil
}

func (c *commonSpace) DebugAllHeads() []headsync.TreeHeads {
	return nil
}

func (c *commonSpace) Description() (desc commonspace.SpaceDescription, err error) {
	return
}

func (c *commonSpace) TreeBuilder() objecttreebuilder.TreeBuilder {
	return nil
}

func (c *commonSpace) TreeSyncer() treesyncer.TreeSyncer {
	return nil
}

func (c *commonSpace) SyncStatus() syncstatus.StatusUpdater {
	return nil
}

func (c *commonSpace) Storage() spacestorage.SpaceStorage {
	return nil
}

func (c *commonSpace) DeleteTree(ctx context.Context, id string) (err error) {
	return nil
}

func (c *commonSpace) GetNodePeers(ctx context.Context) (peer []peer.Peer, err error) {
	return
}

func (c *commonSpace) HandleMessage(ctx context.Context, msg objectsync.HandleMessage) (err error) {
	return
}

func (c *commonSpace) HandleSyncRequest(ctx context.Context, req *spacesyncproto.ObjectSyncMessage) (resp *spacesyncproto.ObjectSyncMessage, err error) {
	return
}

func (c *commonSpace) HandleRangeRequest(ctx context.Context, req *spacesyncproto.HeadSyncRequest) (resp *spacesyncproto.HeadSyncResponse, err error) {
	return
}

func (c *commonSpace) TryClose(objectTTL time.Duration) (close bool, err error) {
	return
}

func (c *commonSpace) Close() error {
	return nil
}
