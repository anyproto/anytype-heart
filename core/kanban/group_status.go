package kanban

import (
	"fmt"
	"sort"

	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type GroupStatus struct {
	key     string
	store   objectstore.ObjectStore
	Options []*model.RelationOption
}

func (gs *GroupStatus) InitGroups(spaceID string, f *database.Filters) error {
	if spaceID == "" {
		return fmt.Errorf("spaceId is required")
	}
	options, err := gs.store.SpaceStore(spaceID).ListRelationOptions(gs.key)
	if err != nil {
		return err
	}

	gs.Options = options

	return nil
}

func (gs *GroupStatus) MakeGroups() (GroupSlice, error) {
	var groups GroupSlice

	uniqMap := make(map[string]bool)

	for _, rel := range gs.Options {
		if !uniqMap[rel.Text] {
			uniqMap[rel.Text] = true
			groups = append(groups, Group{
				Id:   rel.Id,
				Data: GroupData{Ids: []string{rel.Id}},
			})
		}
	}

	return groups, nil
}

func (gs *GroupStatus) MakeDataViewGroups() ([]*model.BlockContentDataviewGroup, error) {
	var result []*model.BlockContentDataviewGroup

	groups, err := gs.MakeGroups()
	if err != nil {
		return nil, err
	}

	for _, g := range groups {
		if len(g.Data.Ids) < 1 {
			continue
		}
		result = append(result, &model.BlockContentDataviewGroup{
			Id: g.Id,
			Value: &model.BlockContentDataviewGroupValueOfStatus{
				Status: &model.BlockContentDataviewStatus{
					Id: g.Data.Ids[0],
				}},
		})
	}

	sort.Slice(groups[:], func(i, j int) bool {
		return groups[i].Id < groups[j].Id
	})

	result = append([]*model.BlockContentDataviewGroup{{
		Id:    "empty",
		Value: &model.BlockContentDataviewGroupValueOfStatus{Status: &model.BlockContentDataviewStatus{}},
	}}, result...)

	return result, nil
}
