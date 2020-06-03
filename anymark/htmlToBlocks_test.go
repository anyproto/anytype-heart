package anymark

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync/atomic"
	"testing"

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

type SequenceIDGetter struct {
	count *int64
}

func (sg SequenceIDGetter) String() string {
	return fmt.Sprintf("%d", atomic.AddInt64(sg.count, 1))
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

	var c int64 = 0
	idGetter := SequenceIDGetter{count: &c}
	defaultIdGetter = &idGetter

	for _, testCase := range testCases {
		t.Run(testCase.Desc, func(t *testing.T) {
			atomic.StoreInt64(idGetter.count, 0)
			mdToBlocksConverter := New()
			_, blocks, _ := mdToBlocksConverter.HTMLToBlocks([]byte(testCase.HTML))

			actualJson, err := json.Marshal(blocks)
			require.NoError(t, err)

			var actual []map[string]interface{}
			err = json.Unmarshal(actualJson, &actual)
			fmt.Println(string(actualJson))
			require.NoError(t, err)
			require.Equal(t, testCase.Blocks, actual)
		})
	}
}
