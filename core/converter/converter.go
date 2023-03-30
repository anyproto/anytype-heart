package converter

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/gogo/protobuf/types"
)

type Converter interface {
	Convert(model.SmartBlockType) (result []byte)
	SetKnownDocs(docs map[string]*types.Struct) Converter
	FileHashes() []string
	ImageHashes() []string
	Ext() string
}

type MultiConverter interface {
	Converter
	Add(state *state.State) error
}
