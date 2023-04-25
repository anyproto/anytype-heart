//go:build gomobile || windows || nographviz || ignore || !cgo
// +build gomobile windows nographviz ignore !cgo

package dot

import (
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
)

func NewMultiConverter(format int, sbtProvider typeprovider.SmartBlockTypeProvider) converter.MultiConverter {
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

func (d *dot) Add(st *state.State) error {
	return nil
}

func (d *dot) Convert() []byte {
	panic("not supported on windows")
	return nil
}

func (d *dot) Ext() string {
	return ""
}
