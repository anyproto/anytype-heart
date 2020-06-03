package anymark

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anytypeio/go-anytype-middleware/anymark/blocksUtil"

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
	BR := blocksUtil.NewRWriter(writer, "", []string{}, defaultIdGetter)

	err := markdown.ConvertBlocks(source, BR)
	if err != nil {
		t.Error(err.Error())
	}

	assert.NotEmpty(t, BR.GetBlocks())
}
