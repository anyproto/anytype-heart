package kanban

import (
	"fmt"
	"sort"
	"strings"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

type GroupTag struct {
	Key     domain.RelationKey
	store   objectstore.ObjectStore
	Records []database.Record
}

func (t *GroupTag) InitGroups(spaceID string, f *database.Filters) error {
	if spaceID == "" {
		return fmt.Errorf("spaceId is required")
	}
	filterTag := database.FiltersAnd{
		database.FilterNot{Filter: database.FilterEmpty{Key: t.Key}},
	}

	if f == nil {
		f = &database.Filters{FilterObj: filterTag}
	} else {
		f.FilterObj = database.FiltersAnd{f.FilterObj, filterTag}
	}

	relationOptionFilter := database.FiltersAnd{
		database.FilterEq{
			Key:   bundle.RelationKeyRelationKey,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: domain.String(string(t.Key)),
		},
		database.FilterEq{
			Key:   bundle.RelationKeyResolvedLayout,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: domain.Int64(model.ObjectType_relationOption),
		},
		database.FilterEq{
			Key:   bundle.RelationKeyIsArchived,
			Cond:  model.BlockContentDataviewFilter_NotEqual,
			Value: domain.Bool(true),
		},
		database.FilterEq{
			Key:   bundle.RelationKeyIsDeleted,
			Cond:  model.BlockContentDataviewFilter_NotEqual,
			Value: domain.Bool(true),
		},
	}
	f.FilterObj = database.FiltersOr{f.FilterObj, relationOptionFilter}

	records, err := t.store.SpaceIndex(spaceID).QueryRaw(f, 0, 0)
	if err != nil {
		return fmt.Errorf("init kanban by tag, objectStore query error: %w", err)
	}

	t.Records = records

	return nil
}

func (t *GroupTag) MakeGroups() (GroupSlice, error) {
	var groups GroupSlice

	uniqMap := make(map[string]bool)

	// single tag groups
	for _, v := range t.Records {
		if tagOption := v.Details.GetString(bundle.RelationKeyRelationKey); tagOption == string(t.Key) {
			optionID := v.Details.GetString(bundle.RelationKeyId)
			if !uniqMap[optionID] {
				uniqMap[optionID] = true
				groups = append(groups, Group{
					Id:   optionID,
					Data: GroupData{Ids: []string{optionID}},
				})
			}
		}
	}

	// multiple tag groups
	for _, rec := range t.Records {
		tagIDs := slice.Filter(rec.Details.GetStringList(t.Key), func(tagID string) bool { // filter removed options
			return uniqMap[tagID]
		})

		if len(tagIDs) > 1 {
			sort.Strings(tagIDs)
			hash := strings.Join(tagIDs, "")
			if !uniqMap[hash] {
				uniqMap[hash] = true
				groups = append(groups, Group{
					Id:   hash,
					Data: GroupData{Ids: tagIDs},
				})
			}
		}
	}

	return groups, nil
}

func (t *GroupTag) MakeDataViewGroups() ([]*model.BlockContentDataviewGroup, error) {
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
