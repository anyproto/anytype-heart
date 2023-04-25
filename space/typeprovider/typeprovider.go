package typeprovider

import (
	"context"
	"errors"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/space"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"strings"
	"sync"
)

const CName = "space.typeprovider"

var ErrUnknownSmartBlockType = errors.New("error unknown smartblock type")

type ObjectTypeProvider interface {
	Type(id string) (smartblock.SmartBlockType, error)
}

type objectTypeProvider struct {
	sync.Mutex
	spaceService space.Service
	cache        map[string]smartblock.SmartBlockType
}

func (o *objectTypeProvider) Init(a *app.App) (err error) {
	o.spaceService = a.MustComponent(space.CName).(space.Service)
	return
}

func (o *objectTypeProvider) Name() (name string) {
	return CName
}

func (o *objectTypeProvider) ObjectType(id string) (smartblock.SmartBlockType, error) {
	if strings.HasPrefix(id, addr.BundledRelationURLPrefix) {
		return smartblock.SmartBlockTypeBundledRelation, nil
	}

	if strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) {
		return smartblock.SmartBlockTypeBundledObjectType, nil
	}

	if len(strings.Split(id, addr.SubObjectCollectionIdSeparator)) == 2 {
		return smartblock.SmartBlockTypeSubObject, nil
	}

	// workaround for options that have no prefix
	// todo: remove this after migration to the new records format
	if bson.IsObjectIdHex(id) {
		return smartblock.SmartBlockTypeSubObject, nil
	}

	if strings.HasPrefix(id, addr.AnytypeProfileId) {
		return smartblock.SmartBlockTypeProfilePage, nil
	}
	if strings.HasPrefix(id, addr.VirtualPrefix) {
		sbt, err := addr.ExtractVirtualSourceType(id)
		if err != nil {
			return 0, err
		}
		return smartblock.SmartBlockType(sbt), nil
	}
	if strings.HasPrefix(id, addr.DatePrefix) {
		return smartblock.SmartBlockTypeDate, nil
	}

	c, err := cid.Decode(id)
	if err != nil {
		return smartblock.SmartBlockTypePage, err
	}
	// TODO: discard this fragile condition as soon as we will move to the multiaddr with prefix
	if c.Prefix().Codec == cid.DagProtobuf && c.Prefix().MhType == multihash.SHA2_256 {
		return smartblock.SmartBlockTypeFile, nil
	}
	if c.Prefix().Codec == cid.DagCBOR {
		return o.objectTypeFromSpace(id)
	}

	return smartblock.SmartBlockTypePage, ErrUnknownSmartBlockType
}

func (o *objectTypeProvider) objectTypeFromSpace(id string) (tp smartblock.SmartBlockType, err error) {
	o.Lock()
	tp, exists := o.cache[id]
	if exists {
		o.Unlock()
		return
	}
	o.Unlock()

	sp, err := o.spaceService.AccountSpace(context.Background())
	if err != nil {
		return
	}

	store := sp.Storage()
	rawRoot, err := store.TreeRoot(id)
	if err != nil {
		return
	}

	// TODO: move this into common
	root, err := o.unmarshallRoot(rawRoot)
	if err != nil {
		return
	}

	ot, err := o.objectType(root.ChangeType)
	if err != nil {
		return
	}
	o.Lock()
	defer o.Unlock()
	o.cache[id] = ot
	return
}

func (o *objectTypeProvider) objectType(changeType string) (smartblock.SmartBlockType, error) {
	return smartblock.SmartBlockTypePage, nil
}

func (o *objectTypeProvider) unmarshallRoot(rawRoot *treechangeproto.RawTreeChangeWithId) (root *treechangeproto.RootChange, err error) {
	raw := &treechangeproto.RawTreeChange{}
	err = proto.Unmarshal(rawRoot.GetRawChange(), raw)
	if err != nil {
		return
	}

	root = &treechangeproto.RootChange{}
	err = proto.Unmarshal(raw.Payload, root)
	if err != nil {
		return
	}
	return
}
