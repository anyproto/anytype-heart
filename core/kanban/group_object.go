package kanban

import (
	"fmt"
	"sort"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type GroupObject struct {
	Key     string
	store   objectstore.ObjectStore
	Records []database.Record
}

func (t *GroupObject) InitGroups(spaceID string, f *database.Filters) error {
	spaceFilter := database.FilterEq{
		Key:   bundle.RelationKeySpaceId.String(),
		Cond:  model.BlockContentDataviewFilter_Equal,
		Value: pbtypes.String(spaceID),
	}

	filterTag := database.FiltersAnd{
		database.FilterNot{Filter: database.FilterEmpty{Key: t.Key}},
	}

	if spaceID != "" {
		filterTag = append(filterTag, spaceFilter)
	}

	if f == nil {
		f = &database.Filters{FilterObj: filterTag}
	} else {
		f.FilterObj = database.FiltersAnd{f.FilterObj, filterTag}
	}

	records, err := t.store.QueryRaw(f, 0, 0)
	if err != nil {
		return fmt.Errorf("init kanban by tag, objectStore query error: %w", err)
	}

	t.Records = records
	return nil
}

func (t *GroupObject) MakeGroups() (GroupSlice, error) {
	var groups GroupSlice
	uniqMap := make(map[string]bool)
	for _, v := range t.Records {
		if objectId := pbtypes.GetString(v.Details, t.Key); objectId == "" {
			if objectIds := pbtypes.GetStringList(v.Details, t.Key); len(objectIds) > 0 {
				sort.Strings(objectIds)
				hash := strings.Join(objectIds, "")
				if !uniqMap[hash] {
					uniqMap[hash] = true
					groups = append(groups, Group{
						Id:   hash,
						Data: GroupData{Ids: objectIds},
					})
				}
			}
		} else if _, ok := uniqMap[objectId]; !ok {
			uniqMap[objectId] = true
			groups = append(groups, Group{
				Id:   objectId,
				Data: GroupData{Ids: []string{objectId}},
			})
		}
	}
	return groups, nil
}

func (t *GroupObject) MakeDataViewGroups() ([]*model.BlockContentDataviewGroup, error) {
	var result []*model.BlockContentDataviewGroup

	groups, err := t.MakeGroups()
	if err != nil {
		return nil, err
	}

	sort.Sort(groups)

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
		Id: "empty",
		Value: &model.BlockContentDataviewGroupValueOfTag{
			Tag: &model.BlockContentDataviewTag{
				Ids: make([]string, 0),
			}},
	}}, result...)

	return result, nil
}
