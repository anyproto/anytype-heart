package common

import (
	"fmt"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	simpleDataview "github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type ImportCollectionSetting struct {
	collectionName                                      string
	targetObjects                                       []string
	icon                                                string
	fileKeys                                            []*pb.ChangeFileKeys
	needToAddDate, shouldBeFavorite, shouldAddRelations bool
}

func MakeImportCollectionSetting(
	collectionName string,
	targetObjects []string,
	icon string,
	fileKeys []*pb.ChangeFileKeys,
	needToAddDate, shouldBeFavorite, shouldAddRelations bool,
) *ImportCollectionSetting {
	return &ImportCollectionSetting{
		collectionName:     collectionName,
		targetObjects:      targetObjects,
		icon:               icon,
		fileKeys:           fileKeys,
		needToAddDate:      needToAddDate,
		shouldBeFavorite:   shouldBeFavorite,
		shouldAddRelations: shouldAddRelations,
	}
}

type ImportCollection struct {
	service *collection.Service
}

func NewImportCollection(service *collection.Service) *ImportCollection {
	return &ImportCollection{service: service}
}

func (r *ImportCollection) MakeImportCollection(req *ImportCollectionSetting) (*Snapshot, error) {
	if req.needToAddDate {
		importDate := time.Now().Format(time.RFC3339)
		req.collectionName = fmt.Sprintf("%s %s", req.collectionName, importDate)
	}
	detailsStruct := r.getCreateCollectionRequest(req.collectionName, req.icon, req.shouldBeFavorite)
	_, _, st, err := r.service.CreateCollection(detailsStruct, []*model.InternalFlag{{
		Value: model.InternalFlag_collectionDontIndexLinks,
	}})
	if err != nil {
		return nil, err
	}

	if req.shouldAddRelations {
		err = r.addRelations(st)
		if err != nil {
			return nil, err
		}
	}

	detailsStruct = pbtypes.StructMerge(st.CombinedDetails(), detailsStruct, false)
	st.UpdateStoreSlice(template.CollectionStoreKey, req.targetObjects)

	return r.getRootCollectionSnapshot(req.collectionName, st, detailsStruct, req.fileKeys), nil
}

func (r *ImportCollection) getRootCollectionSnapshot(
	collectionName string,
	st *state.State,
	detailsStruct *types.Struct,
	fileKeys []*pb.ChangeFileKeys,
) *Snapshot {
	if detailsStruct.GetFields() == nil {
		detailsStruct = &types.Struct{Fields: map[string]*types.Value{}}
	}
	detailsStruct.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Int64(int64(model.ObjectType_collection))
	return &Snapshot{
		Id:       uuid.New().String(),
		FileName: collectionName,
		SbType:   sb.SmartBlockTypePage,
		Snapshot: &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
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

func (r *ImportCollection) getCreateCollectionRequest(collectionName string, icon string, shouldBeFavorite bool) *types.Struct {
	details := make(map[string]*types.Value, 0)
	details[bundle.RelationKeySourceFilePath.String()] = pbtypes.String(collectionName)
	details[bundle.RelationKeyName.String()] = pbtypes.String(collectionName)
	details[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(shouldBeFavorite)
	details[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))
	details[bundle.RelationKeyIconImage.String()] = pbtypes.String(icon)

	detailsStruct := &types.Struct{Fields: details}
	return detailsStruct
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
