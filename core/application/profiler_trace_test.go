package application

import (
	"archive/zip"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zipEntryNames opens the archive at path and returns the set of entry names.
func zipEntryNames(t *testing.T, path string) map[string]struct{} {
	t.Helper()
	r, err := zip.OpenReader(path)
	require.NoError(t, err)
	defer r.Close()
	names := make(map[string]struct{}, len(r.File))
	for _, f := range r.File {
		names[f.Name] = struct{}{}
	}
	return names
}

// TestRunTimedProfiler_TraceGating verifies that the runtime execution trace
// (and the retired account_select_trace) are absent by default and that the
// trace is included only when includeTrace is set. A pre-cancelled context is
// passed so the profiling window is ~0s and the test stays fast.
func TestRunTimedProfiler_TraceGating(t *testing.T) {
	// Always-present captures regardless of the includeTrace flag.
	baseEntries := []string{
		"cpu_profile",
		"heap_start",
		"heap_end",
		"goroutines_start.txt",
		"goroutines_end.txt",
		"info.json",
	}

	newService := func() *Service { return &Service{} }

	cancelledCtx := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}

	t.Run("trace off by default: no trace, no account_select_trace", func(t *testing.T) {
		// given
		dir := t.TempDir()
		s := newService()

		// when
		path, err := s.runTimedProfiler(cancelledCtx(), dir, 1, "USER_REQUEST", "test", false)

		// then
		require.NoError(t, err)
		require.NotEmpty(t, path)
		names := zipEntryNames(t, path)
		for _, want := range baseEntries {
			assert.Contains(t, names, want, "expected base entry %q", want)
		}
		assert.NotContains(t, names, "trace", "runtime trace must be absent when includeTrace=false")
		assert.NotContains(t, names, "account_select_trace", "account_select_trace must never be bundled")
	})

	t.Run("includeTrace adds the runtime trace but never account_select_trace", func(t *testing.T) {
		// given
		dir := t.TempDir()
		s := newService()

		// when
		path, err := s.runTimedProfiler(cancelledCtx(), dir, 1, "USER_REQUEST", "test", true)

		// then
		require.NoError(t, err)
		require.NotEmpty(t, path)
		names := zipEntryNames(t, path)
		for _, want := range baseEntries {
			assert.Contains(t, names, want, "expected base entry %q", want)
		}
		assert.Contains(t, names, "trace", "runtime trace must be present when includeTrace=true")
		assert.NotContains(t, names, "account_select_trace", "account_select_trace must never be bundled")
	})
}
