package converter

import (
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type Converter interface {
	Convert(sbType model.SmartBlockType, id string) (result []byte)
	SetKnownDocs(docs map[string]*types.Struct) Converter
	FileHashes() []string
	ImageHashes() []string
	Ext() string
}

type MultiConverter interface {
	Converter
	Add(state *state.State) error
}
