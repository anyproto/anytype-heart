package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

// fileEnv is a key verb environment on a real file store under a scratch
// home, run in process so a stage of the write can be failed.
type fileEnv struct {
	*keyEnv
	st  *store.File
	out *bytes.Buffer
	err *bytes.Buffer
}

func newFileEnv(t *testing.T, home string) *fileEnv {
	st := store.OpenAt(home, "dev")
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	return &fileEnv{keyEnv: &keyEnv{st: st, stdout: out, stderr: errb}, st: st, out: out, err: errb}
}

func (e *fileEnv) line(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(e.out.Bytes()), &m), "stdout: %q", e.out.String())
	return m
}

func failStages(stages ...string) func(string) error {
	return func(stage string) error {
		for _, s := range stages {
			if s == stage {
				return errors.New("injected at " + stage)
			}
		}
		return nil
	}
}

func itemStamp(t *testing.T, home string) string {
	t.Helper()
	raw, err := store.OpenAt(home, "dev").Read(context.Background(), "conn-a")
	if errors.Is(err, store.ErrNotFound) {
		return "absent"
	}
	require.NoError(t, err)
	it, err := store.DecodeItem(raw)
	require.NoError(t, err)
	return it.Stamp()
}

// M3: a receipt tells the truth. An error that comes after the item may
// have been replaced or removed is settled by reading the item: in place
// is ok, unknown is unconfirmed, and exit 7 is only ever given when the
// item provably did not change.
func TestReceiptsTellTheTruth(t *testing.T) {
	seed := func(t *testing.T) (home, stamp string) {
		home = t.TempDir()
		e := newFileEnv(t, home)
		require.Equal(t, exitOK, e.run("set", keyFlags{connection: "conn-a", expect: "absent"},
			bytes.NewReader([]byte(payload(canary, "A", 1, "small")))))
		return home, itemStamp(t, home)
	}
	verbs := map[string]struct {
		stage string
		run   func(e *fileEnv, stamp string) int
		after func(t *testing.T, home, before string)
	}{
		"set": {"rename-result", func(e *fileEnv, stamp string) int {
			return e.run("set", keyFlags{connection: "conn-a", expect: stamp}, bytes.NewReader([]byte(payload(canary+"2", "A", 1, "small"))))
		}, func(t *testing.T, home, before string) {
			assert.Equal(t, store.Fingerprint(canary+"2")+"@2", itemStamp(t, home))
		}},
		"retarget": {"rename-result", func(e *fileEnv, stamp string) int {
			return e.run("retarget", keyFlags{connection: "conn-a", expect: stamp, account: "A", port: 2, tier: "small"}, nil)
		}, func(t *testing.T, home, before string) {
			assert.Equal(t, store.Fingerprint(canary)+"@2", itemStamp(t, home))
		}},
		"delete": {"unlink-result", func(e *fileEnv, stamp string) int {
			return e.run("delete", keyFlags{connection: "conn-a", expect: stamp, account: "A"}, nil)
		}, func(t *testing.T, home, before string) {
			assert.Equal(t, "absent", itemStamp(t, home))
		}},
	}
	for name, v := range verbs {
		t.Run(name+": the operation reported an error but took effect: ok", func(t *testing.T) {
			home, stamp := seed(t)
			e := newFileEnv(t, home)
			e.st.FailAt(failStages(v.stage))
			assert.Equal(t, exitOK, v.run(e, stamp), e.err.String())
			assert.Equal(t, true, e.line(t)["ok"])
			v.after(t, home, stamp)
		})

		t.Run(name+": it took effect and the item can't be read back: unconfirmed, never exit 7", func(t *testing.T) {
			home, stamp := seed(t)
			e := newFileEnv(t, home)
			e.st.FailAt(failStages(v.stage, "holds"))
			// nor can the verb read it afterwards
			e.keyEnv.st = &blindAfterChange{Store: e.st}
			code := v.run(e, stamp)
			assert.Equal(t, exitInternal, code, "not exit 7: the change happened")
			assert.Equal(t, true, e.line(t)["unconfirmed"])
			v.after(t, home, stamp)
		})

		t.Run(name+": a later sync fails: the item is in place, so ok", func(t *testing.T) {
			home, stamp := seed(t)
			e := newFileEnv(t, home)
			e.st.FailAt(failStages("after-rename", "after-unlink"))
			assert.Equal(t, exitOK, v.run(e, stamp), e.err.String())
			v.after(t, home, stamp)
		})
	}

	t.Run("a failure before the rename is exit 7 and the old item is provably there", func(t *testing.T) {
		home, stamp := seed(t)
		e := newFileEnv(t, home)
		e.st.FailAt(failStages("before-rename"))
		code := e.run("set", keyFlags{connection: "conn-a", expect: stamp}, bytes.NewReader([]byte(payload(canary+"2", "A", 1, "small"))))
		assert.Equal(t, exitTemporary, code)
		assert.Equal(t, stamp, itemStamp(t, home))
		assert.NotContains(t, e.out.String()+e.err.String(), canary)
	})
}

