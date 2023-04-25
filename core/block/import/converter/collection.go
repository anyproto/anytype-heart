package converter

import (
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"

	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	simpleDataview "github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	sb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type RootCollection struct {
	service *collection.Service
}

func NewRootCollection(service *collection.Service) *RootCollection {
	return &RootCollection{service: service}
}

func (r *RootCollection) AddObjects(collectionName string, targetObjects []string) (*Snapshot, error) {
	detailsStruct := r.getCreateCollectionRequest(collectionName)
	_, _, st, err := r.service.CreateCollection(detailsStruct, nil)
	if err != nil {
		return nil, err
	}

	err = r.addRelations(st)
	if err != nil {
		return nil, err
	}

	detailsStruct = pbtypes.StructMerge(st.CombinedDetails(), detailsStruct, false)
	st.StoreSlice(smartblock.CollectionStoreKey, targetObjects)

	return r.getRootCollectionSnapshot(collectionName, st, detailsStruct), nil
}

func (r *RootCollection) getRootCollectionSnapshot(collectionName string, st *state.State, detailsStruct *types.Struct) *Snapshot {
	rootCol := &Snapshot{
		Id:       uuid.New().String(),
		FileName: collectionName,
		SbType:   sb.SmartBlockTypeCollection,
		Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Blocks:        st.Blocks(),
			Details:       detailsStruct,
			ObjectTypes:   []string{bundle.TypeKeyCollection.URL()},
			RelationLinks: st.GetRelationLinks(),
			Collections:   st.Store(),
		},
		},
	}
	return rootCol
}

func (r *RootCollection) addRelations(st *state.State) error {
	for _, relation := range []*model.Relation{
		{
			Key:    bundle.RelationKeyTag.String(),
			Format: model.RelationFormat_tag,
		},
		{
			Key:    bundle.RelationKeyCreatedDate.String(),
			Format: model.RelationFormat_date,
		},
	} {
		err := addRelationsToCollectionDataView(st, relation)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RootCollection) getCreateCollectionRequest(collectionName string) *types.Struct {
	details := make(map[string]*types.Value, 0)
	details[bundle.RelationKeySource.String()] = pbtypes.String(collectionName)
	details[bundle.RelationKeyName.String()] = pbtypes.String(collectionName)
	details[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(true)
	details[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))

	detailsStruct := &types.Struct{Fields: details}
	return detailsStruct
}

func addRelationsToCollectionDataView(st *state.State, rel *model.Relation) error {
	return st.Iterate(func(bl simple.Block) (isContinue bool) {
		if dv, ok := bl.(simpleDataview.Block); ok {
			if len(bl.Model().GetDataview().GetViews()) == 0 {
				return false
			}
			err := dv.AddViewRelation(bl.Model().GetDataview().GetViews()[0].GetId(), &model.BlockContentDataviewRelation{
				Key:       rel.Key,
				IsVisible: true,
				Width:     192,
			})
			if err != nil {
				return false
			}
			err = dv.AddRelation(&model.RelationLink{
				Key:    rel.Key,
				Format: rel.Format,
			})
			if err != nil {
				return false
			}
		}
		return true
	})
}
