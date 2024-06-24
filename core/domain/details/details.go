package details

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
)

type Value struct {
	kind     uint8
	bool     bool
	floats   []float64
	strings  []string
	string   string
	float    float64
	abool    int
	aboo2l   int
	aboo3l   int
	keks     []int
	astrings []string
}

type valueType uint8

const (
	valueTypeNone valueType = iota
	valueTypeBool
	valueTypeString
	valueTypeFloat
	valueTypeStrings
	valueTypeFloats
)

type Details struct {
	data map[domain.RelationKey]Value
}

func (d *Details) Set(key domain.RelationKey, val Value) {
}

type ODetails struct {
	data map[domain.RelationKey]*types.Value
}

type IDetails struct {
	data map[domain.RelationKey]any
}
