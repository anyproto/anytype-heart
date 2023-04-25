package blockbuilder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuilder(t *testing.T) {
	makeTree := func() *Block {
		return Text("kek", Children(
			Text("level 2", Color("red")),
			Text("level 2.1", Children(
				Text("level 3.1"), Text("level 3.2"), Text("level 3.3"))),
		))
	}

	b := makeTree()
	blocks := b.Build()

	root := BuildAST(blocks)

	assert.Equal(t, b, root)

}

func TestTreesEquality(t *testing.T) {
	makeTree := func() *Block {
		root := Text("level 1", Children(
			Text("level 2", Color("red")),
			Text("level 2.1", Children(
				Text("level 3.1"), Text("level 3.2"), Text("level 3.3"))),
		))

		return BuildAST(root.Build())
	}
	a := makeTree()
	b := makeTree()

	AssertPagesEqual(t, a.Build(), b.Build())
}
