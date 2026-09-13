# AnyBlock import sweep

This command starts headless Heart with a **new local-only account**, creates a
new space for each retained export folder, and imports through `ObjectImport`
using the protobuf importer. It does not open or recover the source account.

```sh
go run ./cmd/anyblockimportsweep -output /tmp/import-sweep-001 -keep-account \
  /path/to/verified/native /path/to/repaired/native
```

Inputs can be individual bundles or roots containing per-space directories.
Later roots override earlier directories with the same source space ID, so a
repaired export replaces its earlier copy. The sweep reads `skipped-spaces.json`
beside each `native/` root and records those inputs as `skipped` without creating
a destination space. The export/eval harness writes this manifest for deleted
spaces. Empty directories without explicit skip metadata are still attempted
and reported as missing exports. `-space ID`, `-limit N`, and `-timeout 10m` allow
focused runs. By default the command builds this checkout's `cmd/grpcserver`;
`-heart-binary PATH` uses an explicitly supplied binary.

Each run requires a new output location. `report.json` is updated after every
space and records source paths, destination space IDs, completion process IDs,
errors, and log diagnostics. `heart.log` retains the complete server log.
`-keep-account` keeps the scratch account and records its path; otherwise it is
removed on shutdown. Reports and logs can contain private exported content.

The initial RPC response only acknowledges scheduling. A stream registration
barrier precedes imports; the harness waits for the matching import notification
and process completion, in either order. Completed failures do not stop the
sweep. An unknown completion (timeout or broken stream) stops it and leaves
remaining entries `not_attempted`, avoiding overlapping imports.

Failures include RPC errors, notification failures, process errors, and ERROR
logs from importer components (some older stages only log their failures).
Unrelated service logs remain diagnostics, not attributed import failures.
Exit status is nonzero if any import failed. A passing result establishes that
no import error was reported; it is not a semantic round-trip comparison.

The same sweep is available as an opt-in test:

```sh
ANYBLOCK_SWEEP_INPUTS='/path/to/verified/native:/path/to/repaired/native' \
ANYBLOCK_SWEEP_OUTPUT=/tmp/import-sweep-test-001 \
go test ./cmd/anyblockimportsweep -run '^TestSweep$' -count=1 -timeout 2h -v
```

Use the platform path-list separator in `ANYBLOCK_SWEEP_INPUTS`. Set
`ANYBLOCK_SWEEP_KEEP_ACCOUNT=1` to retain the test's scratch account.

Skip manifest format (outside the bundle payloads):

```json
{"version":1,"spaces":[{"spaceId":"SOURCE_SPACE_ID","reason":"accountStatus: Deleted"}]}
```

Skipped entries do not count against `-limit`. If all selected inputs are skipped,
Heart is not started. Later input roots use their own manifest, allowing a
repaired export to replace an earlier skipped input.
