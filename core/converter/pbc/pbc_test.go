package pbc

import (
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestPbc_Convert(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		s := state.NewDoc("root", nil).(*state.State)
		template.InitTemplate(s, template.WithTitle)
		c := NewConverter(s, false, nil)
		result := c.Convert(model.SmartBlockType_Page)
		assert.NotEmpty(t, result)
	})
	t.Run("dependent details", func(t *testing.T) {
		s := state.NewDoc("root", nil).(*state.State)
		template.InitTemplate(s, template.WithTitle)
		records := []database.Record{
			{
				Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId:   domain.String("test"),
					bundle.RelationKeyName: domain.String("test"),
				}),
			},
		}
		c := NewConverter(s, true, records)
		result := c.Convert(model.SmartBlockType_Page)
		assert.NotEmpty(t, result)

		var resultSnapshot pb.SnapshotWithType
		err := jsonpb.UnmarshalString(string(result), &resultSnapshot)
		assert.Nil(t, err)
		expected := []*pb.DependantDetail{
			{
				Id: "test",
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyId.String():   pbtypes.String("test"),
					bundle.RelationKeyName.String(): pbtypes.String("test"),
				}},
			},
		}
		assert.Equal(t, expected, resultSnapshot.DependantDetails)
	})
}
