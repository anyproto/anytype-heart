package kanban

import (
	"crypto/md5"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"sort"
	"strings"
)

type GroupTag struct {
	store objectstore.ObjectStore
	Records []database.Record
}

func (t *GroupTag) Init(reqFilters []*model.BlockContentDataviewFilter) error {
	filters := []*model.BlockContentDataviewFilter{
		{RelationKey: string(bundle.RelationKeyIsDeleted), Condition: model.BlockContentDataviewFilter_Equal},
		{RelationKey: string(bundle.RelationKeyIsArchived), Condition: model.BlockContentDataviewFilter_Equal},
		{RelationKey: string(bundle.RelationKeyType), Condition: model.BlockContentDataviewFilter_NotIn, Value: pbtypes.StringList([]string{
			bundle.TypeKeyFile.URL(),
			bundle.TypeKeyImage.URL(),
			bundle.TypeKeyVideo.URL(),
			bundle.TypeKeyAudio.URL(),
		})},
		{RelationKey: string(bundle.RelationKeyTag), Condition: model.BlockContentDataviewFilter_NotEmpty},
	}

	filters = append(filters, reqFilters...)
	records, _, err := t.store.Query(nil, database.Query{
		Filters: filters,
	})
	if err != nil {
		return err
	}

	t.Records = records

	return nil
}

func (t *GroupTag) MakeGroups() ([]Group, error) {
	var groups []Group

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

	sort.Slice(groups[:], func(i, j int) bool {
		return len(groups[i].Id) > len(groups[j].Id)
	})

	for _, g := range groups {
		result = append(result, &model.BlockContentDataviewGroup{
			Id:  fmt.Sprintf("%x", md5.Sum([]byte(g.Id))),
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
