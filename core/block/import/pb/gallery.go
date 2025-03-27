package pb

import (
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	widgets "github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

const widgetCollectionPattern = "'s Widgets"

type GalleryImport struct {
	service *collection.Service
}

func NewGalleryImport(service *collection.Service) *GalleryImport {
	return &GalleryImport{service: service}
}

func (g *GalleryImport) ProvideCollection(snapshots []*common.Snapshot,
	widget *common.Snapshot,
	_ map[string]string,
	params *pb.RpcObjectImportRequestPbParams,
	workspaceSnapshot *common.Snapshot,
	isNewSpace bool,
) (resultSnapshots []*common.Snapshot, err error) {
	if isNewSpace {
		return nil, nil
	}
	var widgetObjects []string
	if widget != nil {
		widgetObjects = g.getObjectsFromWidgets(widget)
	}
	var (
		icon           string
		fileKeys       []*pb.ChangeFileKeys
		widgetSnapshot *common.Snapshot
	)
	if workspaceSnapshot != nil { // we use space icon for import collection
		icon = workspaceSnapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyIconImage)
		fileKeys = lo.Filter(workspaceSnapshot.Snapshot.FileKeys, func(item *pb.ChangeFileKeys, index int) bool { return item.Hash == icon })
	}
	collectionName := params.GetCollectionTitle() // collection name should be the name of experience
	if collectionName == "" {
		collectionName = rootCollectionName
	}
	rootCollection := common.NewImportCollection(g.service)
	if len(widgetObjects) > 0 {
		resultSnapshots, widgetSnapshot, err = g.getWidgetsCollection(collectionName, rootCollection, widgetObjects, icon, fileKeys, resultSnapshots)
		if err != nil {
			return nil, err
		}
	}
	objectsIDs := g.getObjectsIDs(snapshots)
	settings := common.NewImportCollectionSetting(
		common.WithCollectionName(collectionName),
		common.WithTargetObjects(objectsIDs),
		common.WithIcon(icon),
		common.WithRelations(),
		common.WithFileKeys(fileKeys),
		common.WithWidgetSnapshot(widgetSnapshot),
	)
	objectsCollection, widgetSnapshot, err := rootCollection.MakeImportCollection(settings)
	if err != nil {
		return nil, err
	}
	resultSnapshots = append(resultSnapshots, objectsCollection, widgetSnapshot)
	return resultSnapshots, nil
}

func (g *GalleryImport) getWidgetsCollection(
	collectionName string,
	rootCollection *common.ImportCollection,
	widgetObjects []string,
	icon string,
	fileKeys []*pb.ChangeFileKeys,
	collectionsSnapshots []*common.Snapshot,
) ([]*common.Snapshot, *common.Snapshot, error) {
	widgetCollectionName := collectionName + widgetCollectionPattern
	settings := common.NewImportCollectionSetting(
		common.WithCollectionName(widgetCollectionName),
		common.WithTargetObjects(widgetObjects),
		common.WithIcon(icon),
		common.WithRelations(),
		common.WithFileKeys(fileKeys),
	)
	widgetsCollectionSnapshot, widget, err := rootCollection.MakeImportCollection(settings)
	if err != nil {
		return nil, nil, err
	}
	if widgetsCollectionSnapshot != nil {
		collectionsSnapshots = append(collectionsSnapshots, widgetsCollectionSnapshot)
	}
	return collectionsSnapshots, widget, nil
}

func (g *GalleryImport) getObjectsFromWidgets(widgetSnapshot *common.Snapshot) []string {
	widgetState := state.NewDocFromSnapshot("", widgetSnapshot.Snapshot.ToProto()).(*state.State)
	var objectsInWidget []string
	err := widgetState.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil && link.TargetBlockId != "" {
			if widgets.IsPredefinedWidgetTargetId(link.TargetBlockId) {
				return true
			}
			if link.TargetBlockId == addr.MissingObject {
				return true
			}
			objectsInWidget = append(objectsInWidget, link.TargetBlockId)
		}
		return true
	})
	if err != nil {
		return nil
	}
	return objectsInWidget
}

func (g *GalleryImport) getObjectsIDs(snapshots []*common.Snapshot) []string {
	var resultIDs []string
	for _, snapshot := range snapshots {
		if snapshot.Snapshot.SbType == smartblock.SmartBlockTypePage {
			resultIDs = append(resultIDs, snapshot.Id)
		}
	}
	return resultIDs
}
