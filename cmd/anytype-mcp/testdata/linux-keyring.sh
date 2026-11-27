#!/usr/bin/env bash
# Runs the forwarder's Secret Service tests against a real gnome-keyring, one private session bus
# per scenario: an unlocked keyring, a locked one, a keyring that is installed but not started, a bus
# with no Secret Service at all, and no bus. Needs dbus and gnome-keyring (apt: dbus gnome-keyring).
# Usage, from the repository root: bash cmd/anytype-mcp/testdata/linux-keyring.sh
set -euo pipefail

pkg=./cmd/anytype-mcp/internal/store
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
# a private home: the keyring files of these runs never touch the user's
export HOME="$work/home" XDG_DATA_HOME="$work/home/.local/share" XDG_RUNTIME_DIR="$work/run"
mkdir -p "$HOME" "$XDG_RUNTIME_DIR"
chmod 700 "$XDG_RUNTIME_DIR"
export GOCACHE="${GOCACHE:-$work/gocache}" GOFLAGS="${GOFLAGS:-}"

go test -c -o "$work/store.test" "$pkg"

unlocked='printf scratch | gnome-keyring-daemon --unlock --components=secrets >/dev/null'

echo "== unlocked keyring: the store round trip"
dbus-run-session -- bash -c "$unlocked; ANYTYPE_MCP_REAL_STORE=1 '$work/store.test' -test.run 'TestRealStore' -test.v"

echo "== locked keyring: try again, nothing destroyed"
dbus-run-session -- bash -c "$unlocked; ANYTYPE_MCP_LINUX_SCENARIO=locked '$work/store.test' -test.run 'TestSecretServiceScenario' -test.v"

echo "== keyring installed but not started: D-Bus activation, never 'no key store'"
rm -rf "$HOME/.local/share/keyrings"
dbus-run-session -- bash -c "ANYTYPE_MCP_LINUX_SCENARIO=unstarted '$work/store.test' -test.run 'TestSecretServiceScenario' -test.v"

echo "== a session bus with no Secret Service: no key store"
cat > "$work/bare.conf" <<'CONF'
<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>session</type>
  <listen>unix:tmpdir=/tmp</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>
CONF
dbus-run-session --config-file="$work/bare.conf" -- bash -c "ANYTYPE_MCP_LINUX_SCENARIO=none '$work/store.test' -test.run 'TestSecretServiceScenario' -test.v"

echo "== no session bus: try again"
DBUS_SESSION_BUS_ADDRESS="unix:path=$work/no-such-bus" ANYTYPE_MCP_LINUX_SCENARIO=unreachable "$work/store.test" -test.run 'TestSecretServiceScenario' -test.v
env -u DBUS_SESSION_BUS_ADDRESS XDG_RUNTIME_DIR="$work/empty-run" ANYTYPE_MCP_LINUX_SCENARIO=unreachable "$work/store.test" -test.run 'TestSecretServiceScenario' -test.v

echo "linux keyring scenarios: ok"
