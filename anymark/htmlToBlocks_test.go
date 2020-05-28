package anymark_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/anymark"
	"github.com/stretchr/testify/assert"
)

var (
	pasteCmdArgs = "pbpaste"
	copyCmdArgs  = "pbcopy"
)

type TestCase struct {
	HTML string `json:"html"`
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

	for testNum, _ := range testCases {
		fmt.Println("Case ", testNum)
		//if testNum != 13 {
		//	continue
		//}
		mdToBlocksConverter := anymark.New()
		_, blocks := mdToBlocksConverter.HTMLToBlocks([]byte(testCases[testNum].HTML))

		for _, b := range blocks {
			fmt.Println(b)
			assert.NotEmpty(t, b)
		}
	}
}
