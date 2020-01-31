package anymark_test

import (
	"bufio"
	"bytes"
	"fmt"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/anymark/blocksUtil"

	. "github.com/anytypeio/go-anytype-middleware/anymark"
	"github.com/anytypeio/go-anytype-middleware/anymark/renderer/html"
)

func TestConvertBlocks(t *testing.T) {
	markdown := New(WithRendererOptions(
		html.WithXHTML(),
		html.WithUnsafe(),
	))
	source := []byte("## Hello world!\n Olol*ol*olo \n\n 123123")
	var b bytes.Buffer

	writer := bufio.NewWriter(&b)
	BR := blocksUtil.NewRWriter(writer)

	err := markdown.ConvertBlocks(source, BR)
	if err != nil {
		t.Error(err.Error())
	}

	fmt.Println("rState:", BR.GetBlocks())
	fmt.Println("b:", b.String())
}
