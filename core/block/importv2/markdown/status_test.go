package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
)

// The converter half of the §15 producer seam: the plan step announces
// ANALYZING (the stage ImportV2LLM.md §3 specified and nothing ever
// reported), and every page announces itself as the current item — the
// strongest not-stuck signal a long crawl has.

func TestConverterAnnouncesAnalysis(t *testing.T) {
	t.Run("the plan step brackets itself with analyzing and back to fetching", func(t *testing.T) {
		// given: a source whose containers give the planner something to do
		sink, _, _ := runConverter(t, map[string]string{
			"Tasks.csv":  "unused",
			"Tasks/a.md": "# A",
		})

		// then
		assert.Equal(t, []importv2.Phase{importv2.PhaseAnalyzing, importv2.PhaseFetching}, sink.phases)
	})

	t.Run("a source with nothing to plan announces the stage anyway", func(t *testing.T) {
		// given — the bracket is unconditional: a client that saw ANALYZING
		// begin must always see it end, whatever the planner had to do
		sink, _, _ := runConverter(t, map[string]string{"a.md": "# A"})

		// then
		assert.Equal(t, []importv2.Phase{importv2.PhaseAnalyzing, importv2.PhaseFetching}, sink.phases)
	})
}

func TestConverterAnnouncesCurrentItem(t *testing.T) {
	t.Run("every converted page names itself, in emission order", func(t *testing.T) {
		// given
		sink, _, _ := runConverter(t, map[string]string{
			"alpha.md": "# Alpha",
			"beta.md":  "# Beta",
		})

		// then: the entry name, which for markdown is a path INSIDE the
		// user's tree — user content, hence a DisplayText
		assert.Equal(t, []importv2.DisplayText{"alpha.md", "beta.md"}, sink.items)
	})
}
