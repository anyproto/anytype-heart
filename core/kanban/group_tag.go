package kanban

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ipfs/go-datastore/query"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

type GroupTag struct {
	Key     string
	store   objectstore.ObjectStore
	Records []database.Record
}

func (t *GroupTag) InitGroups(f *database.Filters) error {
	filterTag := filter.Not{Filter: filter.Empty{Key: t.Key}}
	if f == nil {
		f = &database.Filters{FilterObj: filterTag}
	} else {
		f.FilterObj = filter.AndFilters{f.FilterObj, filterTag}
	}

	f.FilterObj = filter.OrFilters{f.FilterObj, filter.AndFilters{
		filter.Eq{
			Key:   bundle.RelationKeyRelationKey.String(),
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: pbtypes.String(t.Key),
		},
		filter.Eq{
			Key:   bundle.RelationKeyType.String(),
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: pbtypes.String(bundle.TypeKeyRelationOption.URL()),
		},
	}}

	records, err := t.store.QueryRaw(query.Query{
		Filters: []query.Filter{f},
	})
	if err != nil {
		return fmt.Errorf("init kanban by tag, objectStore query error: %v", err)
	}

	t.Records = records

	return nil
}

func (t *GroupTag) MakeGroups() (GroupSlice, error) {
	var groups GroupSlice

	uniqMap := make(map[string]bool)

	// single tag groups
	for _, v := range t.Records {
		if tagOption := pbtypes.GetString(v.Details, bundle.RelationKeyRelationKey.String()); tagOption == t.Key {
			optionID := pbtypes.GetString(v.Details, bundle.RelationKeyId.String())
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
		tagIDs := slice.Filter(pbtypes.GetStringList(rec.Details, t.Key), func(tagID string) bool { // filter removed options
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
