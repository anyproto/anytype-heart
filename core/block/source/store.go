package source

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree/updatelistener"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pb"
)

var _ updatelistener.UpdateListener = (*store)(nil)

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
	return nil, fmt.Errorf("not supported")
}

func (s *store) PushChange(params PushChangeParams) (id string, err error) {
	return "", fmt.Errorf("not supported")
}

func (s *store) ReadStoreDoc(ctx context.Context, store *storestate.StoreState) (err error) {
	tx, err := store.NewTx(ctx)
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
	return
}

func (s *store) Update(tree objecttree.ObjectTree) {

}

func (s *store) Rebuild(tree objecttree.ObjectTree) {

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
