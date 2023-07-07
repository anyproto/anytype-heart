package space

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/dgraph-io/badger/v3"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
)

type SmartBlockTypeProvider interface {
	Type(spaceID string, id string) (smartblock.SmartBlockType, error)
	RegisterStaticType(id string, tp smartblock.SmartBlockType)
}

type provider struct {
	sync.RWMutex
	badger       *badger.DB
	spaceService Service
	cache        map[string]smartblock.SmartBlockType
}

var badgerPrefix = []byte("typeprovider/")

func (p *provider) Init(a *app.App) (err error) {
	p.cache = map[string]smartblock.SmartBlockType{}
	p.badger, err = app.MustComponent[datastore.Datastore](a).SpaceStorage()
	if err != nil {
		return fmt.Errorf("get badger storage: %w", err)
	}
	err = p.badger.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = badgerPrefix
		iter := txn.NewIterator(opts)
		defer iter.Close()

		for iter.Rewind(); iter.Valid(); iter.Next() {
			it := iter.Item()
			err := it.Value(func(v []byte) error {
				// TODO Use helpers
				id := string(bytes.TrimPrefix(it.Key(), badgerPrefix))
				rawType := binary.LittleEndian.Uint64(v)
				p.cache[id] = smartblock.SmartBlockType(rawType)
				fmt.Println("init cache:", id, smartblock.SmartBlockType(rawType))
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
	return
}

func (p *provider) Name() (name string) {
	return CName
}

func (p *provider) Type(spaceID string, id string) (tp smartblock.SmartBlockType, err error) {
	tp, err = typeprovider.SmartblockTypeFromID(id)
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

func (p *provider) objectTypeFromSpace(spaceID string, id string) (tp smartblock.SmartBlockType, err error) {
	p.RLock()
	tp, exists := p.cache[id]
	if exists {
		p.RUnlock()
		return
	}
	p.RUnlock()

	sp, err := p.spaceService.GetSpace(context.Background(), spaceID)
	if err != nil {
		return
	}
	store := sp.Storage()
	rawRoot, err := store.TreeRoot(id)
	if err != nil {
		return
	}
	root, err := p.unmarshallRoot(rawRoot)
	if err != nil {
		return
	}
	if root.ChangeType != ChangeType {
		err = ErrUnknownChangeType
		return
	}
	payload, err := p.objectType(root.ChangePayload)
	if err != nil {
		return
	}
	err = p.setType(id, smartblock.SmartBlockType(payload.SmartBlockType))
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

func (p *provider) unmarshallRoot(rawRoot *treechangeproto.RawTreeChangeWithId) (root *treechangeproto.RootChange, err error) {
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

func (p *provider) objectType(changePayload []byte) (payload *model.ObjectChangePayload, err error) {
	payload = &model.ObjectChangePayload{}
	err = proto.Unmarshal(changePayload, payload)
	return
}

var (
	ErrUnknownChangeType = errors.New("error unknown change type")
)
