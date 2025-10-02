package typeprovider

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/globalsign/mgo/bson"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/anystorage"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
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

	store keyvaluestore.Store[smartblock.SmartBlockType]
	cache map[string]smartblock.SmartBlockType
}

func New() SmartBlockTypeProvider {
	return &provider{}
}

type DbProvider interface {
	GetCommonDb() anystore.DB
}

func (p *provider) Init(a *app.App) (err error) {
	p.cache = map[string]smartblock.SmartBlockType{}

	dbProvider := app.MustComponent[DbProvider](a)

	store, err := keyvaluestore.New(dbProvider.GetCommonDb(), "smartblock_types", func(tp smartblock.SmartBlockType) ([]byte, error) {
		raw := binary.LittleEndian.AppendUint64(nil, uint64(tp))
		return raw, nil
	}, func(raw []byte) (smartblock.SmartBlockType, error) {
		return smartblock.SmartBlockType(binary.LittleEndian.Uint64(raw)), nil
	})
	if err != nil {
		return fmt.Errorf("init store: %w", err)
	}

	iter := store.Iterator(context.Background())
	for k, v := range iter.All() {
		p.cache[k] = v
	}
	err = iter.Err()
	if err != nil {
		return fmt.Errorf("warm-up cache: %w", err)
	}

	p.store = store
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
	if strings.HasPrefix(id, domain.ParticipantPrefix) {
		return smartblock.SmartBlockTypeParticipant, nil
	}

	c, err := cid.Decode(id)
	if err != nil {
		return smartblock.SmartBlockTypePage,
			fmt.Errorf("failed to determine smartblock type, objectID: %s, err: %w", id, err)
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
	ctx := context.Background()
	sp, err := p.spaceService.Get(ctx, spaceID)
	if err != nil {
		return
	}
	rawRoot, err := sp.Storage().(anystorage.ClientSpaceStorage).TreeRoot(ctx, id)
	if err != nil {
		return
	}
	tp, _, err = GetTypeAndKeyFromRoot(rawRoot)
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
	err = p.store.Set(context.Background(), id, tp)
	if err != nil {
		return fmt.Errorf("set in store: %w", err)
	}

	p.Lock()
	defer p.Unlock()
	p.cache[id] = tp
	return nil
}

func GetTypeAndKeyFromRootChange(root *treechangeproto.RootChange) (sbt smartblock.SmartBlockType, key string, err error) {
	if root.ChangeType != spacedomain.ChangeType {
		err = ErrUnknownChangeType
		return 0, "", err
	}
	payload, err := objectType(root.ChangePayload)
	if err != nil {
		return 0, "", fmt.Errorf("get object type: %w", err)
	}
	return smartblock.SmartBlockType(payload.SmartBlockType), payload.Key, nil
}

func GetTypeAndKeyFromRoot(rawRoot *treechangeproto.RawTreeChangeWithId) (sbt smartblock.SmartBlockType, key string, err error) {
	root, err := unmarshallRoot(rawRoot)
	if err != nil {
		return 0, "", fmt.Errorf("unmarshall root: %w", err)
	}

	return GetTypeAndKeyFromRootChange(root)
}

func unmarshallRoot(rawRoot *treechangeproto.RawTreeChangeWithId) (root *treechangeproto.RootChange, err error) {
	raw := &treechangeproto.RawTreeChange{}
	err = raw.UnmarshalVT(rawRoot.GetRawChange())
	if err != nil {
		return
	}

	root = &treechangeproto.RootChange{}
	err = root.UnmarshalVT(raw.Payload)
	if err != nil {
		return
	}
	return
}

func objectType(changePayload []byte) (payload *model.ObjectChangePayload, err error) {
	payload = &model.ObjectChangePayload{}
	err = payload.Unmarshal(changePayload)
	return
}
