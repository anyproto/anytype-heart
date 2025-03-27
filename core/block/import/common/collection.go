package common

import (
	"fmt"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	simpleDataview "github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const rootWidget = "rootWidget"

type ImportCollectionSetting struct {
	collectionName                                      string
	targetObjects                                       []string
	icon                                                string
	fileKeys                                            []*pb.ChangeFileKeys
	needToAddDate, shouldBeFavorite, shouldAddRelations bool
	widgetSnapshot                                      *Snapshot
}

type ImportCollectionOption func(*ImportCollectionSetting)

func NewImportCollectionSetting(opts ...ImportCollectionOption) *ImportCollectionSetting {
	s := &ImportCollectionSetting{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithCollectionName(name string) ImportCollectionOption {
	return func(s *ImportCollectionSetting) {
		s.collectionName = name
	}
}

func WithTargetObjects(objs []string) ImportCollectionOption {
	return func(s *ImportCollectionSetting) {
		s.targetObjects = objs
	}
}

func WithIcon(icon string) ImportCollectionOption {
	return func(s *ImportCollectionSetting) {
		s.icon = icon
	}
}

func WithFileKeys(keys []*pb.ChangeFileKeys) ImportCollectionOption {
	return func(s *ImportCollectionSetting) {
		s.fileKeys = keys
	}
}

func WithAddDate() ImportCollectionOption {
	return func(s *ImportCollectionSetting) {
		s.needToAddDate = true
	}
}

func WithFavorite() ImportCollectionOption {
	return func(s *ImportCollectionSetting) {
		s.shouldBeFavorite = true
	}
}

func WithRelations() ImportCollectionOption {
	return func(s *ImportCollectionSetting) {
		s.shouldAddRelations = true
	}
}

func WithWidgetSnapshot(snapshot *Snapshot) ImportCollectionOption {
	return func(s *ImportCollectionSetting) {
		s.widgetSnapshot = snapshot
	}
}

type ImportCollection struct {
	service *collection.Service
}

func NewImportCollection(service *collection.Service) *ImportCollection {
	return &ImportCollection{service: service}
}

func (r *ImportCollection) MakeImportCollection(req *ImportCollectionSetting) (*Snapshot, *Snapshot, error) {
	if req.needToAddDate {
		importDate := time.Now().Format(time.RFC3339)
		req.collectionName = fmt.Sprintf("%s %s", req.collectionName, importDate)
	}
	detailsStruct := r.getCreateCollectionRequest(req.collectionName, req.icon, req.shouldBeFavorite)
	_, _, st, err := r.service.CreateCollection(detailsStruct, []*model.InternalFlag{{
		Value: model.InternalFlag_collectionDontIndexLinks,
	}})
	if err != nil {
		return nil, nil, err
	}

	if req.shouldAddRelations {
		err = r.addRelations(st)
		if err != nil {
			return nil, nil, err
		}
	}

	detailsStruct = st.CombinedDetails().Merge(detailsStruct)
	st.UpdateStoreSlice(template.CollectionStoreKey, req.targetObjects)

	rootCollectionSnapshot := r.getRootCollectionSnapshot(req.collectionName, st, detailsStruct, req.fileKeys)
	widgetSnapshot := r.makeWidgetSnapshot(req, rootCollectionSnapshot)
	return rootCollectionSnapshot, widgetSnapshot, nil
}

func (r *ImportCollection) makeWidgetSnapshot(req *ImportCollectionSetting, rootSnapshot *Snapshot) *Snapshot {
	if req.widgetSnapshot == nil {
		return r.buildNewWidgetSnapshot(rootSnapshot.Id)
	}
	return r.enhanceExistingSnapshot(req.widgetSnapshot, rootSnapshot.Id)
}

func (r *ImportCollection) buildNewWidgetSnapshot(targetID string) *Snapshot {
	linkBlock := r.createLinkBlock(targetID)
	widgetBlock := r.createWidgetBlock(linkBlock.Id)
	rootBlock := r.createSmartBlock(widgetBlock.Id)

	return &Snapshot{
		Id:       rootBlock.Id,
		FileName: rootWidget,
		Snapshot: &SnapshotModel{
			SbType: sb.SmartBlockTypeWidget,
			Data: &StateSnapshot{
				Blocks:      []*model.Block{rootBlock, widgetBlock, linkBlock},
				Details:     r.defaultWidgetDetails(),
				ObjectTypes: []string{bundle.TypeKeyDashboard.String()},
			},
		},
	}
}

func (r *ImportCollection) enhanceExistingSnapshot(snapshot *Snapshot, targetID string) *Snapshot {
	linkBlock := r.createLinkBlock(targetID)
	widgetBlock := r.createWidgetBlock(linkBlock.Id)

	for _, block := range snapshot.Snapshot.Data.Blocks {
		if block.GetSmartblock() != nil {
			block.ChildrenIds = append(block.ChildrenIds, widgetBlock.Id)
		}
	}

	snapshot.Snapshot.Data.Blocks = append(snapshot.Snapshot.Data.Blocks, linkBlock, widgetBlock)
	return snapshot
}

func (r *ImportCollection) createLinkBlock(targetID string) *model.Block {
	return &model.Block{
		Id: bson.NewObjectId().Hex(),
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: targetID,
			},
		},
	}
}

func (r *ImportCollection) createWidgetBlock(childID string) *model.Block {
	return &model.Block{
		Id:          bson.NewObjectId().Hex(),
		ChildrenIds: []string{childID},
		Content: &model.BlockContentOfWidget{
			Widget: &model.BlockContentWidget{
				Layout: model.BlockContentWidget_CompactList,
			},
		},
	}
}

func (r *ImportCollection) createSmartBlock(childID string) *model.Block {
	return &model.Block{
		Id:          uuid.New().String(),
		ChildrenIds: []string{childID},
		Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
	}
}

func (r *ImportCollection) defaultWidgetDetails() *domain.Details {
	return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyLayout:   domain.Int64(model.ObjectType_dashboard),
		bundle.RelationKeyIsHidden: domain.Bool(true),
	})
}

