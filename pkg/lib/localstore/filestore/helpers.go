package filestore

import (
	"github.com/dgraph-io/badger/v3"
	"github.com/gogo/protobuf/proto"
	dsCtx "github.com/ipfs/go-datastore"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

func (m *dsFileStore) updateTxn(f func(txn *badger.Txn) error) error {
	return badgerhelper.RetryOnConflict(func() error {
		return m.db.Update(f)
	})
}

func (m *dsFileStore) getInt(key dsCtx.Key) (int, error) {
	val, err := badgerhelper.GetValue(m.db, key.Bytes(), badgerhelper.UnmarshalInt)
	if badgerhelper.IsNotFound(err) {
		return 0, localstore.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return val, nil
}

func (m *dsFileStore) setInt(key dsCtx.Key, val int) error {
	return m.updateTxn(func(txn *badger.Txn) error {
		return badgerhelper.SetValueTxn(txn, key.Bytes(), val)
	})
}

func unmarshalFileInfo(raw []byte) (*storage.FileInfo, error) {
	v := &storage.FileInfo{}
	return v, proto.Unmarshal(raw, v)
}
