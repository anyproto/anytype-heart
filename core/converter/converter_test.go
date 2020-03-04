package converter_test

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestConverter_ProcessTree(t *testing.T) {
	t.Run("Tree: trivia", func(t *testing.T) {
		blocks := []*model.Block{
			{Id:"1", ChildrenIds:[]string{"2"}, Content:&model.BlockContentOfText{Text: &model.BlockContentText{Text:"111"}}},
			{Id:"2", Content:&model.BlockContentOfText{Text: &model.BlockContentText{Text:"222"}}},
		}

		W := converter.New()

		newTree := W.CreateTree(blocks)

		assert.NotEmpty(t, W.PrintNode(&newTree))
	})


	t.Run("Tree: trivia 2", func(t *testing.T) {
		blocks := []*model.Block{
			{Id:"1", ChildrenIds:[]string{"2", "3"}, Content:&model.BlockContentOfText{Text: &model.BlockContentText{Text:"111"}}},
			{Id:"2",  ChildrenIds:[]string{"4", "5"}, Content:&model.BlockContentOfText{Text: &model.BlockContentText{Text:"222"}}},
			{Id:"3", Content:&model.BlockContentOfText{Text: &model.BlockContentText{Text:"333"}}},
			{Id:"4", Content:&model.BlockContentOfText{Text: &model.BlockContentText{Text:"444"}}},
			{Id:"5", Content:&model.BlockContentOfText{Text: &model.BlockContentText{Text:"555"}}},
		}

		W := converter.New()

		newTree := W.CreateTree(blocks)

		images := make(map[string][]byte)
		assert.NotEmpty(t, W.ProcessTree(&newTree, images))
	})

	t.Run("Tree: medium", func(t *testing.T) {
		blocks := []*model.Block{
			{Id:"1", ChildrenIds:[]string{"2", "3"}, Content:&model.BlockContentOfText{Text: &model.BlockContentText{Text:"111"}}},
			{Id:"2", ChildrenIds:[]string{"4", "5"}, Content:&model.BlockContentOfText{Text: &model.BlockContentText{Text:"222"}}},
			{Id:"3", Content:&model.BlockContentOfText{Text: &model.BlockContentText{Text:"333"}}},
			{Id:"4", Content:&model.BlockContentOfText{Text: &model.BlockContentText{Text:"444"}}},
			{Id:"5", Content:&model.BlockContentOfText{Text: &model.BlockContentText{Text:"555"}}},
			{Id:"6", Content:&model.BlockContentOfText{Text: &model.BlockContentText{Text:"666"}}},
			{Id:"7", Content:&model.BlockContentOfText{Text: &model.BlockContentText{Text:"777"}}},
			{Id:"8", Content:&model.BlockContentOfText{Text: &model.BlockContentText{Text:"888"}}},
		}

		W := converter.New()

		newTree := W.CreateTree(blocks)

		images := make(map[string][]byte)
		assert.NotEmpty(t, W.ProcessTree(&newTree, images))
	})
}

func TestConverter_Convert(t *testing.T) {
	t.Run("Trivia", func(t *testing.T) {
		blocks := []*model.Block{
			{Id: "1", ChildrenIds: []string{"2", "3"}, Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "111"}}},
			{Id: "2", ChildrenIds: []string{"4", "5"}, Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "222"}}},
			{Id: "3", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "333"}}},
			{Id: "4", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "444"}}},
			{Id: "5", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "555"}}},
		}


		W := converter.New()
		images := make(map[string][]byte)
		assert.NotEmpty(t, W.Convert(blocks, images))
	})

	t.Run("No structure", func(t *testing.T) {
		blocks := []*model.Block{
			{Id: "1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "111"}}},
			{Id: "2", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "222"}}},
		}


		W := converter.New()
		images := make(map[string][]byte)

		assert.NotEmpty(t, W.Convert(blocks, images))
	})

	t.Run("Layout", func(t *testing.T) {
		blocks := []*model.Block{
			{
				Id:          "1",
				ChildrenIds: []string{"2", "3"},
				Content:     &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Column}},
			},

			{
				Id:          "3",
				ChildrenIds: []string{"4", "5"},
				Content:     &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Row}},
			},

			{
				Id:      "4",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "abcdef"}},
			},

			{
				Id: "5",
				Content: &model.BlockContentOfFile{File: &model.BlockContentFile{
					Hash:  "Qmcm5gdPCMDRAgmHdnduWB93Qk1X4RyUrsjFXjeAchnGcZ",
					Name:  "FileName.png",
					Type:  model.BlockContentFile_Image,
					State: model.BlockContentFile_Done,
				}},
			},
		}

		W := converter.New()

		images := make(map[string][]byte)
		images["Qmcm5gdPCMDRAgmHdnduWB93Qk1X4RyUrsjFXjeAchnGcZ"] = []byte{0, 0, 0, 0, 0}

		assert.NotEmpty(t, W.Convert(blocks, images))
	})

}
