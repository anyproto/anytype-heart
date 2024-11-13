package kanban

import (
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type GroupCheckBox struct {
}

func (gCh *GroupCheckBox) InitGroups(spaceID string, f *database.Filters) error {
	if spaceID == "" {
		return fmt.Errorf("spaceId is required")
	}
	return nil
}

func (gCh *GroupCheckBox) MakeGroups() (GroupSlice, error) {
	return []Group{{Id: "true"}, {Id: "false"}}, nil
}

func (gCh *GroupCheckBox) MakeDataViewGroups() ([]*model.BlockContentDataviewGroup, error) {
	var result []*model.BlockContentDataviewGroup

	result = []*model.BlockContentDataviewGroup{{
		Id: "true",
		Value: &model.BlockContentDataviewGroupValueOfCheckbox{
			Checkbox: &model.BlockContentDataviewCheckbox{
				Checked: true,
			}},
	}, {
		Id: "false",
		Value: &model.BlockContentDataviewGroupValueOfCheckbox{
			Checkbox: &model.BlockContentDataviewCheckbox{
				Checked: false,
			}},
	}}

	return result, nil
}
