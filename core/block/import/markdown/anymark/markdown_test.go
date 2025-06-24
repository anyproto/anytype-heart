package anymark

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type MdCase struct {
	MD     string                   `json:"md"`
	Blocks []map[string]interface{} `json:"blocks"`
	Desc   string                   `json:"desc"`
}

func TestConvertMdToBlocks(t *testing.T) {
	bs, err := os.ReadFile("testdata/md_cases.json")
	if err != nil {
		panic(err)
	}
	var testCases []MdCase
	if err := json.Unmarshal(bs, &testCases); err != nil {
		panic(err)
	}

	for testNum, testCase := range testCases {
		t.Run(testCase.Desc, func(t *testing.T) {
			blocks, _, err := MarkdownToBlocks([]byte(testCases[testNum].MD), "", []string{})
			require.NoError(t, err)
			replaceFakeIds(blocks)

			actualJson, err := json.Marshal(blocks)
			require.NoError(t, err)

			var actual []map[string]interface{}
			err = json.Unmarshal(actualJson, &actual)
			require.NoError(t, err)

			if !reflect.DeepEqual(testCase.Blocks, actual) {
				fmt.Println("expected:\n", string(actualJson))
				require.Equal(t, testCase.Blocks, actual)
			}
		})
	}
}

func TestAnytypeSchemeLinks(t *testing.T) {
	t.Run("anytype link", func(t *testing.T) {
		md := "Link to [obj](anytype://123)"
		blocks, _, err := MarkdownToBlocks([]byte(md), "", nil)
		require.NoError(t, err)

		var markParam string
		for _, b := range blocks {
			if b.GetText() != nil && b.GetText().GetMarks() != nil {
				marks := b.GetText().GetMarks().GetMarks()
				if len(marks) > 0 {
					markParam = marks[0].GetParam()
					break
				}
			}
		}
		require.Equal(t, "anytype://123", markParam)
	})

	t.Run("anytype image", func(t *testing.T) {
		md := "![img](anytype://image)"
		blocks, _, err := MarkdownToBlocks([]byte(md), "", nil)
		require.NoError(t, err)

		var imgName string
		for _, b := range blocks {
			if f := b.GetFile(); f != nil {
				imgName = f.GetName()
				break
			}
		}
		require.Equal(t, "anytype://image", imgName)
	})
}
