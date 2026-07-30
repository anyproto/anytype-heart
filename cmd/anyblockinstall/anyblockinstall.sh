#!/usr/bin/env bash
#
# anyblockinstall — build an AnyBlock bundle into an importable archive and
# install it into a brand-new space, so a bundle can be looked at rather than
# only validated.
#
#   anyblockinstall.sh --token <t> --port <p> --bundle <dir> [--name "Space"]
#
# Does three things:
#   1. anyblockconvert  bundle -> pb snapshots + the `profile` file (SPEC §2c)
#   2. zip              archive laid out the way builtinobjects expects
#   3. grpcurl          WorkspaceCreate, then ObjectImportExperience
#
# The middleware has no gRPC reflection, so the .proto files are passed
# explicitly; they are read from the repo this script lives in.

set -euo pipefail

die() { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }
step() { printf '\033[1m==>\033[0m %s\n' "$*"; }
note() { printf '    %s\n' "$*"; }

TOKEN=""; PORT=""; BUNDLE=""; SPACE_NAME=""; KEEP=0; FORMAT="pb"; DRY=0

usage() {
  sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'
  cat <<'EOF'

Options:
  --token <token>    session token (gRPC `token` metadata header)
  --port <port>      middleware gRPC port on 127.0.0.1
  --bundle <dir>     AnyBlock bundle directory (the one holding index.json)
  --name <name>      space name; defaults to index.json's `name`
  --format pb|json   snapshot format inside the archive (default: pb)
  --keep             keep the build directory and print its path
  --dry-run          build the archive and print the two calls, without making
                     them; implies --keep, and needs neither grpcurl nor a
                     running middleware
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --token)  TOKEN="${2:-}"; shift 2 ;;
    --port)   PORT="${2:-}"; shift 2 ;;
    --bundle) BUNDLE="${2:-}"; shift 2 ;;
    --name)   SPACE_NAME="${2:-}"; shift 2 ;;
    --format) FORMAT="${2:-}"; shift 2 ;;
    --keep)   KEEP=1; shift ;;
    --dry-run) DRY=1; KEEP=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1 (try --help)" ;;
  esac
done

[[ -n "$BUNDLE" ]] || die "--bundle is required"
[[ -d "$BUNDLE" ]] || die "no such bundle directory: $BUNDLE"
if [[ "$DRY" == 0 ]]; then
  [[ -n "$TOKEN" ]] || die "--token is required"
  [[ -n "$PORT"  ]] || die "--port is required"
  command -v grpcurl >/dev/null || die "grpcurl not found (brew install grpcurl)"
fi
command -v jq  >/dev/null || die "jq not found (brew install jq)"
command -v zip >/dev/null || die "zip not found"

# the repo root: this script lives in <repo>/cmd/anyblockinstall/
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PROTO="pb/protos/service/service.proto"
[[ -f "$REPO/$PROTO" ]] || die "cannot find $PROTO under $REPO"

BUNDLE="$(cd "$BUNDLE" && pwd)"   # ObjectImportExperience needs an absolute path
BUILD="$(mktemp -d "${TMPDIR:-/tmp}/anyblockinstall.XXXXXX")"
cleanup() { [[ "$KEEP" == 1 ]] || rm -rf "$BUILD"; }
trap cleanup EXIT

# ---------------------------------------------------------------- convert ---
step "Converting $(basename "$BUNDLE")"
( cd "$REPO" && go run ./cmd/anyblockconvert -in "$BUNDLE" -out "$BUILD/archive" -format "$FORMAT" ) \
  || die "conversion failed — fix the bundle before installing it"

[[ -f "$BUILD/archive/profile" ]] || die "no profile file was produced: the bundle needs an index.json (SPEC §2c), or the space gets no name, entry point or sidebar"

