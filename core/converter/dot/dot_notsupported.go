// +build gomobile windows nographviz

package dot

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
)

func NewMultiConverter(format int) converter.MultiConverter {
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

func (d *dot) SetKnownLinks(ids []string) converter.Converter {
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
