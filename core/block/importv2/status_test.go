package importv2

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhaseMessage(t *testing.T) {
	t.Run("every phase carries the legacy progress message it replaces", func(t *testing.T) {
		// given: the strings the pre-redesign Reporter.Phase call sites passed as
		// free text (engine.go:564,594,775 / spool.go:231), plus the
		// ANALYZING message the plan phase needed and nothing ever
		// emitted
		want := map[Phase]string{
			PhaseScanning:   "Scanning source",
			PhaseAnalyzing:  "Analyzing structure",
			PhaseFetching:   "Fetching content",
			PhaseCreating:   "Creating objects",
			PhaseFinalizing: "Finalizing",
		}

		// then
		for phase, message := range want {
			assert.Equal(t, message, phase.String())
		}
	})
}

func TestDisplayTextIsNeverLoggable(t *testing.T) {
	t.Run("rendering user content through fmt yields the hash, not the title", func(t *testing.T) {
		// given: the rule — currentItem is user content, displayable
		// but NEVER loggable — carried by a type whose fmt rendering follows
		// the codebase's existing logging-hygiene rule for user text (v1
		// notion's hashText, core/block/import/notion/api/block/link.go)
		title := DisplayText("Q3 Planning — salary review")

		// when: the ways a title reaches a log line by accident
		direct := fmt.Sprintf("%s", title)
		valued := fmt.Sprintf("%v", title)
		wrapped := fmt.Sprintf("%v", struct{ Item DisplayText }{Item: title})
		logged := fmt.Errorf("fetch %s: boom", title).Error()

		// then
		for _, rendered := range []string{direct, valued, wrapped, logged} {
			assert.NotContains(t, rendered, "Q3 Planning")
			assert.NotContains(t, rendered, "salary review")
		}
		assert.Equal(t, title.Hash(), direct)

		// and: the wire value stays reachable through the explicit accessor
		assert.Equal(t, "Q3 Planning — salary review", title.Display())
	})

	t.Run("the hash is stable and empty text stays empty", func(t *testing.T) {
		// given
		title := DisplayText("Meeting notes")

		// then
		assert.Equal(t, DisplayText("Meeting notes").Hash(), title.Hash())
		assert.NotEqual(t, DisplayText("Meeting note").Hash(), title.Hash())
		require.Empty(t, DisplayText("").Display())
		assert.Empty(t, DisplayText("").Hash(), "an empty item must not render as a hash of nothing")
	})
}
