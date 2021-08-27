package converter

import "github.com/anytypeio/go-anytype-middleware/core/block/editor/state"

type Converter interface {
	Convert() (result []byte)
	SetKnownLinks(ids []string) Converter
	FileHashes() []string
	ImageHashes() []string
	Ext() string
}

type MultiConverter interface {
	Converter
	Add(state *state.State) error
}
