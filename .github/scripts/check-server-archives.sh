#!/usr/bin/env bash
# Checks the js_<version>_<os-arch> archives against what was built.
#
#   check-server-archives.sh digest <os-arch>...
#       Right after the build, in the directory of the build outputs: writes server-<os-arch>.sha256
#       with the digests of <os-arch>, <os-arch>-mcp-dev and <os-arch>-mcp-release.
#
#   check-server-archives.sh verify <version> <release dir> <digests dir>
#       Opens the five final archives and asserts, for each: the member names; the execute bit; the
#       architecture; static linkage of the linux binaries; each binary's digest against its build
#       output (server-<os-arch>.sha256, found anywhere under the digests dir); and the
#       namespace each anytype-mcp carries, by the marker linked into it and, where this host can run
#       the binary, by what `version --json` reports.
set -euo pipefail

targets=(darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64)

sha() { shasum -a 256 "$1" | cut -d' ' -f1; }

fail() {
  echo "::error::$*" >&2
  exit 1
}

if [ "${1:-}" = digest ]; then
  shift
  for target in "$@"; do
    : > "server-$target.sha256"
    for bin in "$target" "$target-mcp-dev" "$target-mcp-release"; do
      [ -f "$bin" ] || fail "build output $bin is missing"
      echo "$(sha "$bin")  $bin" >> "server-$target.sha256"
    done
  done
  exit 0
fi

[ "${1:-}" = verify ] && [ $# -eq 4 ] || fail "usage: $0 digest <os-arch>... | verify <version> <release dir> <digests dir>"
version=$2
release=$(cd "$3" && pwd)
digests=$(cd "$4" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

host="$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m)"
case "$host" in
  linux-x86_64) host=linux-amd64 ;;
  linux-aarch64) host=linux-arm64 ;;
  darwin-arm64) host=darwin-arm64 ;;
  darwin-x86_64) host=darwin-amd64 ;;
esac

arch_pattern() {
  case "$1" in
    darwin-amd64) echo 'Mach-O 64-bit.*x86_64' ;;
    darwin-arm64) echo 'Mach-O 64-bit.*arm64' ;;
    linux-amd64) echo 'ELF 64-bit.*x86-64' ;;
    linux-arm64) echo 'ELF 64-bit.*aarch64' ;;
    windows-amd64) echo 'PE32\+.*x86-64' ;;
  esac
}

for target in "${targets[@]}"; do
  echo "== $target"
  dir="$work/$target"
  mkdir -p "$dir"
  ext=
  if [ "$target" = windows-amd64 ]; then
    ext=.exe
    archive="$release/js_${version}_$target.zip"
    [ -f "$archive" ] || fail "$archive is missing"
    unzip -q "$archive" -d "$dir"
  else
    archive="$release/js_${version}_$target.tar.gz"
    [ -f "$archive" ] || fail "$archive is missing"
    tar -xzf "$archive" -C "$dir"
  fi

  # member names: exactly these at the top level
  want=$(printf '%s\n' "anytype-mcp-dev$ext" "anytype-mcp-release$ext" "grpc-server$ext" json protobuf | sort)
  got=$(ls -A "$dir" | sort)
  [ "$got" = "$want" ] || fail "$target: members are [$(echo $got)], want [$(echo $want)]"

  sums=$(find "$digests" -name "server-$target.sha256" | head -1)
  [ -n "$sums" ] || fail "$target: no server-$target.sha256 under $digests"

  for pair in "grpc-server$ext:$target" "anytype-mcp-dev$ext:$target-mcp-dev" "anytype-mcp-release$ext:$target-mcp-release"; do
    member=${pair%%:*}
    built=${pair##*:}
    bin="$dir/$member"
    [ -f "$bin" ] || fail "$target: $member is not a file"
    [ -x "$bin" ] || fail "$target: $member has no execute bit"
    desc=$(file -b "$bin")
    echo "$member: $desc"
    echo "$desc" | grep -Eq "$(arch_pattern "$target")" || fail "$target: $member is not a $target binary"
    if [[ $target == linux-* ]] && ! echo "$desc" | grep -q 'statically linked'; then
      fail "$target: $member is not statically linked"
    fi
    expected=$(awk -v name="$built" '$2 == name { print $1 }' "$sums")
    [ -n "$expected" ] || fail "$target: $sums has no digest for $built"
    [ "$(sha "$bin")" = "$expected" ] || fail "$target: $member differs from the build output $built"
  done

  # the namespace is linked in: the release marker is in the release build and in no other
  for ns in dev release; do
    bin="$dir/anytype-mcp-$ns$ext"
    has_release=no
    if LC_ALL=C grep -aq 'anytype-mcp-namespace:release' "$bin"; then
      has_release=yes
    fi
    if [ "$ns" = release ] && [ "$has_release" = no ]; then
      fail "$target: anytype-mcp-release does not carry the release namespace"
    fi
    if [ "$ns" = dev ] && [ "$has_release" = yes ]; then
      fail "$target: anytype-mcp-dev carries the release namespace"
    fi
    if [ "$target" = "$host" ]; then
      reported=$("$bin" version --json)
      echo "anytype-mcp-$ns reports $reported"
      echo "$reported" | grep -q "\"namespace\":\"$ns\"" || fail "$target: anytype-mcp-$ns reports another namespace"
    fi
  done
done

echo "All five server archives hold the server and both anytype-mcp builds."
