package common

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	simpleDataview "github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ImportCollectionSetting struct {
	collectionName                                      string
	targetObjects                                       []string
	icon                                                string
	needToAddDate, shouldBeFavorite, shouldAddRelations bool
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

	detailsStruct = st.CombinedDetails().Merge(detailsStruct)
	st.UpdateStoreSlice(template.CollectionStoreKey, req.targetObjects)

	return r.getRootCollectionSnapshot(req.collectionName, st, detailsStruct), nil
}

func (r *ImportCollection) getRootCollectionSnapshot(
	collectionName string,
	st *state.State,
	detailsStruct *domain.Details,
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
				Blocks:      st.Blocks(),
				Details:     detailsStruct,
				ObjectTypes: []string{bundle.TypeKeyCollection.String()},
				Collections: st.Store(),
			},
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
