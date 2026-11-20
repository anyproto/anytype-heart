#!/usr/bin/env bash
# Packs the release archives from the outputs of the build jobs, with the same layout and commands as
# the single-job build used:
#   js_<version>_<os-arch>.tar.gz (.zip for windows): grpc-server[.exe], protobuf/ (JS), json/
#   ios_framework_<version>.tar.gz: Lib.xcframework, protobuf/ (Swift), json/
#   android_lib_<version>.tar.gz: lib.aar, protobuf/ (Java), json/
#
# Usage: pack-release.sh <version> <artifacts dir> <output dir>
# The artifacts dir holds one directory per build artifact, as actions/download-artifact writes them:
#   protos/protos.tar          js/, swift/ and java/, each with its protobuf/ and json/
#   <any>/server-<os-arch>.tar the server binary, named <os-arch> (one job may upload several)
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
  if [ "$target" = windows-amd64 ]; then
    mv "$dir/$target" "$dir/grpc-server.exe"
    (cd "$dir" && zip -qr "$out/js_${version}_$target.zip" grpc-server.exe protobuf json)
  else
    mv "$dir/$target" "$dir/grpc-server"
    (cd "$dir" && "$tar" -czf "$out/js_${version}_$target.tar.gz" grpc-server protobuf json)
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
