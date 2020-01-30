package anymark_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/anymark"
	"github.com/anytypeio/go-anytype-middleware/anymark/blocksUtil"
	htmlToMdConverter "github.com/anytypeio/html-to-markdown"
)

var (
	pasteCmdArgs = "pbpaste"
	copyCmdArgs  = "pbcopy"
)

func getPasteCommand() *exec.Cmd {
	return exec.Command(pasteCmdArgs)
}

func readAll() (string, error) {
	pasteCmd := getPasteCommand()
	out, err := pasteCmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

type TestCase struct {
	HTML string `json:"html"`
}

func TestConvertHTMLToBlocks(t *testing.T) {
	bs, err := ioutil.ReadFile("_test/spec.json")
	if err != nil {
		panic(err)
	}
	var testCases []TestCase
	if err := json.Unmarshal(bs, &testCases); err != nil {
		panic(err)
	}

	for _, c := range testCases {
		convertToBlocksAndPrint(c.HTML)
	}
}

func TestConvertHTMLToBlocks2(t *testing.T) {
	bs, err := ioutil.ReadFile("_test/testData.json")
	if err != nil {
		panic(err)
	}
	var testCases []TestCase
	if err := json.Unmarshal(bs, &testCases); err != nil {
		panic(err)
	}

	testNum := 9
	s := testCases[testNum].HTML

	mdToBlocksConverter := anymark.New()
	_, blocks := mdToBlocksConverter.HTMLToBlocks([]byte(s))
	fmt.Println("html:", testCases[testNum].HTML)
	for i, b := range blocks {
		fmt.Println(i, " block: ", b)
	}

}

func convertToBlocksAndPrint(html string) error {
	mdToBlocksConverter := anymark.New()

	converter := htmlToMdConverter.NewConverter("", true, nil)
	md, err := converter.ConvertString(html)
	if err != nil {
		log.Fatal(err)
	}
	//fmt.Println("md ->", md)

	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	BR := blocksUtil.NewRWriter(writer)

	err = mdToBlocksConverter.ConvertBlocks([]byte(md), BR)
	if err != nil {
		return err
	}

	fmt.Println("blocks:", BR.GetBlocks())
	return nil
}
