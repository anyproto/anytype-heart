//go:build gomobile || windows || nographviz || ignore || !cgo
// +build gomobile windows nographviz ignore !cgo

package dot

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/converter"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
)

func NewMultiConverter(format int, _ typeprovider.SmartBlockTypeProvider) converter.MultiConverter {
	return &dot{}
}

const (
	ExportFormatDOT = 0
	ExportFormatSVG = 1
)

type edgeType int

const (
	EdgeTypeRelation edgeType = iota
	EdgeTypeLink
)

type dot struct {
}

func (d *dot) SetKnownDocs(docs map[string]*types.Struct) converter.Converter {
	return d
}

func (d *dot) FileHashes() []string {
	return nil
}

func (d *dot) ImageHashes() []string {
	return nil
}

func (d *dot) Add(space smartblock.Space, st *state.State) error {
	return nil
}

func (d *dot) Convert(_ smartblock.SmartBlock) []byte {
	panic("not supported on windows")
	return nil
}

func (d *dot) Ext() string {
	return ""
}

func (d *dot) SetFileKeys(fileKeys *files.FileKeys) {}
