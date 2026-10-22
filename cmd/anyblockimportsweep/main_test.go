package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/stretchr/testify/require"
)

func TestDiscoverRepairedAndEmptySpaces(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "repaired")
	for _, name := range []string{"first/a", "first/b", "repaired/a"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, name), 0700))
	}
	require.NoError(t, os.WriteFile(filepath.Join(second, "a", "index.json"), []byte(`{}`), 0600))
	got, err := discover([]string{first, second})
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, filepath.Join(second, "a"), got[0].Path)
	require.True(t, got[0].HasIndex)
	require.False(t, got[1].HasIndex)
}

func TestAwaitImportCorrelatesCompletion(t *testing.T) {
	notification := &model.Notification{Payload: &model.NotificationPayloadOfImport{Import: &model.NotificationImport{SpaceId: "target", ProcessId: "our-process", ErrorCode: model.ImportErrorCode(1)}}}
	process := &pb.ModelProcess{Id: "our-process", Error: "invalid input"}
	for _, reverse := range []bool{false, true} {
		events := make(chan received, 4)
		events <- received{Done: &pb.ModelProcess{Id: "other", Error: "unrelated error"}}
		events <- received{Notification: &model.Notification{Payload: &model.NotificationPayloadOfImport{Import: &model.NotificationImport{SpaceId: "other-space", ProcessId: "other"}}}}
		if reverse {
			events <- received{Notification: notification}
			events <- received{Done: process}
		} else {
			events <- received{Done: process}
			events <- received{Notification: notification}
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		done, n, err := awaitImport(ctx, events, "target")
		cancel()
		require.NoError(t, err)
		require.Equal(t, process, done)
		require.Equal(t, notification.GetImport(), n)
	}
}

func TestAwaitImportRequiresBothCompletionEvents(t *testing.T) {
	for _, only := range []received{
		{Done: &pb.ModelProcess{Id: "our-process"}},
		{Notification: &model.Notification{Payload: &model.NotificationPayloadOfImport{Import: &model.NotificationImport{SpaceId: "target", ProcessId: "our-process"}}}},
	} {
		events := make(chan received, 1)
		events <- only
		close(events)
		_, _, err := awaitImport(context.Background(), events, "target")
		require.ErrorContains(t, err, "event stream closed")
	}
}

// TestSweep is intentionally opt-in: input exports can be large and private.
// It always creates a fresh account; it cannot log into the source account.
func TestSweep(t *testing.T) {
	inputs := os.Getenv("ANYBLOCK_SWEEP_INPUTS")
	if inputs == "" {
		t.Skip("set ANYBLOCK_SWEEP_INPUTS to per-space export roots, separated by the OS path-list separator")
	}
	output := os.Getenv("ANYBLOCK_SWEEP_OUTPUT")
	if output == "" {
		output = filepath.Join(t.TempDir(), "report")
	}
	r, err := run(context.Background(), options{Inputs: filepath.SplitList(inputs), Output: output, Keep: os.Getenv("ANYBLOCK_SWEEP_KEEP_ACCOUNT") == "1", Timeout: 10 * time.Minute})
	if r != nil {
		for _, row := range r.Results {
			if len(row.Errors) > 0 {
				t.Logf("%s: %+v", row.Input.Path, row.Errors)
			}
		}
	}
	require.NoError(t, err, "report: %s", output)
}

func TestImportLogErrorsIncludesSwallowedFailures(t *testing.T) {
	got := importLogErrors([]string{
		`{"level":"ERROR","logger":"import","msg":"file uploading failed","error":"missing bytes"}`,
		`{"level":"WARN","logger":"import-anyblock","msg":"missing definition"}`,
		`{"level":"ERROR","logger":"filesync","msg":"offline"}`,
	})
	require.Equal(t, []issue{{Stage: "import_log", Message: "file uploading failed: missing bytes"}}, got)
}

func TestDeletedSpaceManifestSkipsBeforeStartingHeart(t *testing.T) {
	root := t.TempDir()
	native := filepath.Join(root, "native")
	require.NoError(t, os.MkdirAll(filepath.Join(native, "deleted"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "skipped-spaces.json"), []byte(`{"version":1,"spaces":[{"spaceId":"deleted","reason":"accountStatus: Deleted"}]}`), 0600))
	inputs, err := discover([]string{native})
	require.NoError(t, err)
	require.Len(t, inputs, 1)
	require.Equal(t, "accountStatus: Deleted", inputs[0].SkipReason)
	r, err := run(context.Background(), options{Inputs: []string{native}, Output: filepath.Join(root, "report"), Binary: "/does-not-exist"})
	require.NoError(t, err)
	require.Empty(t, r.AccountID)
	require.Equal(t, "skipped", r.Results[0].Status)
	require.Empty(t, r.Results[0].SpaceID)
	require.Empty(t, r.Results[0].Errors)
	// A repaired export in a later root is independent of the earlier skip manifest.
	repaired := filepath.Join(t.TempDir(), "native")
	require.NoError(t, os.MkdirAll(filepath.Join(repaired, "deleted"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(repaired, "deleted", "index.json"), []byte(`{}`), 0600))
	inputs, err = discover([]string{native, repaired})
	require.NoError(t, err)
	require.Empty(t, inputs[0].SkipReason)
}