// blindAfterChange can't read the item once a write or a delete was tried.
type blindAfterChange struct {
	store.Store
	changed bool
}

func (s *blindAfterChange) Write(ctx context.Context, id string, value []byte) error {
	s.changed = true
	return s.Store.Write(ctx, id, value)
}

func (s *blindAfterChange) Delete(ctx context.Context, id string) error {
	s.changed = true
	return s.Store.Delete(ctx, id)
}

func (s *blindAfterChange) Read(ctx context.Context, id string) ([]byte, error) {
	if s.changed {
		return nil, store.Temporary(store.ReasonUnreachable, errors.New("stale handle"))
	}
	return s.Store.Read(ctx, id)
}

// slowStore delays a call of the store.
type slowStore struct {
	store.Store
	beforeRead  func()
	beforeWrite func()
}

func (s *slowStore) Read(ctx context.Context, id string) ([]byte, error) {
	if s.beforeRead != nil {
		s.beforeRead()
	}
	return s.Store.Read(ctx, id)
}

func (s *slowStore) Write(ctx context.Context, id string, value []byte) error {
	if s.beforeWrite != nil {
		s.beforeWrite()
	}
	return s.Store.Write(ctx, id, value)
}

// M2: the verb deadline covers stdin, the lock and the store's I/O.
func TestKeyVerbsAreBounded(t *testing.T) {
	short := func(t *testing.T, d time.Duration) {
		oldVerb, oldGrace := verbTimeout, commitGrace
		verbTimeout, commitGrace = d, d
		t.Cleanup(func() { verbTimeout, commitGrace = oldVerb, oldGrace })
	}
	forever := make(chan struct{})
	t.Cleanup(func() { close(forever) })

	t.Run("key set with a stdin that is never closed: timeout, exit 7, nothing written", func(t *testing.T) {
		short(t, 200*time.Millisecond)
		home := t.TempDir()
		e := newFileEnv(t, home)
		stdin, w := io.Pipe()
		t.Cleanup(func() { _ = w.Close() })
		began := time.Now()
		code := e.run("set", keyFlags{connection: "conn-a", expect: "absent"}, stdin)
		assert.Equal(t, exitTemporary, code)
		assert.Less(t, time.Since(began), 3*time.Second)
		assert.Equal(t, false, e.line(t)["ok"])
		assert.Contains(t, e.line(t)["reason"], "timed out")
		_, err := os.Lstat(filepath.Join(home, ".anytype"))
		assert.ErrorIs(t, err, os.ErrNotExist, "nothing was created")
	})

	t.Run("a held lock: the verb ends by its deadline with exit 7", func(t *testing.T) {
		short(t, 300*time.Millisecond)
		home := t.TempDir()
		holder := store.OpenAt(home, "dev")
		unlock, err := holder.Lock(context.Background(), "conn-a")
		require.NoError(t, err)
		defer unlock()
		e := newFileEnv(t, home)
		code := e.run("set", keyFlags{connection: "conn-a", expect: "absent"}, bytes.NewReader([]byte(payload(canary, "A", 1, "small"))))
		assert.Equal(t, exitTemporary, code)
		assert.Equal(t, "absent", itemStamp(t, home))
	})

	t.Run("a store that stalls before the write: exit 7, and nothing is written after the deadline", func(t *testing.T) {
		short(t, 200*time.Millisecond)
		home := t.TempDir()
		e := newFileEnv(t, home)
		resume := make(chan struct{})
		e.keyEnv.st = &slowStore{Store: e.st, beforeWrite: func() { <-resume }}
		code := e.run("set", keyFlags{connection: "conn-a", expect: "absent"}, bytes.NewReader([]byte(payload(canary, "A", 1, "small"))))
		assert.Equal(t, exitTemporary, code)
		// the stalled goroutine wakes up after the verb has answered
		close(resume)
		time.Sleep(200 * time.Millisecond)
		assert.Equal(t, "absent", itemStamp(t, home), "the late write was stopped at its commit point")
		assert.Len(t, bytes.Split(bytes.TrimSpace(e.out.Bytes()), []byte("\n")), 1, "one JSON line, the timeout")
		entries, _ := os.ReadDir(e.st.Dir())
		for _, entry := range entries {
			assert.NotContains(t, entry.Name(), ".tmp", "no key-bearing temp file is left")
		}
	})

	t.Run("a store that stalls after the item was replaced: unconfirmed, not exit 7", func(t *testing.T) {
		short(t, 200*time.Millisecond)
		home := t.TempDir()
		e := newFileEnv(t, home)
		reads := 0
		// the read-back after the write never returns
		e.keyEnv.st = &slowStore{Store: e.st, beforeRead: func() {
			reads++
			if reads > 1 {
				<-forever
			}
		}}
		code := e.run("set", keyFlags{connection: "conn-a", expect: "absent"}, bytes.NewReader([]byte(payload(canary, "A", 1, "small"))))
		assert.Equal(t, exitInternal, code)
		assert.Equal(t, true, e.line(t)["unconfirmed"])
		assert.Equal(t, store.Fingerprint(canary)+"@1", itemStamp(t, home), "the write did happen")
	})
}
