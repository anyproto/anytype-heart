package anymark

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/stretchr/testify/require"
)

var (
	pasteCmdArgs = "pbpaste"
	copyCmdArgs  = "pbcopy"
)

type TestCase struct {
	Blocks []map[string]interface{} `json:"blocks"`
	HTML   string                   `json:"html"`
	Desc   string                   `json:"desc"`
}

func replaceFakeIds(blocks []*model.Block) {
	var m = make(map[string]string, len(blocks))

	for i, block := range blocks {
		newId := fmt.Sprintf("%d", i+1)
		m[block.Id] = newId
		block.Id = newId
	}

	for _, block := range blocks {
		for j := range block.ChildrenIds {
			block.ChildrenIds[j] = m[block.ChildrenIds[j]]
		}
	}

}

func TestConvertHTMLToBlocks(t *testing.T) {
	bs, err := ioutil.ReadFile("_test/testData.json")
	if err != nil {
		panic(err)
	}
	var testCases []TestCase
	if err := json.Unmarshal(bs, &testCases); err != nil {
		panic(err)
	}

	for _, testCase := range testCases {
		t.Run(testCase.Desc, func(t *testing.T) {
			mdToBlocksConverter := New()
			_, blocks, _ := mdToBlocksConverter.HTMLToBlocks([]byte(testCase.HTML))
			replaceFakeIds(blocks)

			actualJson, err := json.Marshal(blocks)
			require.NoError(t, err)

			var actual []map[string]interface{}
			err = json.Unmarshal(actualJson, &actual)
			require.NoError(t, err)

			if !reflect.DeepEqual(testCase.Blocks, actual) {
				require.Equal(t, testCase.Blocks, actual)
			}
		})
	}
}
