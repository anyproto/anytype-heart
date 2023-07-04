package objectstore

import (
	ds "github.com/ipfs/go-datastore"

	"github.com/anyproto/anytype-heart/pb"
)

func (d *dsObjectStore) SaveState(hash string, csh *pb.ChangeSnapshot) error {
	return setValue(d.db, currentState.Child(ds.NewKey(hash)).Bytes(), csh)
}

func (d *dsObjectStore) GetState(hash string) (*pb.ChangeSnapshot, error) {
	return getValue(d.db, currentState.Child(ds.NewKey(hash)).Bytes(), bytesToSnapshots)
}

func (d *dsObjectStore) DeleteState(hash string) error {
	return deleteValue(d.db, currentState.Child(ds.NewKey(hash)).Bytes())
}

func bytesToSnapshots(bytes []byte) (*pb.ChangeSnapshot, error) {
	var csn pb.ChangeSnapshot
	err := csn.Unmarshal(bytes)
	if err != nil {
		log.Errorf("GetState unmarshall error: %v", err)
		return nil, err
	}

	return &csn, nil
}
