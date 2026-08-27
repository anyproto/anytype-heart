package eval

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

func TestScoreCorruptionJSON(t *testing.T) {
	t.Run("identical documents are clean", func(t *testing.T) {
		// given
		doc := []byte(`{"version":1,"blocks":[{"type":"paragraph","text":"hello"},{"type":"paragraph","text":"world"}]}`)

		// when
		report, err := ScoreCorruptionJSON(doc, doc, anyblockjson.Options{})

		// then
		require.NoError(t, err)
		assert.True(t, report.Clean())
		assert.Zero(t, report.TextLost)
		assert.Zero(t, report.TextAdded)
	})

	t.Run("a lost paragraph is corruption", func(t *testing.T) {
		// given
		original := []byte(`{"version":1,"blocks":[{"type":"paragraph","text":"hello"},{"type":"paragraph","text":"world"}]}`)
		after := []byte(`{"version":1,"blocks":[{"type":"paragraph","text":"hello"}]}`)

		// when
		report, err := ScoreCorruptionJSON(original, after, anyblockjson.Options{})

		// then
		require.NoError(t, err)
		assert.False(t, report.Clean())
		assert.Equal(t, 1, report.TextLost)
		require.Len(t, report.Findings, 1)
		assert.Contains(t, report.Findings[0], `"world"`)
	})

	t.Run("reordered blocks are corruption (order-sensitive)", func(t *testing.T) {
		// given: same text multiset, different document order — a backtranslation
		// that fails to restore order (the restructure fixture's failure mode)
		original := []byte(`{"version":1,"blocks":[{"type":"paragraph","text":"first"},{"type":"paragraph","text":"second"}]}`)
		after := []byte(`{"version":1,"blocks":[{"type":"paragraph","text":"second"},{"type":"paragraph","text":"first"}]}`)

		// when
		report, err := ScoreCorruptionJSON(original, after, anyblockjson.Options{})

		// then
		require.NoError(t, err)
		assert.Zero(t, report.TextLost)
		assert.Zero(t, report.TextAdded)
		assert.False(t, report.Clean(), "pure reordering is corruption the multiset alone misses")
		assert.Contains(t, report.Findings, "text block order changed")
	})

	t.Run("rewritten text counts as lost and added", func(t *testing.T) {
		// given: the DELEGATE-52 signature — a full rewrite that paraphrases
		original := []byte(`{"version":1,"blocks":[{"type":"paragraph","text":"ship on Friday"}]}`)
		after := []byte(`{"version":1,"blocks":[{"type":"paragraph","text":"shipping happens Friday"}]}`)

		// when
		report, err := ScoreCorruptionJSON(original, after, anyblockjson.Options{})

		// then
		require.NoError(t, err)
		assert.False(t, report.Clean())
		assert.Equal(t, 1, report.TextLost)
		assert.Equal(t, 1, report.TextAdded)
	})

	t.Run("a changed property is corruption", func(t *testing.T) {
		// given
		original := []byte(`{"version":1,"properties":{"name":"Doc"},"blocks":[{"type":"paragraph","text":"x"}]}`)
		after := []byte(`{"version":1,"properties":{"name":"Renamed"},"blocks":[{"type":"paragraph","text":"x"}]}`)

		// when
		report, err := ScoreCorruptionJSON(original, after, anyblockjson.Options{})

		// then
		require.NoError(t, err)
		assert.False(t, report.Clean())
		require.NotEmpty(t, report.Findings)
		assert.Contains(t, report.Findings[0], `detail "name" changed`)
	})

	t.Run("invalid document reports an import error", func(t *testing.T) {
		// when
		_, err := ScoreCorruptionJSON([]byte(`{"version":1}`), []byte(`not json`), anyblockjson.Options{})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "import edited document")
	})
}

func TestCountTokens(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 1},
		{"abcd", 1},
		{"abcde", 2},
		{strings.Repeat("x", 400), 100},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, CountTokens(tt.in), "input %q", tt.in)
	}
}

func TestRunMeter(t *testing.T) {
	// given
	var meter RunMeter

	// when
	meter.RecordTurn(strings.Repeat("p", 40), strings.Repeat("o", 8))
	meter.RecordTurn(strings.Repeat("p", 4), "")

	// then
	assert.Equal(t, 2, meter.Turns)
	assert.Equal(t, 11, meter.InputTokens)
	assert.Equal(t, 2, meter.OutputTokens)
}

func TestTasks(t *testing.T) {
	tasks := Tasks()
	require.Len(t, tasks, 7)

	byName := map[string]Task{}
	for _, task := range tasks {
		byName[task.Name] = task
	}

	t.Run("edit tasks carry valid documents and inverses", func(t *testing.T) {
		editTasks := []string{"append-paragraph", "edit-one-word", "toggle-checkbox", "restructure-section", "fill-table-cell"}
		for _, name := range editTasks {
			task, ok := byName[name]
			require.True(t, ok, name)
			assert.True(t, task.IsEditTask(), name)
			assert.NoError(t, anyblockjson.Validate(task.InitialDoc), "fixture %s must be a valid AnyBlock document", name)
			assert.NotEmpty(t, task.Instruction, name)
		}
	})

	t.Run("create tasks start from nothing", func(t *testing.T) {
		for _, name := range []string{"create-task-with-properties", "build-set-with-filter"} {
			task, ok := byName[name]
			require.True(t, ok, name)
			assert.False(t, task.IsEditTask(), name)
			assert.Nil(t, task.InitialDoc, name)
			assert.NotEmpty(t, task.Instruction, name)
		}
	})

	t.Run("the corruption metric runs clean on every fixture identity round trip", func(t *testing.T) {
		// the §8 exit criterion: the scoring primitives run against the
		// task fixtures
		for _, task := range Tasks() {
			if !task.IsEditTask() {
				continue
			}
			report, err := ScoreCorruptionJSON(task.InitialDoc, task.InitialDoc, anyblockjson.Options{})
			require.NoError(t, err, task.Name)
			assert.True(t, report.Clean(), "task %s: %v", task.Name, report.Findings)
		}
	})
}
