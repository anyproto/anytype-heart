package typeprovider

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/dgraph-io/badger/v3"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"

	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

const CName = "space.typeprovider"

var log = logging.Logger(CName)

var (
	ErrUnknownChangeType = errors.New("error unknown change type")
)

type SmartBlockTypeProvider interface {
	app.Component
	Type(spaceID string, id string) (smartblock.SmartBlockType, error)
	RegisterStaticType(id string, tp smartblock.SmartBlockType)
	PartitionIDsByType(spaceId string, ids []string) (map[smartblock.SmartBlockType][]string, error)
}

func (p *provider) PartitionIDsByType(spaceId string, ids []string) (map[smartblock.SmartBlockType][]string, error) {
	result := map[smartblock.SmartBlockType][]string{}
	for _, id := range ids {
		t, err := p.Type(spaceId, id)
		if err != nil {
			return nil, err
		}
		result[t] = append(result[t], id)
	}
	return result, nil
}

type provider struct {
	sync.RWMutex
	spaceService spacecore.SpaceCoreService
	badger       *badger.DB
	cache        map[string]smartblock.SmartBlockType
}

func New() SmartBlockTypeProvider {
	return &provider{}
}

var badgerPrefix = []byte("typeprovider/")

func (p *provider) Init(a *app.App) (err error) {
	p.cache = map[string]smartblock.SmartBlockType{}
	p.badger, err = app.MustComponent[datastore.Datastore](a).SpaceStorage()
	if err != nil {
		return fmt.Errorf("get badger storage: %w", err)
	}
	// TODO multi-space: I forgot why we need this
	err = p.badger.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = badgerPrefix
		iter := txn.NewIterator(opts)
		defer iter.Close()

		for iter.Rewind(); iter.Valid(); iter.Next() {
			it := iter.Item()
			err := it.Value(func(v []byte) error {
				id := string(bytes.TrimPrefix(it.Key(), badgerPrefix))
				rawType := binary.LittleEndian.Uint64(v)
				p.cache[id] = smartblock.SmartBlockType(rawType)
				return nil
			})
			if err != nil {
				return fmt.Errorf("get value from key %s: %w", it.Key(), err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("init cache from badger: %w", err)
	}
	p.spaceService = app.MustComponent[spacecore.SpaceCoreService](a)
	return
}

func (p *provider) Name() (name string) {
	return CName
}

func (p *provider) Type(spaceID string, id string) (tp smartblock.SmartBlockType, err error) {
	tp, err = SmartblockTypeFromID(id)
	if err == nil && tp != smartblock.SmartBlockTypePage {
		return
	}
	return p.objectTypeFromSpace(spaceID, id)
}

func (p *provider) RegisterStaticType(id string, tp smartblock.SmartBlockType) {
	p.Lock()
	defer p.Unlock()
	p.cache[id] = tp
}

func SmartblockTypeFromID(id string) (smartblock.SmartBlockType, error) {
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

	if strings.HasPrefix(id, addr.MissingObject) {
		return smartblock.SmartBlockTypeMissingObject, nil
	}
	if strings.HasPrefix(id, addr.IdentityPrefix) {
		return smartblock.SmartBlockTypeIdentity, nil
	}

	c, err := cid.Decode(id)
	if err != nil {
		return smartblock.SmartBlockTypePage,
			fmt.Errorf("failed to determine smartblock type, objectID: %s, err: %s", id, err.Error())
	}
	// TODO: discard this fragile condition as soon as we will move to the multiaddr with prefix
	if c.Prefix().Codec == cid.DagProtobuf && c.Prefix().MhType == multihash.SHA2_256 {
		return smartblock.SmartBlockTypeFile, nil
	}
	if c.Prefix().Codec == cid.DagCBOR {
		return smartblock.SmartBlockTypePage, nil
	}

	return smartblock.SmartBlockTypePage, smartblock.ErrNoSuchSmartblock
}

func (p *provider) objectTypeFromSpace(spaceID string, id string) (tp smartblock.SmartBlockType, err error) {
	p.RLock()
	tp, exists := p.cache[id]
	if exists {
		p.RUnlock()
		return
	}
	p.RUnlock()

	sp, err := p.spaceService.Get(context.Background(), spaceID)
	if err != nil {
		return
	}
	store := sp.Storage()
	rawRoot, err := store.TreeRoot(id)
	if err != nil {
		return
	}

	tp, err = GetTypeFromRoot(rawRoot)
	if err != nil {
		return
	}
	err = p.setType(id, tp)
	if err != nil {
		return
	}
	return
}

func (p *provider) setType(id string, tp smartblock.SmartBlockType) (err error) {
	err = p.badger.Update(func(txn *badger.Txn) error {
		return txn.Set(append(badgerPrefix, []byte(id)...), binary.LittleEndian.AppendUint64(nil, uint64(tp)))
	})
	if err != nil {
		return fmt.Errorf("set type in badger: %w", err)
	}
	p.Lock()
	defer p.Unlock()
	p.cache[id] = tp
	return nil
}

func GetTypeFromRoot(rawRoot *treechangeproto.RawTreeChangeWithId) (smartblock.SmartBlockType, error) {
	root, err := unmarshallRoot(rawRoot)
	if err != nil {
		return 0, fmt.Errorf("unmarshall root: %w", err)
	}
	if root.ChangeType != spacecore.ChangeType {
		err = ErrUnknownChangeType
		return 0, err
	}
	payload, err := objectType(root.ChangePayload)
	if err != nil {
		return 0, fmt.Errorf("get object type: %w", err)
	}
	return smartblock.SmartBlockType(payload.SmartBlockType), nil
}

func unmarshallRoot(rawRoot *treechangeproto.RawTreeChangeWithId) (root *treechangeproto.RootChange, err error) {
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

func objectType(changePayload []byte) (payload *model.ObjectChangePayload, err error) {
	payload = &model.ObjectChangePayload{}
	err = proto.Unmarshal(changePayload, payload)
	return
}
