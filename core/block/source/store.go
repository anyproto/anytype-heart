package source

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree/updatelistener"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

var _ updatelistener.UpdateListener = (*store)(nil)

type Store interface {
	GetStore() *storestate.StoreState
	ReadStoreDoc(ctx context.Context) (err error)
	PushStoreChange(params PushStoreChangeParams) (id string, err error)
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
	st.SetObjectTypeKey(bundle.TypeKeyParticipant)
	return st, nil
}

func (s *store) PushChange(params PushChangeParams) (id string, err error) {
	return "", nil
}

func (s *store) GetStore() *storestate.StoreState {
	return s.store
}

func (s *store) ReadStoreDoc(ctx context.Context) (err error) {
	tx, err := s.store.NewTx(ctx)
	if err != nil {
		return
	}
	applier := &storeApply{
		tx: tx,
		ot: s.ObjectTree,
	}
	if err = applier.Apply(); err != nil {
		_ = tx.Rollback()
		return
	}
	return tx.Commit()
}

func (s *store) PushStoreChange(params PushStoreChangeParams) (id string, err error) {
	change := &pb.StoreChange{
		ChangeSet: params.Changes,
	}
	data, dataType, err := MarshalStoreChange(change)
	if err != nil {
		return
	}
	_, err = s.ObjectTree.AddContent(context.Background(), objecttree.SignableChangeContent{
		Data:        data,
		Key:         s.accountKeysService.Account().SignKey,
		IsSnapshot:  params.DoSnapshot,
		IsEncrypted: true,
		DataType:    dataType,
		Timestamp:   params.Time.Unix(),
	})
	if err != nil {
		return
	}
	return "", nil
}

func (s *store) Update(tree objecttree.ObjectTree) {
	// TODO !!!
}

func (s *store) Rebuild(tree objecttree.ObjectTree) {
	// TODO !!!
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
