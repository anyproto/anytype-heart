package simple

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
)

type BlockCreator = func(m *model.Block) Block

var (
	registry []BlockCreator
	fallback BlockCreator
)

func RegisterCreator(c BlockCreator) {
	registry = append(registry, c)
}

func RegisterFallback(c BlockCreator) {
	fallback = c
}

type Block interface {
	Model() *model.Block
	ModelToSave() *model.Block
	Diff(block Block) (msgs []EventMessage, err error)
	String() string
	Copy() Block
}

type FileHashes interface {
	FillFileHashes(hashes []string) []string
}

type DetailsService interface {
	Details() *types.Struct
	SetDetail(key string, value *types.Value)
}

type DetailsHandler interface {
	// will call after block create
	DetailsInit(s DetailsService)
	// will call for every details change
	OnDetailsChange(prev Block, s DetailsService) (msgs []EventMessage, err error)
	// will call for applying block data to details
	ApplyToDetails(prev Block, s DetailsService) (msgs []EventMessage, err error)
}

type EventMessage struct {
	Virtual bool
	Msg     *pb.EventMessage
}

func New(block *model.Block) (b Block) {
	if block.Id == "" {
		block.Id = bson.NewObjectId().Hex()
	}
	for _, c := range registry {
		if b = c(block); b != nil {
			return
		}
	}
	return fallback(block)
}
