package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree/updatelistener"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

var _ updatelistener.UpdateListener = (*store)(nil)

type Store interface {
	ReadStoreDoc(ctx context.Context, stateStore *storestate.StoreState) (err error)
	PushStoreChange(ctx context.Context, params PushStoreChangeParams) (changeId string, err error)
}

type PushStoreChangeParams struct {
	State      *storestate.StoreState
	Changes    []*pb.StoreChangeContent
	Time       time.Time // used to derive the lastModifiedDate; Default is time.Now()
	DoSnapshot bool
}

type store struct {
	*source
	store *storestate.StoreState
}

func (s *store) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (s *store) ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	// Fake state, this kind of objects not support state operations

	st := state.NewDoc(s.id, nil).(*state.State)
	// Set object type here in order to derive value of Type relation in smartblock.Init
	st.SetObjectTypeKey(bundle.TypeKeyChat)
	return st, nil
}

func (s *store) PushChange(params PushChangeParams) (id string, err error) {
	return "", nil
}

func (s *store) ReadStoreDoc(ctx context.Context, storeState *storestate.StoreState) (err error) {
	s.store = storeState

	tx, err := s.store.NewTx(ctx)
	if err != nil {
		return
	}
	applier := &storeApply{
		tx: tx,
		ot: s.ObjectTree,
	}
	if err = applier.Apply(); err != nil {
		return errors.Join(tx.Rollback(), err)
	}
	return tx.Commit()
}

func (s *store) PushStoreChange(ctx context.Context, params PushStoreChangeParams) (changeId string, err error) {
	tx, err := s.store.NewTx(ctx)
	if err != nil {
		return "", fmt.Errorf("new tx: %w", err)
	}
	rollback := func(err error) error {
		return errors.Join(tx.Rollback(), err)
	}

	change := &pb.StoreChange{
		ChangeSet: params.Changes,
	}
	data, dataType, err := MarshalStoreChange(change)
	if err != nil {
		return "", fmt.Errorf("marshal change: %w", err)
	}
	addResult, err := s.ObjectTree.AddContentWithValidator(ctx, objecttree.SignableChangeContent{
		Data:        data,
		Key:         s.accountKeysService.Account().SignKey,
		IsSnapshot:  params.DoSnapshot,
		IsEncrypted: true,
		DataType:    dataType,
		Timestamp:   params.Time.Unix(),
	}, func(change *treechangeproto.RawTreeChangeWithId) error {
		order := tx.NextOrder(tx.GetMaxOrder())
		err = tx.ApplyChangeSet(storestate.ChangeSet{
			Id:      change.Id,
			Order:   order,
			Changes: params.Changes,
		})
		if err != nil {
			return fmt.Errorf("apply change set: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", rollback(fmt.Errorf("add content: %w", err))
	}

	if len(addResult.Added) == 0 {
		return "", rollback(fmt.Errorf("add changes list is empty"))
	}
	return addResult.Added[0].Id, tx.Commit()
}

func (s *store) update(ctx context.Context, tree objecttree.ObjectTree) error {
	tx, err := s.store.NewTx(ctx)
	if err != nil {
		return err
	}
	applier := &storeApply{
		tx: tx,
		ot: tree,
	}
	if err = applier.Apply(); err != nil {
		return errors.Join(tx.Rollback(), err)
	}
	return tx.Commit()
}

func (s *store) Update(tree objecttree.ObjectTree) {
	err := s.update(context.Background(), tree)
	if err != nil {
		log.With("objectId", s.id).Errorf("update: failed to read store doc: %v", err)
	}
}

func (s *store) Rebuild(tree objecttree.ObjectTree) {
	err := s.update(context.Background(), tree)
	if err != nil {
		log.With("objectId", s.id).Errorf("rebuild: failed to read store doc: %v", err)
	}
}

func MarshalStoreChange(change *pb.StoreChange) (result []byte, dataType string, err error) {
	data := bytesPool.Get().([]byte)[:0]
	defer bytesPool.Put(data)

	data = slices.Grow(data, change.Size())
	n, err := change.MarshalTo(data)
	if err != nil {
		return
	}
	data = data[:n]

	if n > snappyLowerLimit {
		result = snappy.Encode(nil, data)
		dataType = dataTypeSnappy
	} else {
		result = bytes.Clone(data)
	}

	return
}

func UnmarshalStoreChange(treeChange *objecttree.Change, data []byte) (result any, err error) {
	change := &pb.StoreChange{}
	if treeChange.DataType == dataTypeSnappy {
		buf := bytesPool.Get().([]byte)[:0]
		defer bytesPool.Put(buf)

		var n int
		if n, err = snappy.DecodedLen(data); err == nil {
			buf = slices.Grow(buf, n)[:n]
			var decoded []byte
			decoded, err = snappy.Decode(buf, data)
			if err == nil {
				data = decoded
			}
		}
	}
	if err = proto.Unmarshal(data, change); err == nil {
		result = change
	}
	return
}
