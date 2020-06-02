package anymark_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anytypeio/go-anytype-middleware/anymark"
)

type MdCase struct {
	MD string `json:"md"`
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

	for testNum, _ := range testCases {
		mdToBlocksConverter := anymark.New()
		fmt.Println("TEST CASE:\n\n", testCases[testNum].MD, "\n   ***   ")
		blocks, _ := mdToBlocksConverter.MarkdownToBlocks([]byte(testCases[testNum].MD), "", []string{})

		for i, b := range blocks {
			fmt.Println(i, ": ", b)
			assert.NotEmpty(t, b)
		}
	}
}
