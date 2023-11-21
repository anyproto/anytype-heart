package pb

import (
	"github.com/globalsign/mgo/bson"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	widgets "github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type GalleryImport struct {
	service *collection.Service
}

func NewGalleryImport(service *collection.Service) *GalleryImport {
	return &GalleryImport{service: service}
}

func (g *GalleryImport) ProvideCollection(
	_ []*common.Snapshot,
	widget *common.Snapshot,
	_ map[string]string,
	params *pb.RpcObjectImportRequestPbParams,
	workspaceSnapshot *common.Snapshot,
) (*common.Snapshot, error) {
	var widgetObjects []string
	if widget != nil {
		widgetObjects = g.getObjectsFromWidgets(widget)
	}
	var (
		icon     string
		fileKeys []*pb.ChangeFileKeys
	)
	if workspaceSnapshot != nil { // we use space icon for import collection
		icon = pbtypes.GetString(workspaceSnapshot.Snapshot.Data.Details, bundle.RelationKeyIconImage.String())
		fileKeys = lo.Filter(workspaceSnapshot.Snapshot.FileKeys, func(item *pb.ChangeFileKeys, index int) bool { return item.Hash == icon })
	}
	collectionName := params.GetCollectionTitle() // collection name should be the name of experience
	if collectionName == "" {
		collectionName = rootCollectionName
	}
	rootCollection := common.NewRootCollection(g.service)
	collectionSnapshot, err := rootCollection.MakeRootCollection(collectionName, widgetObjects, icon, fileKeys)
	if collectionSnapshot != nil && widget != nil {
		g.addCollectionWidget(widget, collectionSnapshot.Id)
	}
	return collectionSnapshot, err
}

func (g *GalleryImport) getObjectsFromWidgets(widgetSnapshot *common.Snapshot) []string {
	widgetState := state.NewDocFromSnapshot("", widgetSnapshot.Snapshot).(*state.State)
	var objectsInWidget []string
	err := widgetState.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil && link.TargetBlockId != "" {
			if widgets.IsPredefinedWidgetTargetId(link.TargetBlockId) {
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

func (g *GalleryImport) addCollectionWidget(widgetSnapshot *common.Snapshot, collectionID string) {
	id := bson.NewObjectId().Hex()
	// create widget for import collection
	linkBlock := &model.Block{
		Id: id,
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: collectionID,
			},
		},
	}
	widgetID := bson.NewObjectId().Hex()
	widgetBlock := &model.Block{
		Id:          widgetID,
		ChildrenIds: []string{id},
		Content: &model.BlockContentOfWidget{Widget: &model.BlockContentWidget{
			Layout: 0,
			Limit:  0,
			ViewId: "",
		}},
	}
	// for widget object we only add import collection, other widgets should be erased
	widgetSnapshot.Snapshot.Data.Blocks = []*model.Block{widgetBlock, linkBlock}
}
