#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

export GOOS=linux
export GOARCH=amd64

tags="nogrpcserver gomobile nowatchdog nosigar timetzdata rasterizesvg"
entry="./clientlibrary/service"

echo "==> Computing dependency graph for mobile entry (linux host view)..."
deps=$(go list -deps -tags "$tags" "$entry" 2>&1 || true)

banned_transports=(
  "github.com/quic-go/quic-go"
  "github.com/anyproto/any-sync/net/transport/quic"
  "github.com/anyproto/any-sync/net/transport/yamux"
  "github.com/anyproto/any-sync/net/transport/webtransport"
)

echo "==> Checking banned transport/discovery packages..."
found_banned=""
for pkg in "${banned_transports[@]}"; do
  if echo "$deps" | grep -qxF "$pkg"; then
    echo "BANNED in deps: $pkg"
    found_banned="yes"
  fi
done

if [ -n "$found_banned" ]; then
  echo "ERROR: banned transport packages found in mobile dependency graph"
  exit 1
fi

for pkg in "github.com/libp2p/zeroconf" "github.com/hashicorp/mdns"; do
  if echo "$deps" | grep -qxF "$pkg"; then
    echo "NOTE: $pkg present in linux dependency view (android-excluded via build tags)"
  fi
done

echo "==> Checking for outbound HTTP/dial call sites..."
patterns='http\.Get|http\.Post|http\.NewRequest|net\.Dial\(|net\.DialTimeout|net\.DialContext|net\.DialTCP|net\.DialUDP|DialTCP|DialUDP|http\.Client'
files=$(grep -RrE "$patterns" --include="*.go" . 2>/dev/null | grep -v "_test.go" | grep -v "/mock_" | grep -v "vendor/" | grep -v "^./cmd/" || true)

pending=(
  core/block/import
  core/block/importv2
  core/payments
  core/identity
  core/ai
  core/api
  core/files/fileuploader
  space/spacecore/localdiscovery
)

bad=""
while IFS= read -r line; do
  [ -n "$line" ] || continue
  allowed=""
  for a in pkg/lib/gateway core/anytype/offlinenet "${pending[@]}"; do
    case "$line" in
      *"$a"*) allowed="yes"; break ;;
    esac
  done
  if [ -z "$allowed" ]; then
    echo "OUTBOUND: $line"
    bad="yes"
  fi
done <<< "$files"

if [ -n "$bad" ]; then
  echo "ERROR: unexpected outbound network call sites found"
  exit 1
fi

if [ -n "$files" ]; then
  echo "NOTE: outbound call sites remain in pending-removal packages (runtime-blocked): ${pending[*]}"
fi

echo "==> verify-offline: OK"
