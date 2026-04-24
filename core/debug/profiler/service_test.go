package profiler

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/debug/debugreporter"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/initialparams"
)

// recordingSender is a minimal event.Sender stub that captures Broadcast
// payloads for assertions. event.Sender has many methods we don't exercise,
// but the profiler's Report path only touches Broadcast.
type recordingSender struct {
	mu     sync.Mutex
	events []*pb.Event
}

func (s *recordingSender) Broadcast(e *pb.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
}

func (s *recordingSender) BroadcastToOtherSessions(string, *pb.Event)  {}
func (s *recordingSender) BroadcastExceptSessions(*pb.Event, []string) {}
func (s *recordingSender) SendToSession(string, *pb.Event)             {}
func (s *recordingSender) IsActive(string) bool                        { return true }
func (s *recordingSender) Init(a *app.App) error                       { return nil }
func (s *recordingSender) Name() string                                { return "recordingSender" }

func TestReport(t *testing.T) {
	setupProfilesDir := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		profiles := filepath.Join(dir, "common", "profiles")
		require.NoError(t, os.MkdirAll(profiles, 0755))
		// initialparams is a process-global; use the test-only helper to
		// load a minimal Params and reset it afterwards.
		initialparams.SetForTest(initialparams.Params{
			Paths: initialparams.Paths{
				Workdir:     dir,
				Common:      filepath.Join(dir, "common"),
				ProfilesDir: profiles,
			},
		})
		t.Cleanup(initialparams.ResetForTest)
		return profiles
	}

	t.Run("KindNone broadcasts event with no artifact and empty path", func(t *testing.T) {
		profilesDir := setupProfilesDir(t)
		sender := &recordingSender{}
		s := &service{sender: sender}

		s.Report("DB_CORRUPTION", map[string]any{
			"table": "objects",
			"err":   "checksum mismatch",
		}, debugreporter.Capture{Kind: debugreporter.KindNone})

		entries, err := os.ReadDir(profilesDir)
		require.NoError(t, err)
		assert.Empty(t, entries, "KindNone must not write an archive")

		require.Len(t, sender.events, 1)
		msg := sender.events[0].Messages[0].GetDebugProfileCreated()
		require.NotNil(t, msg)
		assert.Equal(t, "DB_CORRUPTION", msg.Reason)
		assert.JSONEq(t, `{"err":"checksum mismatch","table":"objects"}`, msg.ReasonDesc)
		assert.Empty(t, msg.Path)
		assert.False(t, msg.Full)
	})

	t.Run("KindHeap writes snapshot zip and broadcasts its path", func(t *testing.T) {
		profilesDir := setupProfilesDir(t)
		sender := &recordingSender{}
		s := &service{sender: sender}

		s.Report("MEMORY_GROWTH", map[string]any{"sysMemory": 1234567}, debugreporter.Capture{Kind: debugreporter.KindHeap})

		entries, err := os.ReadDir(profilesDir)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.True(t, strings.HasPrefix(entries[0].Name(), "snapshot_"))
		assert.True(t, strings.HasSuffix(entries[0].Name(), ".zip"))

		require.Len(t, sender.events, 1)
		msg := sender.events[0].Messages[0].GetDebugProfileCreated()
		assert.Equal(t, "MEMORY_GROWTH", msg.Reason)
		assert.JSONEq(t, `{"sysMemory":1234567}`, msg.ReasonDesc)
		assert.Equal(t, filepath.Join(profilesDir, entries[0].Name()), msg.Path)

		// The archive's info.json must carry the reason + reasonDesc.
		archive, err := zip.OpenReader(msg.Path)
		require.NoError(t, err)
		defer archive.Close()
		var infoRaw []byte
		for _, f := range archive.File {
			if f.Name != "info.json" {
				continue
			}
			rc, err := f.Open()
			require.NoError(t, err)
			infoRaw, err = io.ReadAll(rc)
			require.NoError(t, err)
			rc.Close()
		}
		require.NotEmpty(t, infoRaw, "info.json missing from snapshot archive")
		var parsed map[string]any
		require.NoError(t, json.Unmarshal(infoRaw, &parsed))
		assert.Equal(t, "MEMORY_GROWTH", parsed["reason"])
		assert.Equal(t, `{"sysMemory":1234567}`, parsed["reasonDesc"])
	})

	t.Run("empty extras produce an omitted reasonDesc", func(t *testing.T) {
		setupProfilesDir(t)
		sender := &recordingSender{}
		s := &service{sender: sender}

		s.Report("STARTUP", nil, debugreporter.Capture{Kind: debugreporter.KindNone})

		require.Len(t, sender.events, 1)
		msg := sender.events[0].Messages[0].GetDebugProfileCreated()
		assert.Equal(t, "STARTUP", msg.Reason)
		assert.Empty(t, msg.ReasonDesc)
	})

	t.Run("marshal failure still broadcasts event", func(t *testing.T) {
		setupProfilesDir(t)
		sender := &recordingSender{}
		s := &service{sender: sender}

		// channels are not JSON-marshalable
		s.Report("BAD_EXTRAS", map[string]any{"ch": make(chan int)}, debugreporter.Capture{Kind: debugreporter.KindNone})

		require.Len(t, sender.events, 1)
		msg := sender.events[0].Messages[0].GetDebugProfileCreated()
		assert.Equal(t, "BAD_EXTRAS", msg.Reason)
		assert.Empty(t, msg.ReasonDesc, "marshal failure must fall back to empty reasonDesc")
	})

	t.Run("nil sender is a no-op", func(t *testing.T) {
		setupProfilesDir(t)
		s := &service{sender: nil}
		// Must not panic — used before Init wires the sender.
		s.Report("EARLY", nil, debugreporter.Capture{Kind: debugreporter.KindNone})
	})
}
