package anymark

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertBlocks(t *testing.T) {
	source := []byte("## Hello world!\n Olol*ol*olo \n\n 123123")

	md := New()

	blocks, _, err := md.MarkdownToBlocks(source, "", nil)
	if err != nil {
		t.Error(err.Error())
	}

	assert.NotEmpty(t, blocks)
	assert.NoError(t, err)
}
