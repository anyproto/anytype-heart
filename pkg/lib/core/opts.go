package core

import (
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type ServiceOption func(*ServiceOptions) error
type ServiceOptions struct {
	SnapshotMarshalerFunc func(blocks []*model.Block, details *types.Struct, relations []*model.Relation, objectTypes []string, fileKeys []*files.FileKeys) proto.Marshaler
	NewSmartblockChan     chan string
}
