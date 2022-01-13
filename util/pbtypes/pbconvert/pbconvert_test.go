package pbconvert

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/stretchr/testify/assert"
)

func TestRelationsToMap(t *testing.T) {
	rel := &model.Relation{
		Key:              "test",
		Format:           1,
		Name:             "name",
		DefaultValue:     pbtypes.Int64(42),
		DataSource:       2,
		Hidden:           true,
		ReadOnly:         true,
		ReadOnlyRelation: true,
		Multi:            true,
		ObjectTypes:      []string{"1", "2"},
		SelectDict:       nil,
		MaxCount:         3,
		Description:      "description",
		Scope:            4,
		Creator:          "creator",
	}
	v := RelationToValue(rel)
	rel2 := StructToRelation(v.GetStructValue())
	assert.Equal(t, rel, rel2)
}

func TestRelationOptionToValue(t *testing.T) {
	opt := &model.RelationOption{
		Id:    "1",
		Text:  "2",
		Color: "3",
		Scope: 4,
	}
	v := RelationOptionToValue(opt)
	opt2 := StructToRelationOption(v.GetStructValue())
	assert.Equal(t, opt, opt2)
}
