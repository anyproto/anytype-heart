package service

import (
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/gogo/protobuf/types"
)

type Filter struct {
	RelationKey string
	Condition   model.BlockContentDataviewFilterCondition
	Value       *types.Value
}
