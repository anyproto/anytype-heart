package pb

import (
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type SpaceImport struct {
	service *collection.Service
}

func NewSpaceImport(service *collection.Service) *SpaceImport {
	return &SpaceImport{service: service}
}

func (s *SpaceImport) ProvideCollection(
	snapshots *snapshotSet,
	oldToNewID map[string]string,
	params *pb.RpcObjectImportRequestPbParams,
	_ bool,
) ([]*common.Snapshot, error) {
	if params.GetNoCollection() {
		return nil, nil
	}
	if snapshots == nil {
		snapshots = &snapshotSet{List: []*common.Snapshot{}}
	}
	var (
		rootObjects        []string
		widgetFlags        widget.ImportWidgetFlags
		objectsNotInWidget []*common.Snapshot
	)

	if snapshots.Widget != nil {
		widgetFlags, rootObjects = s.getObjectsFromWidget(snapshots.Widget, oldToNewID)
		objectsNotInWidget = lo.Filter(snapshots.List, func(item *common.Snapshot, index int) bool {
			return !lo.Contains(rootObjects, item.Id)
		})
	}
	if !widgetFlags.IsEmpty() || len(rootObjects) > 0 {
		// add to root collection only objects from widget, dashboard and favorites
		rootObjects = append(rootObjects, s.filterObjects(widgetFlags, objectsNotInWidget)...)
	} else {
		// if we don't have any widget, we add everything (except sub objects and templates) to root collection
		rootObjects = lo.FilterMap(snapshots.List, func(item *common.Snapshot, index int) (string, bool) {
			if !s.objectShouldBeSkipped(item) {
				return item.Id, true
			}
			return item.Id, false
		})
	}
	rootCollection := common.NewRootCollection(s.service)
	rootCollectionSnapshot, err := rootCollection.MakeRootCollection(rootCollectionName, rootObjects, "", nil, true, true)
	if err != nil {
		return nil, err
	}
	return []*common.Snapshot{rootCollectionSnapshot}, nil
}

func (s *SpaceImport) objectShouldBeSkipped(item *common.Snapshot) bool {
	return item.SbType == smartblock.SmartBlockTypeSubObject || item.SbType == smartblock.SmartBlockTypeTemplate ||
		item.SbType == smartblock.SmartBlockTypeRelation || item.SbType == smartblock.SmartBlockTypeObjectType ||
		item.SbType == smartblock.SmartBlockTypeRelationOption
}

func (s *SpaceImport) getObjectsFromWidget(widgetSnapshot *common.Snapshot, oldToNewID map[string]string) (widget.ImportWidgetFlags, []string) {
	widgetState := state.NewDocFromSnapshot("", widgetSnapshot.Snapshot).(*state.State)
	var (
		objectsInWidget     []string
		objectTypesToImport widget.ImportWidgetFlags
	)
	err := widgetState.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil && link.TargetBlockId != "" {
			if builtinWidget := widget.FillImportFlags(link, &objectTypesToImport); builtinWidget {
				return true
			}
			if newID, objectExist := oldToNewID[link.TargetBlockId]; objectExist {
				objectsInWidget = append(objectsInWidget, newID)
			}
		}
		return true
	})
	if err != nil {
		return widget.ImportWidgetFlags{}, nil
	}
	return objectTypesToImport, objectsInWidget
}

func (s *SpaceImport) filterObjects(objectTypesToImport widget.ImportWidgetFlags, objectsNotInWidget []*common.Snapshot) []string {
	var rootObjects []string
	for _, snapshot := range objectsNotInWidget {
		if s.objectShouldBeSkipped(snapshot) {
			continue
		}
		if objectTypesToImport.ImportCollection && lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.URL()) {
			rootObjects = append(rootObjects, snapshot.Id)
			continue
		}
		if objectTypesToImport.ImportSet && lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeySet.URL()) {
			rootObjects = append(rootObjects, snapshot.Id)
			continue
		}
		if pbtypes.GetBool(snapshot.Snapshot.Data.Details, bundle.RelationKeyIsFavorite.String()) {
			rootObjects = append(rootObjects, snapshot.Id)
			continue
		}
		if spaceDashboardID := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeySpaceDashboardId.String()); spaceDashboardID != "" {
			rootObjects = append(rootObjects, spaceDashboardID)
			continue
		}
	}
	return rootObjects
}
