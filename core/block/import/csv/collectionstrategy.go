package csv

import (
	//"strings"

	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark/whitespace"
	"github.com/anyproto/anytype-heart/core/block/simple"
	simpleDataview "github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	//"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	//"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark/whitespace"
	//"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type CollectionStrategy struct {
	collectionService *collection.Service
}

func NewCollectionStrategy(collectionService *collection.Service) *CollectionStrategy {
	return &CollectionStrategy{collectionService: collectionService}
}

func (c *CollectionStrategy) CreateObjects(path string, csvTable [][]string) ([]string, []*converter.Snapshot, error) {
	snapshots := make([]*converter.Snapshot, 0)
	allObjectsIDs := make([]string, 0)
	details := converter.GetDetails(path)
	details.GetFields()[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))
	_, _, st, err := c.collectionService.CreateCollection(details, nil)
	if err != nil {
		return nil, nil, err
	}

	relations, relationsSnapshots := getDetailsFromCSVTable(csvTable)
	objectsSnapshots := getEmptyObjects(csvTable, relations)
	targetIDs := make([]string, 0, len(objectsSnapshots))
	for _, objectsSnapshot := range objectsSnapshots {
		targetIDs = append(targetIDs, objectsSnapshot.Id)
	}
	allObjectsIDs = append(allObjectsIDs, targetIDs...)

	st.UpdateStoreSlice(template.CollectionStoreKey, targetIDs)
	snapshot := c.getCollectionSnapshot(details, st, path, relations)

	snapshots = append(snapshots, snapshot)
	snapshots = append(snapshots, objectsSnapshots...)
	snapshots = append(snapshots, relationsSnapshots...)
	allObjectsIDs = append(allObjectsIDs, snapshot.Id)

	return allObjectsIDs, snapshots, nil
}

func (c *CollectionStrategy) getCollectionSnapshot(details *types.Struct, st *state.State, p string, relations []*model.Relation) *converter.Snapshot {
	details = pbtypes.StructMerge(st.CombinedDetails(), details, false)
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))

	for _, relation := range relations {
		err := addRelationsToDataView(st, &model.RelationLink{
			Key:    relation.Key,
			Format: relation.Format,
		})
		if err != nil {
			//todo logging
		}
	}
	sn := &model.SmartBlockSnapshotBase{
		Blocks:        st.Blocks(),
		Details:       details,
		ObjectTypes:   []string{bundle.TypeKeyCollection.URL()},
		Collections:   st.Store(),
		RelationLinks: st.GetRelationLinks(),
	}

	snapshot := &converter.Snapshot{
		Id:       uuid.New().String(),
		FileName: p,
		Snapshot: &pb.ChangeSnapshot{Data: sn},
		SbType:   smartblock.SmartBlockTypePage,
	}
	return snapshot
}

func getEmptyObjects(csvTable [][]string, relations []*model.Relation) []*converter.Snapshot {
	snapshots := make([]*converter.Snapshot, 0, len(csvTable))
	for i := 1; i < len(csvTable); i++ {
		st := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{
				Content: &model.BlockContentOfSmartblock{
					Smartblock: &model.BlockContentSmartblock{},
				},
			}),
		}).NewState()
		details := &types.Struct{Fields: map[string]*types.Value{}}
		relationLinks := make([]*model.RelationLink, 0)
		var (
			j     = 0
			value string
		)

		for j, value = range csvTable[i] {
			name := strings.TrimSpace(whitespace.WhitespaceNormalizeString(relations[j].Name))
			if strings.EqualFold(name, "name") {
				relations[j].Name = bundle.RelationKeyName.String()
			}
			details.Fields[relations[j].Name] = pbtypes.String(value)
			relationLinks = append(relationLinks, &model.RelationLink{
				Key:    relations[j].Key,
				Format: relations[j].Format,
			})
		}

		st.SetDetails(details)
		template.InitTemplate(st, template.WithTitle)

		st.AddRelationLinks(relationLinks...)
		sn := &converter.Snapshot{
			Id:     uuid.New().String(),
			SbType: smartblock.SmartBlockTypePage,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Blocks:        st.Blocks(),
					Details:       details,
					RelationLinks: st.GetRelationLinks(),
					ObjectTypes:   []string{bundle.TypeKeyPage.URL()},
				},
			},
		}
		snapshots = append(snapshots, sn)
	}
	return snapshots
}

func addRelationsToDataView(st *state.State, rel *model.RelationLink) error {
	return st.Iterate(func(bl simple.Block) (isContinue bool) {
		if dv, ok := bl.(simpleDataview.Block); ok {
			if len(bl.Model().GetDataview().GetViews()) == 0 {
				return false
			}
			for _, view := range bl.Model().GetDataview().GetViews() {
				err := dv.AddViewRelation(view.GetId(), &model.BlockContentDataviewRelation{
					Key:       rel.Key,
					IsVisible: true,
					Width:     192,
				})
				if err != nil {
					return false
				}
			}
			err := dv.AddRelation(&model.RelationLink{
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

func getRelationDetails(key, id string, format float64) *types.Struct {
	details := &types.Struct{Fields: map[string]*types.Value{}}
	details.Fields[bundle.RelationKeyRelationFormat.String()] = pbtypes.Float64(format)
	details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(key)
	details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(addr.RelationKeyToIdPrefix + id)
	details.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(id)
	details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relation))
	return details
}
