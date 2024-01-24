package converter

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/files"
)

type Converter interface {
	Convert(sbType smartblock.SmartBlock) (result []byte)
	SetKnownDocs(docs map[string]*types.Struct) Converter
	FileHashes() []string
	SetFileKeys(fileKeys *files.FileKeys)
	ImageHashes() []string
	Ext() string
}

type MultiConverter interface {
	Converter
	Add(space smartblock.Space, state *state.State) error
}
