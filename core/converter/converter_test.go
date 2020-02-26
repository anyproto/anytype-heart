package converter_test

import (
	"fmt"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
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

		fmt.Println("TREE:", newTree)
		fmt.Println("TREE:", W.PrintNode(&newTree))
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

		fmt.Println("TREE:", newTree)
		fmt.Println("TREE:", W.ProcessTree(&newTree))
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

		fmt.Println("TREE:", newTree)
		fmt.Println("HTML:", W.ProcessTree(&newTree))
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

		fmt.Println("blocks:", blocks)

		W := converter.New()
		fmt.Println("TREE:", W.Convert(blocks))
	})

	t.Run("No structure", func(t *testing.T) {
		blocks := []*model.Block{
			{Id: "1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "111"}}},
			{Id: "2", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "222"}}},
		}

		fmt.Println("blocks:", blocks)

		W := converter.New()
		fmt.Println("TREE:", W.Convert(blocks))
	})

	t.Run("Layout", func(t *testing.T) {
		blocks := []*model.Block{
			{
				Id: "1",
				ChildrenIds: []string{ "2", "3" },
				Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Column}},
			},

			{
				Id: "3",
				ChildrenIds: []string{ "4", "5" },
				Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Row}},
			},

			{
				Id: "4",
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

		fmt.Println("blocks:", blocks)

		W := converter.New()
		fmt.Println("TREE:", W.Convert(blocks))
	})

}
