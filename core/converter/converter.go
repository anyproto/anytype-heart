package converter

import (
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type Converter interface {
	Convert(sbType model.SmartBlockType) (result []byte)
	SetKnownDocs(docs map[string]*domain.Details) Converter
	FileHashes() []string
	ImageHashes() []string
	Ext() string
}

type MultiConverter interface {
	Converter
	Add(space smartblock.Space, state *state.State, fetcher relationutils.RelationFormatFetcher) error
}
