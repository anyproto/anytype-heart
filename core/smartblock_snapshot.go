package core

import (
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	cbornode "github.com/ipfs/go-ipld-cbor"
)

type smartBlockSnapshot struct {
	model     *storage.SmartBlockWithMeta
	state     SmartBlockState
	user      string
	date      *types.Timestamp
	node      *Anytype
}

type smartBlockSnapshotMeta struct {
	model     *storage.BlockMetaOnly
	user      string
	date      *types.Timestamp
	node      *Anytype
}

func (snapshot smartBlockSnapshot) State() SmartBlockState {
	return snapshot.state
}

func (snapshot smartBlockSnapshot) Creator() (string, error) {
	return snapshot.user, nil
}

func (snapshot smartBlockSnapshot) CreatedDate() *time.Time {
	return nil
}

func (snapshot smartBlockSnapshot) ReceivedDate()  *time.Time {
	return nil
}

func (snapshot smartBlockSnapshot) Blocks() ([]*model.Block, error) {
	// todo: blocks lazy loading
	return snapshot.model.Blocks, nil
}

func (snapshot smartBlockSnapshot) Meta() (*SmartBlockMeta, error) {
	return nil, fmt.Errorf("not implemented")
}

func (snapshot smartBlockSnapshotMeta) User() string {
	return snapshot.user
}

func (snapshot smartBlockSnapshotMeta) Date() *types.Timestamp {
	return snapshot.date
}

type threadSnapshot struct {
	Data []byte
}

func init() {
	cbornode.RegisterCborType(threadSnapshot{})
}

func (s *threadSnapshot) BlockWithMeta() (*storage.SmartBlockWithMeta, error) {
	var blockWithMeta storage.SmartBlockWithMeta
	err := proto.Unmarshal(s.Data, &blockWithMeta)
	if err != nil {
		return nil, err
	}

	return &blockWithMeta, nil
}

func (s *threadSnapshot) BlockMetaOnly() (*storage.BlockMetaOnly, error) {
	var blockWithMeta storage.BlockMetaOnly
	err := proto.Unmarshal(s.Data, &blockWithMeta)
	if err != nil {
		return nil, err
	}

	return &blockWithMeta, nil
}
