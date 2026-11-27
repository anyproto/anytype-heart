//go:build darwin && cgo

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// macHome is a scratch home directory with its own login keychain. The
// Security framework finds the user's keychains through HOME, so the
// shipped binaries, run with this HOME, use the real keychain backend
// without ever seeing the developer's keychain.
type macHome struct {
	t    *testing.T
	dir  string
	path string
}

func newMacHome(t *testing.T, withKeychain bool) *macHome {
	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("no security tool")
	}
	// a short path: the keychain's lock files live next to it
	dir, err := os.MkdirTemp("", "amcp-kc-")
	require.NoError(t, err)
	h := &macHome{t: t, dir: dir, path: filepath.Join(dir, "Library", "Keychains", "login.keychain-db")}
	t.Cleanup(func() {
		if withKeychain {
			_ = h.security("delete-keychain", h.path)
		}
		_ = os.RemoveAll(dir)
	})
	if withKeychain {
		require.NoError(t, os.MkdirAll(filepath.Dir(h.path), 0o700))
		require.NoError(t, h.security("create-keychain", "-p", "scratch", h.path))
		require.NoError(t, h.security("set-keychain-settings", h.path), "no auto-lock")
		// the scratch home must not resolve to the developer's keychain
		out, err := h.output("default-keychain")
		if err != nil || !strings.Contains(out, filepath.Base(dir)) {
			t.Skipf("the scratch home does not resolve to its own keychain here (%q): not touching any other", strings.TrimSpace(out))
		}
	}
	return h
}

func (h *macHome) cmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Env = []string{"HOME=" + h.dir, "PATH=/usr/bin:/bin"}
	return cmd
}

func (h *macHome) security(args ...string) error { return h.cmd("security", args...).Run() }

func (h *macHome) output(args ...string) (string, error) {
	out, err := h.cmd("security", args...).CombinedOutput()
	return string(out), err
}

func (h *macHome) run(bin, stdin string, args ...string) result {
	h.t.Helper()
	cmd := h.cmd(bin, args...)
	return runCmd(h.t, cmd, stdin)
}

// TestMacKeychain drives the shipped builds against a real, scratch
// keychain: every verb, the two namespaces, a locked keychain and a home
// with no keychain. No prompt can appear: the binaries turn keychain
// interaction off.
func TestMacKeychain(t *testing.T) {
	_, dev, release := binaries(t)
	set := func(h *macHome, bin, expect, key string) result {
		return h.run(bin, payload(key, "AccountOne", 31009, "full"), "key", "set", "--connection", "conn-a", "--expect", expect)
	}
	check := func(h *macHome, bin string) result { return h.run(bin, "", "key", "check", "--connection", "conn-a") }

	t.Run("verbs and namespaces", func(t *testing.T) {
		h := newMacHome(t, true)
		r := check(h, dev)
		assert.Equal(t, exitNoItem, r.code, r.stderr)
		assert.Equal(t, "keychain", r.json(t)["kind"])
		assert.Equal(t, "ok", r.json(t)["store"])

		r = set(h, dev, "absent", canary)
		require.Equal(t, exitOK, r.code, r.stderr)
		first := r.json(t)
		c := check(h, dev)
		require.Equal(t, exitOK, c.code)
		assert.Equal(t, stamp(first), stamp(c.json(t)))

		// the release build can't see the development item, and its own
		// item is a separate one
		assert.Equal(t, exitNoItem, check(h, release).code)
		require.Equal(t, exitOK, set(h, release, "absent", canary+"r").code)
		assert.Equal(t, "release", check(h, release).json(t)["namespace"])
		assert.Equal(t, stamp(first), stamp(check(h, dev).json(t)), "the development item is untouched")

		// an update in place
		r = set(h, dev, stamp(first), canary+"2")
		require.Equal(t, exitOK, r.code, r.stderr)
		assert.EqualValues(t, 2, r.json(t)["rev"])
		r = h.run(dev, "", "key", "retarget", "--connection", "conn-a", "--expect", stamp(r.json(t)), "--account", "AccountOne", "--port", "40000", "--tier", "full")
		require.Equal(t, exitOK, r.code, r.stderr)
		assert.EqualValues(t, 40000, check(h, dev).json(t)["port"])

		// the stored value is the item and nothing is left in files
		out, err := h.output("find-generic-password", "-s", "Anytype MCP Dev", "-a", "conn-a", h.path)
		require.NoError(t, err, out)
		assert.Contains(t, out, `"Anytype MCP Dev"`)
		assert.NotContains(t, out, canary)

		r = h.run(dev, "", "key", "delete", "--connection", "conn-a", "--expect", stamp(r.json(t)), "--account", "AccountOne")
		assert.Equal(t, exitOK, r.code, r.stderr)
		assert.Equal(t, exitNoItem, check(h, dev).code)
		assert.Equal(t, exitOK, check(h, release).code, "the release item is untouched")
	})

	t.Run("a locked keychain is exit 7, nothing is deleted, and serve stays not connected", func(t *testing.T) {
		h := newMacHome(t, true)
		require.Equal(t, exitOK, set(h, dev, "absent", canary).code)
		st := stamp(check(h, dev).json(t))
		attrs := func() string {
			out, err := h.output("find-generic-password", "-s", "Anytype MCP Dev", "-a", "conn-a", h.path)
			require.NoError(t, err, out)
			return out
		}
		before := attrs()
		require.NoError(t, h.security("lock-keychain", h.path))

		r := check(h, dev)
		assert.Equal(t, exitTemporary, r.code)
		assert.Equal(t, "locked", r.json(t)["store"])
		assert.Equal(t, exitTemporary, set(h, dev, st, canary+"2").code)
		assert.Equal(t, exitTemporary, h.run(dev, "", "key", "retarget", "--connection", "conn-a", "--expect", st, "--account", "AccountOne", "--port", "2", "--tier", "full").code)
		assert.Equal(t, exitTemporary, h.run(dev, "", "key", "delete", "--connection", "conn-a", "--expect", st, "--account", "AccountOne").code)

		serve := h.run(dev, initLine+"\n", "serve", "--connection", "conn-a")
		assert.Equal(t, exitOK, serve.code)
		assert.Contains(t, serve.stdout, `"code":-32001`)

		// the item is still there and was not modified (its attributes,
		// modification date included, are readable while locked)
		assert.Equal(t, before, attrs())
	})

	t.Run("a home with no keychain is exit 7", func(t *testing.T) {
		h := newMacHome(t, false)
		assert.Equal(t, exitTemporary, check(h, dev).code)
		assert.Equal(t, exitTemporary, set(h, dev, "absent", canary).code)
	})
}
