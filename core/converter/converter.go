package converter

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/gogo/protobuf/types"
)

type Converter interface {
	Convert() (result []byte)
	SetKnownDocs(docs map[string]*types.Struct) Converter
	FileHashes() []string
	ImageHashes() []string
	Ext() string
}

type MultiConverter interface {
	Converter
	Add(state *state.State) error
}
