package chmodel

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
)

type Type int

const (
	TypeAdd Type = iota
	TypeMove
	TypeUpdate
)

type Change struct {
	Type  Type
	Value interface{}
}

type ChangeValueBlockPosition struct {
	Blocks   []*model.Block
	BlockIds []string
	TargetId string
	Position model.BlockPosition
}