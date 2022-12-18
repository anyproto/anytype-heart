package kanban

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database/filter"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/ipfs/go-datastore/query"
	"sort"
	"strings"
)

type GroupTag struct {
	store objectstore.ObjectStore
	Records []database.Record
}

func (t *GroupTag) InitGroups(f *database.Filters) error {
	filterTag := filter.Not{Filter: filter.Empty{Key: string(bundle.RelationKeyTag)}}
	if f == nil {
		f = &database.Filters{FilterObj: filterTag}
	} else {
		f.FilterObj = filter.AndFilters{f.FilterObj, filterTag}
	}

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

	for _, v := range t.Records {
		if tags := pbtypes.GetStringList(v.Details, bundle.RelationKeyTag.String()); len(tags) > 0 {
			sort.Strings(tags)
			hash := strings.Join(tags, "")
			if !uniqMap[hash] {
				uniqMap[hash] = true
				groups = append(groups, Group{
					Id:   hash,
					Data: GroupData{Ids: tags},
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
