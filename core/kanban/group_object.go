package kanban

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	defaultGroup = "empty"
	columnLimit  = 100
)

type GroupObject struct {
	key                  string
	store                objectstore.ObjectStore
	objectsWithGivenType []database.Record
	objectsWithRelation  []database.Record
}

func (t *GroupObject) InitGroups(spaceId string, f *database.Filters) error {
	relation, err := t.retrieveRelationFromStore(spaceId)
	if err != nil {
		return err
	}

	t.objectsWithGivenType, err = t.retrieveObjectsWithGivenType(spaceId, relation)
	if err != nil {
		return err
	}

	t.objectsWithRelation, err = t.retrieveObjectsWithGivenRelation(f, spaceId)
	if err != nil {
		return err
	}
	return nil
}

func (t *GroupObject) retrieveObjectsWithGivenRelation(f *database.Filters, spaceID string) ([]database.Record, error) {
	spaceFilter := database.FilterEq{
		Key:   bundle.RelationKeySpaceId.String(),
		Cond:  model.BlockContentDataviewFilter_Equal,
		Value: pbtypes.String(spaceID),
	}

	filterEmptyRelation := database.FiltersAnd{
		database.FilterNot{Filter: database.FilterEmpty{Key: t.key}},
		spaceFilter,
	}

	if f == nil {
		f = &database.Filters{FilterObj: filterEmptyRelation}
	} else {
		f.FilterObj = database.FiltersAnd{f.FilterObj, filterEmptyRelation}
	}

	return t.store.QueryRaw(f, 0, 0)
}

func (t *GroupObject) retrieveObjectsWithGivenType(spaceID string, relation database.Record) ([]database.Record, error) {
	objectTypes := pbtypes.GetValueList(relation.Details, bundle.RelationKeyRelationFormatObjectTypes.String())
	filterObjectTypes := database.FilterIn{
		Key:   bundle.RelationKeyType.String(),
		Value: &types.ListValue{Values: objectTypes},
	}
	spaceFilter := database.FilterEq{
		Key:   bundle.RelationKeySpaceId.String(),
		Cond:  model.BlockContentDataviewFilter_Equal,
		Value: pbtypes.String(spaceID),
	}
	filter := &database.Filters{FilterObj: database.FiltersAnd{spaceFilter, filterObjectTypes}}
	if len(objectTypes) == 0 {
		filter = t.makeFilterForEmptyObjectTypesList(spaceFilter)
	}
	return t.store.QueryRaw(filter, 0, 0)
}

func (t *GroupObject) makeFilterForEmptyObjectTypesList(spaceFilter database.FilterEq) *database.Filters {
	list := pbtypes.GetList(pbtypes.IntList([]int{int(model.ObjectType_relationOption), int(model.ObjectType_space), int(model.ObjectType_spaceView)}...))
	filterLayouts := database.FilterNot{
		Filter: database.FilterIn{
			Key:   bundle.RelationKeyLayout.String(),
			Value: &types.ListValue{Values: list},
		},
	}
	filterRelation := database.FilterNot{Filter: database.FiltersAnd{
		database.FilterEq{
			Key:   bundle.RelationKeyRelationKey.String(),
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: pbtypes.String(t.key),
		},
		database.FilterEq{
			Key:   bundle.RelationKeyLayout.String(),
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: pbtypes.Int64(int64(model.ObjectType_relation)),
		},
	}}
	return &database.Filters{FilterObj: database.FiltersAnd{spaceFilter, filterLayouts, filterRelation}}
}

func (t *GroupObject) retrieveRelationFromStore(spaceID string) (database.Record, error) {
	relationFilter := database.FiltersAnd{
		database.FilterEq{
			Key:   bundle.RelationKeySpaceId.String(),
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: pbtypes.String(spaceID),
		},
		database.FilterEq{
			Key:   bundle.RelationKeyRelationKey.String(),
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: pbtypes.String(t.key),
		},
		database.FilterEq{
			Key:   bundle.RelationKeyLayout.String(),
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: pbtypes.Int64(int64(model.ObjectType_relation)),
		},
	}

	relations, err := t.store.QueryRaw(&database.Filters{FilterObj: relationFilter}, 0, 0)
	if err != nil {
		return database.Record{}, fmt.Errorf("init kanban by tag, objectStore query error: %w", err)
	}

	if len(relations) == 0 {
		return database.Record{}, fmt.Errorf("no such relations")
	}
	return relations[0], nil
}

func (t *GroupObject) MakeGroups() (GroupCounts, error) {
	uniqMap := make(map[string]*GroupCount)
	for _, v := range t.objectsWithGivenType {
		t.makeGroupsFromObjectsWithGivenType(v, uniqMap)
	}
	for _, v := range t.objectsWithRelation {
		t.makeGroupsFromObjectsWithRelation(v, uniqMap)
	}

	var groups GroupCounts = make([]*GroupCount, 0, len(uniqMap))
	for _, group := range uniqMap {
		groups = append(groups, group)
	}
	sort.Sort(groups)
	if groups.Len() > columnLimit {
		groups = groups[:columnLimit]
	}
	return groups, nil
}

func (t *GroupObject) makeGroupsFromObjectsWithGivenType(v database.Record, uniqMap map[string]*GroupCount) {
	if objectId := pbtypes.GetString(v.Details, bundle.RelationKeyId.String()); objectId != "" {
		uniqMap[objectId] = &GroupCount{
			Group: Group{
				Id:   objectId,
				Data: GroupData{Ids: []string{objectId}},
			},
		}
	}
}

func (t *GroupObject) makeGroupsFromObjectsWithRelation(v database.Record, uniqMap map[string]*GroupCount) {
	if objectIds := pbtypes.GetStringList(v.Details, t.key); len(objectIds) > 1 {
		sort.Strings(objectIds)
		hash := strings.Join(objectIds, "")
		if groups, ok := uniqMap[hash]; !ok {
			uniqMap[hash] = &GroupCount{
				Group: Group{
					Id:   hash,
					Data: GroupData{Ids: objectIds},
				},
				Count: 1,
			}
		} else {
			groups.Count++
		}
	}
	if objectIds := pbtypes.GetStringList(v.Details, t.key); len(objectIds) == 1 {
		if groups, ok := uniqMap[objectIds[0]]; ok {
			groups.Count++
		}
	}
}

func (t *GroupObject) MakeDataViewGroups() ([]*model.BlockContentDataviewGroup, error) {
	groups, err := t.MakeGroups()
	if err != nil {
		return nil, err
	}
	result := make([]*model.BlockContentDataviewGroup, 0, len(groups))
	for _, g := range groups {
		result = append(result, &model.BlockContentDataviewGroup{
			Id: Hash(g.Id),
			Value: &model.BlockContentDataviewGroupValueOfTag{
				Tag: &model.BlockContentDataviewTag{
					Ids: g.Data.Ids,
				}},
		})
	}

	result = append([]*model.BlockContentDataviewGroup{{
		Id: defaultGroup,
		Value: &model.BlockContentDataviewGroupValueOfTag{
			Tag: &model.BlockContentDataviewTag{
				Ids: make([]string, 0),
			}},
	}}, result...)

	return result, nil
}
