package pb

import (
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	widgets "github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
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

func (s *SpaceImport) ProvideCollection(snapshots []*converter.Snapshot, widget *converter.Snapshot, oldToNewID map[string]string, params *pb.RpcObjectImportRequestPbParams, workspaceSnapshot *converter.Snapshot) (*converter.Snapshot, error) {
	if params.GetNoCollection() {
		return nil, nil
	}
	var (
		rootObjects         []string
		widgetFlags         widgets.ImportWidgetFlags
		objectsNotInWidgets []*converter.Snapshot
	)

	if widget != nil {
		widgetFlags, rootObjects = s.getObjectsFromWidgets(widget, oldToNewID)
		objectsNotInWidgets = lo.Filter(snapshots, func(item *converter.Snapshot, index int) bool {
			return !lo.Contains(rootObjects, item.Id)
		})
	}
	if !widgetFlags.IsEmpty() || len(rootObjects) > 0 {
		// add to root collection only objects from widgets, dashboard and favorites
		rootObjects = append(rootObjects, s.filterObjects(widgetFlags, objectsNotInWidgets)...)
	} else {
		// if we don't have any widget, we add everything (except sub objects and templates) to root collection
		rootObjects = lo.FilterMap(snapshots, func(item *converter.Snapshot, index int) (string, bool) {
			if !s.objectShouldBeSkipped(item) {
				return item.Id, true
			}
			return item.Id, false
		})
	}
	rootCollection := converter.NewRootCollection(s.service)
	return rootCollection.MakeRootCollection(rootCollectionName, rootObjects, "", nil)
}

func (s *SpaceImport) objectShouldBeSkipped(item *converter.Snapshot) bool {
	return item.SbType == smartblock.SmartBlockTypeSubObject || item.SbType == smartblock.SmartBlockTypeTemplate ||
		item.SbType == smartblock.SmartBlockTypeRelation || item.SbType == smartblock.SmartBlockTypeObjectType ||
		item.SbType == smartblock.SmartBlockTypeRelationOption
}

func (s *SpaceImport) getObjectsFromWidgets(widgetSnapshot *converter.Snapshot, oldToNewID map[string]string) (widgets.ImportWidgetFlags, []string) {
	widgetState := state.NewDocFromSnapshot("", widgetSnapshot.Snapshot).(*state.State)
	var (
		objectsInWidget     []string
		objectTypesToImport widgets.ImportWidgetFlags
	)
	err := widgetState.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil && link.TargetBlockId != "" {
			if builtinWidget := widgets.FillImportFlags(link, &objectTypesToImport); builtinWidget {
				return true
			}
			if newID, objectExist := oldToNewID[link.TargetBlockId]; objectExist {
				objectsInWidget = append(objectsInWidget, newID)
			}
		}
		return true
	})
	if err != nil {
		return widgets.ImportWidgetFlags{}, nil
	}
	return objectTypesToImport, objectsInWidget
}

func (s *SpaceImport) filterObjects(objectTypesToImport widgets.ImportWidgetFlags, objectsNotInWidget []*converter.Snapshot) []string {
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
