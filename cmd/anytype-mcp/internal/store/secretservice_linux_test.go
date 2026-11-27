//go:build linux

package store

import (
	"context"
	"os"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecretServiceScenario checks what the store reports in the session
// the caller set up; cmd/anytype-mcp/testdata/linux-keyring.sh builds each
// one. ANYTYPE_MCP_LINUX_SCENARIO names it:
//
//	none         a session bus where the Secret Service name is neither owned nor activatable
//	unreachable  no session bus at the address given
//	unstarted    a keyring that is installed (activatable) but not running
//	locked       a running keyring whose default collection the test locks
func TestSecretServiceScenario(t *testing.T) {
	scenario := os.Getenv("ANYTYPE_MCP_LINUX_SCENARIO")
	if scenario == "" {
		t.Skip("set ANYTYPE_MCP_LINUX_SCENARIO (see testdata/linux-keyring.sh)")
	}
	ctx := context.Background()
	st := newPlatformStore("anytype-mcp-test")
	const id = "scenario"
	temporary := func(t *testing.T, err error, reasons ...string) {
		t.Helper()
		var te *TemporaryError
		require.ErrorAs(t, err, &te, "want a try-again error, got %v", err)
		if len(reasons) > 0 {
			assert.Contains(t, reasons, te.Reason)
		}
	}
	switch scenario {
	case "none":
		// the only state that is "this system has no key store"
		require.ErrorIs(t, st.Probe(ctx), ErrNoStore)
		_, err := st.Read(ctx, id, false)
		require.ErrorIs(t, err, ErrNoStore)
		require.ErrorIs(t, st.Write(ctx, id, []byte("x"), false), ErrNoStore)
		require.ErrorIs(t, st.Delete(ctx, id), ErrNoStore)

	case "unreachable":
		temporary(t, st.Probe(ctx), ReasonUnreachable)
		_, err := st.Read(ctx, id, false)
		temporary(t, err, ReasonUnreachable)
		temporary(t, st.Write(ctx, id, []byte("x"), false), ReasonUnreachable)
		temporary(t, st.Delete(ctx, id), ReasonUnreachable)

	case "unstarted":
		// D-Bus starts the keyring on demand. Whatever state it comes up
		// in, the answer is never "no key store", and nothing prompts.
		err := st.Probe(ctx)
		require.NotErrorIs(t, err, ErrNoStore)
		if err != nil {
			temporary(t, err)
			temporary(t, st.Write(ctx, id, []byte("x"), false))
		}
		_, err = st.Read(ctx, id, false)
		require.NotErrorIs(t, err, ErrNoStore)

	case "locked":
		require.NoError(t, st.Probe(ctx))
		require.NoError(t, st.Write(ctx, id, []byte("kept"), false))
		lockDefaultCollection(t)

		temporary(t, st.Probe(ctx), ReasonLocked)
		_, err := st.Read(ctx, id, false)
		temporary(t, err, ReasonLocked)
		temporary(t, st.Write(ctx, id, []byte("replaced"), false), ReasonLocked)
		temporary(t, st.Delete(ctx, id), ReasonLocked)
		// a connection nobody ever stored is still simply absent
		_, err = st.Read(ctx, "never-stored", false)
		require.ErrorIs(t, err, ErrNotFound)

	default:
		t.Fatalf("unknown scenario %q", scenario)
	}
}

func lockDefaultCollection(t *testing.T) {
	conn, err := dbus.SessionBusPrivate()
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.Auth(nil))
	require.NoError(t, conn.Hello())
	svc := conn.Object(ssName, ssPath)
	var path dbus.ObjectPath
	require.NoError(t, svc.Call(ssService+".ReadAlias", 0, "default").Store(&path))
	var locked []dbus.ObjectPath
	var prompt dbus.ObjectPath
	require.NoError(t, svc.Call(ssService+".Lock", 0, []dbus.ObjectPath{path}).Store(&locked, &prompt))
	require.Equal(t, ssNoPrompt, prompt)
}
