package anyblockjson

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §7a's containment check walks PAST a container to find the effective parent,
// because a container becomes nothing and its children take its place. The walk
// has to skip a CHAIN, not one level: 3,121 of the corpus's 7,463 containers sit
// inside another, so a chain is the common shape rather than the exotic one.
//
// A single-step walk leaves `row > group > group > paragraph` validating, which
// imports to `row > paragraph` — and Marshal of that snapshot emits a document
// its own Validate rejects. That is the I1 hole §7a exists to close, reopened
// one level down.
//
// These fail only if the walk stops skipping: each asserts a REFUSAL on a
// document whose effective parent is reachable solely through two containers,
// so a walk that gives up after one step accepts it.
func TestValidate_ContainmentIsJudgedThroughAChainOfContainers(t *testing.T) {
	for name, doc := range map[string]string{
		"a row cannot hold a paragraph behind two containers": `{"version": 1, "type": "page", "blocks": [
			{"type": "row"},
			{"indent": 1, "type": "group"},
			{"indent": 2, "type": "group"},
			{"indent": 3, "type": "paragraph", "text": "x"}]}`,
		"a divider holds nothing, behind any number of containers": `{"version": 1, "type": "page", "blocks": [
			{"type": "divider"},
			{"indent": 1, "type": "group"},
			{"indent": 2, "type": "group"},
			{"indent": 3, "type": "paragraph", "text": "x"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			err := Validate([]byte(doc))
			require.Error(t, err, "the effective parent is reachable only through the chain")
			assert.Contains(t, err.Error(), "group",
				"and the message names the container between, or it reads as wrong to whoever wrote it")
		})
	}
}

// the control: what the chain lifts TO must still be accepted, or the test above
// could pass by refusing every chain.
func TestValidate_AChainOfContainersOverALegalChildIsFine(t *testing.T) {
	require.NoError(t, Validate([]byte(`{"version": 1, "type": "page", "blocks": [
		{"type": "row"},
		{"indent": 1, "type": "group"},
		{"indent": 2, "type": "group"},
		{"indent": 3, "type": "column"},
		{"indent": 4, "type": "paragraph", "text": "x"}]}`)),
		"row > group > group > column says row > column, which is legal")
}

// The import lift's indent arithmetic is only OBSERVABLE where import reads an
// ABSOLUTE indent — §7's structural absorption and the primary-dataview pin,
// both of which fire at indent 0. flatSubtree tolerates indent gaps when
// rebuilding a tree, so an off-by-one inside a container is invisible
// everywhere else. These pin the two shapes that make it visible: NESTED
// containers and SIBLING containers.
//
// The 160-object dataview-pin repair is part of what justifies §7a, and before
// this it was pinned for the single-container shape only.
func TestImport_TheLiftsIndentArithmeticSurvivesEveryWrappingShape(t *testing.T) {
	cases := map[string]struct{ doc, wantName, wantId string }{
		"title under two NESTED containers is absorbed": {
			doc: `{"version": 1, "type": "page", "blocks": [
				{"type": "group"}, {"indent": 1, "type": "group"},
				{"indent": 2, "type": "title", "text": "Absorbed"}]}`,
			wantName: "Absorbed",
		},
		"title under the second of two SIBLING containers is absorbed": {
			doc: `{"version": 1, "type": "page", "blocks": [
				{"type": "group"}, {"indent": 1, "type": "paragraph", "text": "a"},
				{"type": "group"}, {"indent": 1, "type": "title", "text": "Absorbed"}]}`,
			wantName: "Absorbed",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, snap, err := Unmarshal([]byte(c.doc), Options{})
			require.NoError(t, err)
			assert.Equal(t, c.wantName, snap.GetDetails().GetFields()["name"].GetStringValue(),
				"a lifted title lands at indent 0 for every purpose (§7, §7a)")
		})
	}
}

// UnmarshalBlock is the fragment surface behind replaceBlock: it addresses
// exactly ONE block, so a container — which contributes none — has to be
// refused rather than silently turned into a wrapper no read will ever show
// the caller again. This pins that CONTRACT.
//
// What it does not pin, said plainly rather than implied: the refusal inside
// blockFromJSON (import.go, the transparentBlockTypes case). Replacing that
// line with the pre-§7a behaviour leaves the whole package green, including
// this test — because UnmarshalBlock validates first and the validation side
// refuses the container on its own. Probed: with the line mutated,
// UnmarshalBlock still returns "validation failed". The line is therefore
// DEFENSIVE ONLY, unreachable through every public entry point, and a future
// reader should not delete it on the strength of a coverage report — it is the
// backstop for a new caller that skips validation.
func TestUnmarshalBlock_RefusesATransparentContainer(t *testing.T) {
	for _, spelling := range []string{
		`{"type": "group"}`,
		`{"type": "group", "background_color": "red"}`,
	} {
		blocks, err := UnmarshalBlock([]byte(spelling), "b1", Options{})
		require.Error(t, err,
			"a caller that asked for one block must be told a container is not one (§7a)")
		assert.Contains(t, err.Error(), "transparent container")
		assert.Nil(t, blocks, "and gets no Layout_Div minted behind its back")
	}
}

// the control: an ordinary single block still comes back, so the refusal above
// cannot pass by rejecting everything.
func TestUnmarshalBlock_StillReturnsAnOrdinaryBlock(t *testing.T) {
	blocks, err := UnmarshalBlock([]byte(`{"type": "paragraph", "text": "x"}`), "b1", Options{})
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, "b1", blocks[0].Id)
}
