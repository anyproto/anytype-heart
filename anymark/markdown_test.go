package anymark

import (
	"encoding/json"
	"io/ioutil"
	"sync/atomic"
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

	var c int64 = 0
	idGetter := SequenceIDGetter{count: &c}
	defaultIdGetter = &idGetter

	for testNum, testCase := range testCases {
		t.Run(testCase.Desc, func(t *testing.T) {
			atomic.StoreInt64(idGetter.count, 0)

			mdToBlocksConverter := New()
			blocks, _, _ := mdToBlocksConverter.MarkdownToBlocks([]byte(testCases[testNum].MD), "", []string{})

			actualJson, err := json.Marshal(blocks)
			require.NoError(t, err)

			var actual []map[string]interface{}
			err = json.Unmarshal(actualJson, &actual)
			require.NoError(t, err)
			require.Equal(t, testCase.Blocks, actual)

		})
	}
}
