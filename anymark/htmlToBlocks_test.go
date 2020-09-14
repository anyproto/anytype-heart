package anymark

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
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

func replaceFakeIds(anySlot []*model.Block) (anySlotConverted []*model.Block) {
	var oldToNew map[string]string
	oldToNew = make(map[string]string)

	for i, _ := range anySlot {
		var oldId = make([]byte, len(anySlot[i].Id))

		newId := fmt.Sprintf("%d", i+1)

		copy(oldId, anySlot[i].Id)
		oldToNew[string(oldId)] = newId
		anySlot[i].Id = newId
	}

	for i, _ := range anySlot {
		cIds := []string{}
		for _, cId := range anySlot[i].ChildrenIds {
			if len(oldToNew[cId]) > 0 {
				cIds = append(cIds, oldToNew[cId])
			}
		}
		anySlot[i].ChildrenIds = cIds
	}

	return anySlot
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

	var dumpTests = os.Getenv("DUMP_TESTS") == "1"
	var dumpPath string
	if dumpTests {
		dumpPath = filepath.Join("_test", "html")
		os.MkdirAll(dumpPath, 0700)
	}

	for _, testCase := range testCases {
		t.Run(testCase.Desc, func(t *testing.T) {
			mdToBlocksConverter := New()
			_, blocks, _ := mdToBlocksConverter.HTMLToBlocks([]byte(testCase.HTML))
			blocks = replaceFakeIds(blocks)

			actualJson, err := json.Marshal(blocks)
			require.NoError(t, err)

			var actual []map[string]interface{}
			err = json.Unmarshal(actualJson, &actual)
			if dumpTests {
				ioutil.WriteFile(filepath.Join(dumpPath, filepath.Clean(testCase.Desc)+".html"), []byte(testCase.HTML), 0644)
			}
			require.NoError(t, err)

			if !reflect.DeepEqual(testCase.Blocks, actual) {
				fmt.Println("real output:\n", string(actualJson))
				fmt.Println("expected:\n", testCase.Blocks)
				require.Equal(t, testCase.Blocks, actual)
			}
		})
	}
}
