package core

import (
	"context"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	cbornode "github.com/ipfs/go-ipld-cbor"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

const (
	snippetMinSize = 50
	snippetMaxSize = 300
)

type ProfileThreadEncryptionKeys struct {
	ServiceKey []byte
	ReadKey    []byte
}

func init() {
	cbornode.RegisterCborType(ProfileThreadEncryptionKeys{})
}

type SmartBlockMeta struct {
	ObjectTypes   []string
	RelationLinks []*model.RelationLink
	Details       *types.Struct
}

type SmartBlock interface {
	ID() string
	Type() smartblock.SmartBlockType
	Creator() (string, error)

	GetLogs() ([]SmartblockLog, error)
	GetRecord(ctx context.Context, recordID string) (*SmartblockRecordEnvelope, error)
	PushRecord(payload proto.Marshaler) (id string, err error)

	SubscribeForRecords(ch chan SmartblockRecordEnvelope) (cancel func(), err error)
	// SubscribeClientEvents provide a way to subscribe for the client-side events e.g. carriage position change
	SubscribeClientEvents(event chan<- proto.Message) (cancelFunc func(), err error)
	// PublishClientEvent gives a way to push the new client-side event e.g. carriage position change
	// notice that you will also get this event in SubscribeForEvents
	PublishClientEvent(event proto.Message) error
}
