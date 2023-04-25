package csv

import (
	"strings"

	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"

	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	sb "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/markdown/anymark/whitespace"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type CollectionStrategy struct {
	collectionService *collection.Service
}

func NewCollectionStrategy(collectionService *collection.Service) *CollectionStrategy {
	return &CollectionStrategy{collectionService: collectionService}
}

func (c *CollectionStrategy) CreateObjects(path string, csvTable [][]string) ([]string, []*converter.Snapshot, map[string][]*converter.Relation, error) {
	snapshots := make([]*converter.Snapshot, 0)
	allObjectsIDs := make([]string, 0)
	details := converter.GetDetails(path)
	details.GetFields()[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))
	_, _, st, err := c.collectionService.CreateCollection(details, nil)
	if err != nil {
		return nil, nil, nil, err
	}

	relations := getDetailsFromCSVTable(csvTable)
	objectsSnapshots, objectsRelations := getEmptyObjects(csvTable, relations)
	targetIDs := make([]string, 0, len(objectsSnapshots))
	for _, objectsSnapshot := range objectsSnapshots {
		targetIDs = append(targetIDs, objectsSnapshot.Id)
	}
	allObjectsIDs = append(allObjectsIDs, targetIDs...)

	st.StoreSlice(sb.CollectionStoreKey, targetIDs)
	snapshot := c.getCollectionSnapshot(details, st, path)

	snapshots = append(snapshots, snapshot)
	snapshots = append(snapshots, objectsSnapshots...)
	allObjectsIDs = append(allObjectsIDs, snapshot.Id)

	objectsRelations[snapshot.Id] = relations
	return allObjectsIDs, snapshots, objectsRelations, nil
}

func (c *CollectionStrategy) getCollectionSnapshot(details *types.Struct, st *state.State, p string) *converter.Snapshot {
	details = pbtypes.StructMerge(st.CombinedDetails(), details, false)
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
		SbType:   smartblock.SmartBlockTypeCollection,
	}
	return snapshot
}

func getEmptyObjects(csvTable [][]string, relations []*converter.Relation) ([]*converter.Snapshot, map[string][]*converter.Relation) {
	snapshots := make([]*converter.Snapshot, 0, len(csvTable))
	objectsRelations := make(map[string][]*converter.Relation, len(csvTable))

	for i := 1; i < len(csvTable); i++ {
		st := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{
				Content: &model.BlockContentOfSmartblock{
					Smartblock: &model.BlockContentSmartblock{},
				},
			}),
		}).NewState()
		details := &types.Struct{Fields: map[string]*types.Value{}}
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
		}

		st.SetDetails(details)
		template.InitTemplate(st, template.WithTitle)

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

		objectsRelations[sn.Id] = relations
	}
	return snapshots, objectsRelations
}
