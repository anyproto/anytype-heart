package anymark

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

type MdCase struct {
	MD     string                   `json:"md"`
	Blocks []map[string]interface{} `json:"blocks"`
	Desc   string                   `json:"desc"`
}

func TestConvertMdToBlocks(t *testing.T) {
	bs, err := ioutil.ReadFile("_test/md_cases.json")
	if err != nil {
		panic(err)
	}
	var testCases []MdCase
	if err := json.Unmarshal(bs, &testCases); err != nil {
		panic(err)
	}

	for testNum, testCase := range testCases {
		t.Run(testCase.Desc, func(t *testing.T) {
			mdToBlocksConverter := New()
			blocks, _, _ := mdToBlocksConverter.MarkdownToBlocks([]byte(testCases[testNum].MD), "", []string{})
			replaceFakeIds(blocks)

			actualJson, err := json.Marshal(blocks)
			require.NoError(t, err)

			var actual []map[string]interface{}
			err = json.Unmarshal(actualJson, &actual)
			require.NoError(t, err)
			require.Equal(t, testCase.Blocks, actual)

		})
	}
}
