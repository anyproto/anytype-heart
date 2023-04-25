package converter

import (
	"bytes"
	"path/filepath"
	"strings"

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
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

var log = logging.Logger("import")

func GetSourceDetail(fileName, importPath string) string {
	var source bytes.Buffer
	source.WriteString(strings.TrimPrefix(filepath.Ext(fileName), "."))
	source.WriteString(":")
	source.WriteString(importPath)
	source.WriteRune(filepath.Separator)
	source.WriteString(fileName)
	return source.String()
}

func GetDetails(name string) *types.Struct {
	var title string

	if title == "" {
		title = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	}

	fields := map[string]*types.Value{
		bundle.RelationKeyName.String():   pbtypes.String(title),
		bundle.RelationKeySource.String(): pbtypes.String(name),
	}
	return &types.Struct{Fields: fields}
}

func AddObjectsToRootCollection(collectionService *collection.Service, collectionName string, targetObject []string) (*Snapshot, error) {
	details := make(map[string]*types.Value, 0)
	details[bundle.RelationKeySource.String()] = pbtypes.String(collectionName)
	details[bundle.RelationKeyName.String()] = pbtypes.String(collectionName)
	details[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(true)
	details[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))

	detailsStruct := &types.Struct{Fields: details}
	_, _, st, err := collectionService.CreateCollection(detailsStruct, nil)
	if err != nil {
		return nil, err
	}

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
		err = addRelationsToCollectionDataView(st, relation)
		if err != nil {
			return nil, err
		}
	}

	detailsStruct = pbtypes.StructMerge(st.CombinedDetails(), detailsStruct, false)
	rootCol := &Snapshot{
		Id:       uuid.New().String(),
		FileName: collectionName,
		SbType:   sb.SmartBlockTypeCollection,
		Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Blocks:        st.Blocks(),
			Details:       detailsStruct,
			ObjectTypes:   []string{bundle.TypeKeyCollection.URL()},
			RelationLinks: st.GetRelationLinks(),
			Collections:   st.GetCollection(smartblock.CollectionStoreKey),
		},
		},
	}

	AddObjectToSnapshot(rootCol, targetObject)

	return rootCol, nil
}

func AddObjectToSnapshot(snapshots *Snapshot, targetID []string) {
	snapshots.Snapshot.Data.Collections = &types.Struct{
		Fields: map[string]*types.Value{smartblock.CollectionStoreKey: pbtypes.StringList(targetID)},
	}
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
