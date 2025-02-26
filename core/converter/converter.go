package converter

import (
	"io"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type Converter interface {
	Convert(st *state.State, sbType model.SmartBlockType, filename string) (result []byte)
	SetKnownDocs(docs map[string]*domain.Details)
	FileHashes() []string
	ImageHashes() []string
	Ext() string
}

type FileWriter interface {
	WriteFile(filename string, r io.Reader, lastModifiedDate int64) (err error)
}

type Flusher interface {
	Flush(fw FileWriter) error
}

type MultiConverter interface {
	Converter
	Add(space smartblock.Space, state *state.State) error
}
