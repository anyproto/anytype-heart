package core

import (
	"time"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/anytypeio/go-anytype-library/vclock"
	"github.com/gogo/protobuf/types"
)

type SmartBlockSnapshot interface {
	State() vclock.VClock
	Creator() (string, error)
	CreatedDate() *time.Time
	ReceivedDate() *time.Time
	Blocks() ([]*model.Block, error)
	Meta() (*SmartBlockMeta, error)
}

type smartBlockSnapshot struct {
	blocks     []*model.Block               `protobuf:"bytes,2,rep,name=blocks,proto3" json:"blocks,omitempty"`
	details    *types.Struct                `protobuf:"bytes,3,opt,name=details,proto3" json:"details,omitempty"`
	keysByHash map[string]*storage.FileKeys `protobuf:"bytes,4,rep,name=keysByHash,proto3" json:"keysByHash,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	state      vclock.VClock

	creator string
	date    *types.Timestamp
	node    *Anytype
}

func (snapshot smartBlockSnapshot) State() vclock.VClock {
	return snapshot.state
}

func (snapshot smartBlockSnapshot) Creator() (string, error) {
	return snapshot.creator, nil
}

func (snapshot smartBlockSnapshot) CreatedDate() *time.Time {
	return nil
}

func (snapshot smartBlockSnapshot) ReceivedDate() *time.Time {
	return nil
}

func (snapshot smartBlockSnapshot) Blocks() ([]*model.Block, error) {
	// todo: blocks lazy loading
	return snapshot.blocks, nil
}

func (snapshot smartBlockSnapshot) Meta() (*SmartBlockMeta, error) {
	return &SmartBlockMeta{Details: snapshot.details}, nil
}
