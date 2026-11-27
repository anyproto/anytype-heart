package objecthandler

import (
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

// LastModifiedInfo describes the user change a handler picked as the object's
// latest. WHICH change is picked is unchanged, long-standing behavior (the
// greatest timestamp; of equals, the first met) and out of scope for
// attribution. What matters here is that every member is read off that ONE
// change: who signed it, when, and what the change says about its own
// authoring (per-change attributes such as the local-API app name — a new
// attribute is one more member).
type LastModifiedInfo struct {
	Timestamp int64
	// Account is the identity that signed the change; "" when no change was
	// picked.
	Account string
	// IntegrationName is pb.Change.IntegrationName of the change: the RAW
	// name of the local-API app that authored it, or "" when it was made in
	// an Anytype app.
	IntegrationName string
}

type SmartblockHandler interface {
	CollectLastModifiedInfo(change *objecttree.Change)
	GetLastModifiedInfo() LastModifiedInfo
}

func GetSmartblockHandler(sbt smartblock.SmartBlockType) SmartblockHandler {
	switch sbt {
	case smartblock.SmartBlockTypeFileObject:
		return &filesHandler{}
	default:
		return &defaultHandler{}
	}
}

// infoFromChange reads the whole tuple off one change.
func infoFromChange(change *objecttree.Change) LastModifiedInfo {
	info := LastModifiedInfo{Timestamp: change.Timestamp, Account: change.Identity.Account()}
	if model, ok := change.Model.(*pb.Change); ok && model != nil {
		info.IntegrationName = model.IntegrationName
	}
	return info
}

type defaultHandler struct {
	info LastModifiedInfo
}

func (d *defaultHandler) CollectLastModifiedInfo(change *objecttree.Change) {
	if change.Timestamp > d.info.Timestamp {
		d.info = infoFromChange(change)
	}
}

func (d *defaultHandler) GetLastModifiedInfo() LastModifiedInfo {
	return d.info
}

type filesHandler struct {
	info LastModifiedInfo
}

func (f *filesHandler) CollectLastModifiedInfo(change *objecttree.Change) {
	if change.Timestamp <= f.info.Timestamp {
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

	f.info = infoFromChange(change)
}

func (f *filesHandler) GetLastModifiedInfo() LastModifiedInfo {
	return f.info
}