# --------------------------------------------------------------------- zip ---
# Entries sit at the archive root (objects/, types/, ..., profile), matching
# util/builtinobjects/data/*.zip. -X drops the extra Finder attributes, and
# .DS_Store / __MACOSX would otherwise ride along as bogus objects.
step "Packing archive"
ARCHIVE="$BUILD/$(basename "$BUNDLE").zip"
find "$BUILD/archive" -name '.DS_Store' -delete
( cd "$BUILD/archive" && zip -r -X -q "$ARCHIVE" . -x '__MACOSX/*' )
note "$ARCHIVE ($(du -h "$ARCHIVE" | cut -f1 | tr -d ' '), $(unzip -Z1 "$ARCHIVE" | wc -l | tr -d ' ') entries)"

# space name: --name wins, else index.json's, else the directory name
if [[ -z "$SPACE_NAME" ]]; then
  SPACE_NAME="$(jq -r '.name // empty' "$BUNDLE/index.json" 2>/dev/null || true)"
  [[ -n "$SPACE_NAME" ]] || SPACE_NAME="$(basename "$BUNDLE")"
fi

rpc() { # rpc <Method> <json>
  grpcurl -plaintext -import-path "$REPO" -proto "$PROTO" \
    -H "token: $TOKEN" -d "$2" \
    "127.0.0.1:$PORT" "anytype.ClientCommands/$1"
}

# every response carries error.code; NULL is success
check() { # check <json> <what>
  local code
  code="$(jq -r '.error.code // "NULL"' <<<"$1")"
  if [[ "$code" != "NULL" && "$code" != "null" && "$code" != "0" ]]; then
    printf '%s\n' "$1" >&2
    die "$2 failed: $code — $(jq -r '.error.description // ""' <<<"$1")"
  fi
}

CREATE_REQ="$(jq -nc --arg n "$SPACE_NAME" '{details:{name:$n}, useCase:"NONE"}')"
IMPORT_PREVIEW="$(jq -nc --arg s '<spaceId from step 1>' --arg u "$ARCHIVE" --arg t "$SPACE_NAME" \
  '{spaceId:$s, url:$u, title:$t, isNewSpace:true, isAi:false}')"
if [[ "$DRY" == 1 ]]; then
  step "Dry run — would call"
  note "1. anytype.ClientCommands/WorkspaceCreate"
  note "   $CREATE_REQ"
  note "2. anytype.ClientCommands/ObjectImportExperience"
  note "   $IMPORT_PREVIEW"
  note ""
  note "archive kept at $ARCHIVE"
  exit 0
fi

step "Creating space \"$SPACE_NAME\""
CREATE_RES="$(rpc WorkspaceCreate "$CREATE_REQ")" || die "WorkspaceCreate: is the middleware listening on 127.0.0.1:$PORT?"
check "$CREATE_RES" "WorkspaceCreate"

SPACE_ID="$(jq -r '.spaceId // empty' <<<"$CREATE_RES")"
[[ -n "$SPACE_ID" ]] || { printf '%s\n' "$CREATE_RES" >&2; die "WorkspaceCreate returned no spaceId"; }
note "spaceId $SPACE_ID"

# ------------------------------------------------------------------ import ---
step "Importing"
IMPORT_REQ="$(jq -nc --arg s "$SPACE_ID" --arg u "$ARCHIVE" --arg t "$SPACE_NAME" \
  '{spaceId:$s, url:$u, title:$t, isNewSpace:true, isAi:false}')"
IMPORT_RES="$(rpc ObjectImportExperience "$IMPORT_REQ")" || die "ObjectImportExperience call failed"
check "$IMPORT_RES" "ObjectImportExperience"

step "Installed"
note "space   $SPACE_NAME"
note "spaceId $SPACE_ID"
if ENTRY="$(jq -r '.entrypoint // (.widgets[0].target) // empty' "$BUNDLE/index.json" 2>/dev/null)" && [[ -n "$ENTRY" ]]; then
  note "should open on: $ENTRY"
fi
if [[ "$KEEP" == 1 ]]; then note "build kept at $BUILD"; fi
exit 0
