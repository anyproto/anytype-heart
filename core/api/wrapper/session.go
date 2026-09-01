package wrapper

// session.go — the handle state (§7.4, fully stated there): find writes
// numbered handles and the working space; the id-taking tools read them;
// each new find renumbers. The CLI persists the session in a file because a
// CLI invocation is a fresh process; the MCP/long-lived delivery keeps the
// same state in memory — the Store interface is that seam.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Handle is one enumerated find result.
type Handle struct {
	N    int    `json:"n"`
	Id   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// LastWrite remembers the Idempotency-Key of the most recent mutation, so
// a repeat of the identical RESOLVED request within the reuse window (a
// regenerated small-model retry, or a harness re-running the CLI after a
// timeout or failure) carries the SAME key and replays instead of
// double-applying (C8). Hash is requestHash over method + path + query +
// body — the server's own identity — so a dry run and its real twin, or
// one tool call re-addressed by a re-find, never share a key. A repeat
// after the window is treated as intentional and applies again — as is one
// whose stamp sits in the FUTURE (a backwards clock step; the record
// persists in the CLI session file, so it can outlive the clock that wrote
// it — runner.withinReuseWindow).
type LastWrite struct {
	Hash string    `json:"hash"`
	Key  string    `json:"key"`
	At   time.Time `json:"at"`
	// PriorHash/Rewrites record an ambiguity-retry rewrite (runner.patchOps):
	// the request was re-addressed mid-flight AFTER its key was minted, so an
	// identical re-run computes the PRE-rewrite hash. To replay instead of
	// 409ing (the server binds the key to the REWRITTEN body) the re-run must
	// reproduce the rewrite: PriorHash identifies it, Rewrites replays it.
	// Both are scoped to the one LastWrite entry and its reuse window.
	PriorHash string            `json:"priorHash,omitempty"`
	Rewrites  map[string]string `json:"rewrites,omitempty"`
}

// idempotencyReuseWindow bounds LastWrite key reuse.
const idempotencyReuseWindow = 60 * time.Second

// Session is the wrapper's per-conversation state.
type Session struct {
	// Space is the working space — set by the last find; the id-taking
	// tools resolve raw object ids against it.
	Space string `json:"space,omitempty"`
	// Handles are the last find's enumerated results.
	Handles []Handle `json:"handles,omitempty"`
	// Me caches spaceId → the caller's participant id (@me resolution).
	Me map[string]string `json:"me,omitempty"`
	// LastWrite is the idempotency reuse record.
	LastWrite *LastWrite `json:"lastWrite,omitempty"`
}

// registerHandle makes an object addressable without a find, and returns
// the number it was given. create's result is the ONE object the caller
// definitely meant, so it earns a handle the way a search hit does: §8.33's
// hazard is an UNRANKED listing, where handle 1 is whichever object the
// index happened to return first, not an object the caller just made.
//
// Numbering continues within the working space and restarts when the space
// changes, because a handle resolves through Space — a number carried
// across spaces would address the wrong object.
func (s *Session) registerHandle(space string, h Handle) int {
	if s.Space != space {
		s.Space = space
		s.Handles = nil
	}
	h.N = len(s.Handles) + 1
	s.Handles = append(s.Handles, h)
	return h.N
}

// handleById resolves a handle number.
func (s *Session) handle(n int) (Handle, bool) {
	for _, h := range s.Handles {
		if h.N == n {
			return h, true
		}
	}
	return Handle{}, false
}

// Store persists the session between tool calls.
type Store interface {
	Load() (*Session, error)
	Save(*Session) error
}

// MemoryStore keeps the session in memory (the MCP / long-lived delivery,
// and tests). Load hands out a deep COPY under a lock — handing out the
// stored pointer would alias every caller's session and make concurrent
// tool calls race on its maps (Me).
type MemoryStore struct {
	mu   sync.Mutex
	data []byte // the session, JSON-encoded (the copy mechanism)
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

func (m *MemoryStore) Load() (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		return &Session{}, nil
	}
	var s Session
	if err := json.Unmarshal(m.data, &s); err != nil {
		return nil, fmt.Errorf("decode stored session: %w", err)
	}
	return &s, nil
}

func (m *MemoryStore) Save(s *Session) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode session: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = data
	return nil
}

// FileStore persists the session as a JSON file (the process-per-invocation
// CLI delivery).
type FileStore struct {
	Path string
}

// DefaultSessionPath is the CLI's session file location.
func DefaultSessionPath() (string, error) {
	if p := os.Getenv("ANYTYPE_CLI_SESSION"); p != "" {
		return p, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve cache dir: %w", err)
	}
	return filepath.Join(dir, "anytype-cli", "session.json"), nil
}

func (f *FileStore) Load() (*Session, error) {
	data, err := os.ReadFile(f.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Session{}, nil
		}
		return nil, fmt.Errorf("read session file: %w", err)
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		// a corrupt session file must never brick the CLI — start fresh
		return &Session{}, nil
	}
	return &s, nil
}

// Save writes atomically (temp file + rename): a concurrent CLI invocation
// may last-write-win the whole session, but it can never observe a torn
// half-written file.
func (f *FileStore) Save(s *Session) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode session: %w", err)
	}
	dir := filepath.Dir(f.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".session-*.tmp")
	if err != nil {
		return fmt.Errorf("create session temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write session temp file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("chmod session temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close session temp file: %w", err)
	}
	if err := os.Rename(tmpName, f.Path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename session file: %w", err)
	}
	return nil
}