func (r *ImportCollection) getRootCollectionSnapshot(
	collectionName string,
	st *state.State,
	detailsStruct *domain.Details,
	fileKeys []*pb.ChangeFileKeys,
) *Snapshot {
	if detailsStruct == nil {
		detailsStruct = domain.NewDetails()
	}
	detailsStruct.SetInt64(bundle.RelationKeyResolvedLayout, int64(model.ObjectType_collection))
	return &Snapshot{
		Id:       uuid.New().String(),
		FileName: collectionName,
		Snapshot: &SnapshotModel{
			SbType: sb.SmartBlockTypePage,
			Data: &StateSnapshot{
				Blocks:        st.Blocks(),
				Details:       detailsStruct,
				ObjectTypes:   []string{bundle.TypeKeyCollection.String()},
				RelationLinks: st.GetRelationLinks(),
				Collections:   st.Store(),
			},
			FileKeys: fileKeys,
		},
	}
}

func (r *ImportCollection) addRelations(st *state.State) error {
	for _, relation := range []*model.RelationLink{
		{
			Key:    bundle.RelationKeyTag.String(),
			Format: model.RelationFormat_tag,
		},
		{
			Key:    bundle.RelationKeyCreatedDate.String(),
			Format: model.RelationFormat_date,
		},
	} {
		err := ReplaceRelationsInDataView(st, relation)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ImportCollection) getCreateCollectionRequest(collectionName string, icon string, shouldBeFavorite bool) *domain.Details {
	return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeySourceFilePath: domain.String(collectionName),
		bundle.RelationKeyName:           domain.String(collectionName),
		bundle.RelationKeyIsFavorite:     domain.Bool(shouldBeFavorite),
		bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_collection),
		bundle.RelationKeyIconImage:      domain.String(icon),
	})
}

func ReplaceRelationsInDataView(st *state.State, rel *model.RelationLink) error {
	return st.Iterate(func(bl simple.Block) (isContinue bool) {
		if dv, ok := bl.(simpleDataview.Block); ok {
			if len(bl.Model().GetDataview().GetViews()) == 0 {
				return true
			}
			for _, view := range bl.Model().GetDataview().GetViews() {
				err := dv.ReplaceViewRelation(view.Id, rel.Key, &model.BlockContentDataviewRelation{
					Key:       rel.Key,
					IsVisible: true,
					Width:     simpleDataview.DefaultViewRelationWidth,
				})
				if err != nil {
					return true
				}
			}
		}
		return true
	})
}
