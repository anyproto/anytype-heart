package converter_test

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
	"testing"
)

func TestExportHtml(t *testing.T) {

	t.Run("Simple", func(t *testing.T) {
		blocks := []*model.Block{
			{Id:"1", ChildrenIds:[]string{"2", "3"}},
			{Id:"2", ChildrenIds:[]string{"4", "5"}},
			{Id:"3"},
			{Id:"4"},
			{Id:"5"},
			{Id:"6"},
			{Id:"7"},
		}
		W := exportHtml.wal
		W.CreateTree(blocks)
	})
}
