package simple

import (
	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
	Diff(spaceId string, block Block) (msgs []EventMessage, err error)
	String() string
	Copy() Block
	Validate() error
}

type LinkedFilesIterator interface {
	IterateLinkedFiles(func(id string))
}

type FileHashes interface {
	FillFileHashes(hashes []string) []string // DEPRECATED, use only for migration and backward compatibility purposes
}

type DetailsService interface {
	Details() *domain.Details
	SetDetail(key domain.RelationKey, value domain.Value)
}

type DetailsHandler interface {
	// will call after block create and for every details change
	DetailsInit(s DetailsService)
	// will call for applying block data to details
	ApplyToDetails(prev Block, s DetailsService) (ok bool, err error)
}

type ObjectLinkReplacer interface {
	ReplaceLinkIds(replacer func(oldId string) (newId string))
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
