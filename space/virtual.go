package space

import (
	"context"
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

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

const name = "virtualSpaceService"

type VirtualSpaceService interface {
	app.ComponentRunnable
	RegisterVirtualSpace(spaceID string) (err error)
}

type virtualSpaceService struct {
	objectStore objectstore.ObjectStore
}

func (v *virtualSpaceService) Init(a *app.App) (err error) {
	v.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	return nil
}

func (v *virtualSpaceService) Name() (name string) {
	return name
}

func (v *virtualSpaceService) Run(ctx context.Context) (err error) {
	return v.cleanupVirtualSpaces(err)
}

func (v *virtualSpaceService) Close(ctx context.Context) (err error) {
	return v.cleanupVirtualSpaces(err)
}

func (v *virtualSpaceService) cleanupVirtualSpaces(err error) error {
	spaces, err := v.objectStore.ListVirtualSpaces()
	if err != nil {
		return err
	}
	for _, id := range spaces {
		err := v.objectStore.DeleteVirtualSpace(id)
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *virtualSpaceService) RegisterVirtualSpace(spaceID string) (err error) {
	return v.objectStore.SaveVirtualSpace(spaceID)
}

func NewVirtualSpaceService() VirtualSpaceService {
	return &virtualSpaceService{}
}

type VirtualSpace struct {
	*space
}

func NewVirtualSpace(s *service, spaceID string) *VirtualSpace {
	coreSpace := newVirtualCommonSpace(spaceID)
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

func newVirtualCommonSpace(spaceID string) commonspace.Space {
	return &virtualCommonSpace{spaceID: spaceID}
}

type virtualCommonSpace struct {
	spaceID string
}

func (c *virtualCommonSpace) Id() string {
	return c.spaceID
}

func (c *virtualCommonSpace) Init(ctx context.Context) error {
	return nil
}

func (c *virtualCommonSpace) Acl() syncacl.SyncAcl {
	return nil
}

func (c *virtualCommonSpace) StoredIds() []string {
	return nil
}

func (c *virtualCommonSpace) DebugAllHeads() []headsync.TreeHeads {
	return nil
}

func (c *virtualCommonSpace) Description() (desc commonspace.SpaceDescription, err error) {
	return
}

func (c *virtualCommonSpace) TreeBuilder() objecttreebuilder.TreeBuilder {
	return nil
}

func (c *virtualCommonSpace) TreeSyncer() treesyncer.TreeSyncer {
	return nil
}

func (c *virtualCommonSpace) SyncStatus() syncstatus.StatusUpdater {
	return nil
}

func (c *virtualCommonSpace) Storage() spacestorage.SpaceStorage {
	return nil
}

func (c *virtualCommonSpace) DeleteTree(ctx context.Context, id string) (err error) {
	return nil
}

func (c *virtualCommonSpace) GetNodePeers(ctx context.Context) (peer []peer.Peer, err error) {
	return
}

func (c *virtualCommonSpace) HandleMessage(ctx context.Context, msg objectsync.HandleMessage) (err error) {
	return
}

func (c *virtualCommonSpace) HandleSyncRequest(ctx context.Context, req *spacesyncproto.ObjectSyncMessage) (resp *spacesyncproto.ObjectSyncMessage, err error) {
	return
}

func (c *virtualCommonSpace) HandleRangeRequest(ctx context.Context, req *spacesyncproto.HeadSyncRequest) (resp *spacesyncproto.HeadSyncResponse, err error) {
	return
}

func (c *virtualCommonSpace) TryClose(objectTTL time.Duration) (close bool, err error) {
	return
}

func (c *virtualCommonSpace) Close() error {
	return nil
}
