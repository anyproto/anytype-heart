package pb

import (
	"fmt"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

type SpaceImport struct {
	service *collection.Service
}

func NewSpaceImport(service *collection.Service) *SpaceImport {
	return &SpaceImport{service: service}
}

func (s *SpaceImport) ProvideCollection(
	snapshots *common.SnapshotContext,
	oldToNewID map[string]string,
	params *pb.RpcObjectImportRequestPbParams,
	_ bool,
) ([]*common.Snapshot, error) {
	if params.GetNoCollection() {
		return nil, nil
	}
	var (
		rootObjects        []string
		widgetFlags        widget.ImportWidgetFlags
		objectsNotInWidget []*common.Snapshot
	)

	if widgetSnapshot := snapshots.GetWidget(); widgetSnapshot != nil {
		var err error
		widgetFlags, rootObjects, err = s.getObjectsFromWidget(widgetSnapshot, oldToNewID)
		if err != nil {
			return nil, fmt.Errorf("get objects from widget: %w", err)
		}
		objectsNotInWidget = lo.Filter(snapshots.List(), func(item *common.Snapshot, index int) bool {
			return !lo.Contains(rootObjects, item.Id)
		})
	}
	if !widgetFlags.IsEmpty() || len(rootObjects) > 0 {
		// add to root collection only objects from widget, dashboard and favorites
		rootObjects = append(rootObjects, s.filterObjects(widgetFlags, objectsNotInWidget)...)
	} else {
		// if we don't have any widget, we add everything (except sub objects and templates) to root collection
		rootObjects = lo.FilterMap(snapshots.List(), func(item *common.Snapshot, index int) (string, bool) {
			if !s.objectShouldBeSkipped(item) {
				return item.Id, true
			}
			return item.Id, false
		})
	}
	rootCollection := common.NewImportCollection(s.service)
	settings := common.NewImportCollectionSetting(
		common.WithCollectionName(rootCollectionName),
		common.WithTargetObjects(rootObjects),
		common.WithRelations(),
		common.WithAddDate(),
	)
	rootCollectionSnapshot, err := rootCollection.MakeImportCollection(settings)
	if err != nil {
		return nil, err
	}
	return []*common.Snapshot{rootCollectionSnapshot}, nil
}

func (s *SpaceImport) objectShouldBeSkipped(item *common.Snapshot) bool {
	return item.Snapshot.SbType == smartblock.SmartBlockTypeSubObject ||
		item.Snapshot.SbType == smartblock.SmartBlockTypeRelation || item.Snapshot.SbType == smartblock.SmartBlockTypeObjectType ||
		item.Snapshot.SbType == smartblock.SmartBlockTypeRelationOption
}

func (s *SpaceImport) getObjectsFromWidget(widgetSnapshot *common.Snapshot, oldToNewID map[string]string) (widget.ImportWidgetFlags, []string, error) {
	widgetState, err := state.NewDocFromSnapshot("", widgetSnapshot.Snapshot.ToProto())
	if err != nil {
		return widget.ImportWidgetFlags{}, nil, fmt.Errorf("doc from snapshot: %w", err)
	}
	var (
		objectsInWidget     []string
		objectTypesToImport widget.ImportWidgetFlags
	)
	err = widgetState.Iterate(func(b simple.Block) (isContinue bool) {
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
		return widget.ImportWidgetFlags{}, nil, nil
	}
	return objectTypesToImport, objectsInWidget, nil
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
		if snapshot.Snapshot.Data.Details.GetBool(bundle.RelationKeyIsFavorite) {
			rootObjects = append(rootObjects, snapshot.Id)
			continue
		}
		if ids := snapshot.Snapshot.Data.Details.GetStringList(bundle.RelationKeySpaceDashboardId); len(ids) > 0 {
			rootObjects = append(rootObjects, ids...)
			continue
		}
	}
	return rootObjects
}
