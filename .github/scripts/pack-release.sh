#!/usr/bin/env bash
# Packs the release archives from the outputs of the build jobs, with the same layout and commands as
# the single-job build used:
#   js_<version>_<os-arch>.tar.gz (.zip for windows): grpc-server[.exe], anytype-mcp-dev[.exe],
#                                  anytype-mcp-release[.exe], protobuf/ (JS), json/
#   ios_framework_<version>.tar.gz: Lib.xcframework, protobuf/ (Swift), json/
#   android_lib_<version>.tar.gz: lib.aar, protobuf/ (Java), json/
#
# Usage: pack-release.sh <version> <artifacts dir> <output dir>
# The artifacts dir holds one directory per build artifact, as actions/download-artifact writes them:
#   protos/protos.tar          js/, swift/ and java/, each with its protobuf/ and json/
#   <any>/server-<os-arch>.tar three binaries: the server, named <os-arch>, and the two anytype-mcp
#                              builds, <os-arch>-mcp-dev and <os-arch>-mcp-release (one job may
#                              upload several tars)
#   ios/ios.tar                Lib.xcframework
#   android/lib.aar
# TAR overrides the tar binary; it must be GNU tar (gtar on macOS).
set -euo pipefail

version=$1
artifacts=$(cd "$2" && pwd)
mkdir -p "$3"
out=$(cd "$3" && pwd)
tar=${TAR:-tar}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

"$tar" -xf "$artifacts/protos/protos.tar" -C "$work"

for target in darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64; do
  dir="$work/server-$target"
  mkdir -p "$dir"
  src=$(find "$artifacts" -name "server-$target.tar" | head -1)
  if [ -z "$src" ]; then
    echo "no server-$target.tar in $artifacts" >&2
    exit 1
  fi
  "$tar" -xf "$src" -C "$dir"
  cp -R "$work/js/protobuf" "$work/js/json" "$dir/"
  ext=
  if [ "$target" = windows-amd64 ]; then
    ext=.exe
  fi
  mv "$dir/$target" "$dir/grpc-server$ext"
  mv "$dir/$target-mcp-dev" "$dir/anytype-mcp-dev$ext"
  mv "$dir/$target-mcp-release" "$dir/anytype-mcp-release$ext"
  members=("grpc-server$ext" "anytype-mcp-dev$ext" "anytype-mcp-release$ext" protobuf json)
  if [ "$target" = windows-amd64 ]; then
    (cd "$dir" && zip -qr "$out/js_${version}_$target.zip" "${members[@]}")
  else
    (cd "$dir" && "$tar" -czf "$out/js_${version}_$target.tar.gz" "${members[@]}")
  fi
done

dir="$work/ios"
mkdir -p "$dir"
"$tar" -xf "$artifacts/ios/ios.tar" -C "$dir"
cp -R "$work/swift/protobuf" "$work/swift/json" "$dir/"
(cd "$dir" && "$tar" --exclude ".*" -czf "$out/ios_framework_$version.tar.gz" Lib.xcframework protobuf json)

dir="$work/android"
mkdir -p "$dir"
cp "$artifacts/android/lib.aar" "$dir/"
cp -R "$work/java/protobuf" "$work/java/json" "$dir/"
(cd "$dir" && "$tar" --exclude ".*" -czf "$out/android_lib_$version.tar.gz" lib.aar protobuf json)

ls -l "$out"
