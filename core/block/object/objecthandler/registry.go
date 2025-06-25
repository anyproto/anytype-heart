package objecthandler

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

type SmartblockHandler interface {
	SkipChangeToSetLastModifiedDate(model *pb.Change) bool
}

var handlersBySmartblockType = map[smartblock.SmartBlockType]SmartblockHandler{
	smartblock.SmartBlockTypeFileObject: &filesHandler{},
}

func GetSmartblockHandler(sbt smartblock.SmartBlockType) SmartblockHandler {
	if handler, ok := handlersBySmartblockType[sbt]; ok {
		return handler
	}
	return &defaultHandler{}
}

type defaultHandler struct {
}

func (d *defaultHandler) SkipChangeToSetLastModifiedDate(model *pb.Change) bool {
	return false
}

type filesHandler struct {
}

func (f *filesHandler) SkipChangeToSetLastModifiedDate(model *pb.Change) bool {
	if model.Snapshot != nil {
		return false
	}
	for _, cnt := range model.Content {
		if set := cnt.GetDetailsSet(); set != nil {
			if set.Key == bundle.RelationKeyFileVariantIds.String() {
				return true
			}
		}
	}
	return false
}
