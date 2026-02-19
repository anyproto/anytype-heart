package objecthandler

import (
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

type SmartblockHandler interface {
	CollectLastModifiedInfo(change *objecttree.Change)
	GetLastModifiedInfo() (lastModified int64, lastModifiedBy string)
}

func GetSmartblockHandler(sbt smartblock.SmartBlockType) SmartblockHandler {
	switch sbt {
	case smartblock.SmartBlockTypeFileObject:
		return &filesHandler{}
	default:
		return &defaultHandler{}
	}
}

type defaultHandler struct {
	lastModified   int64
	lastModifiedBy string
}

func (d *defaultHandler) CollectLastModifiedInfo(change *objecttree.Change) {
	if change.Timestamp > d.lastModified {
		d.lastModified = change.Timestamp
		d.lastModifiedBy = change.Identity.Account()
	}
}

func (d *defaultHandler) GetLastModifiedInfo() (lastModified int64, lastModifiedBy string) {
	return d.lastModified, d.lastModifiedBy
}

type filesHandler struct {
	lastModified   int64
	lastModifiedBy string
}

func (f *filesHandler) CollectLastModifiedInfo(change *objecttree.Change) {
	if change.Timestamp <= f.lastModified {
		return
	}

	model := change.Model.(*pb.Change)
	if model.Snapshot == nil {
		for _, cnt := range model.Content {
			if set := cnt.GetDetailsSet(); set != nil && set.Key == bundle.RelationKeyFileVariantIds.String() {
				return
			}
		}
	}

	f.lastModified = change.Timestamp
	f.lastModifiedBy = change.Identity.Account()
}

func (f *filesHandler) GetLastModifiedInfo() (lastModified int64, lastModifiedBy string) {
	return f.lastModified, f.lastModifiedBy
}
