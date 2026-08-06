# Secret-scanning rules for Anytype JSON-API keys

Anytype JSON-API keys mint in a scannable format (design spec §1b,
`docs/superpowers/specs/2026-08-06-api-key-scoping-design.md`):

```
anytype_<body>_<crc32 as 8 lowercase hex>
```

- body alphabet is `[0-9A-Za-z]` only (currently lowercase unpadded base32,
  52 chars) — no `+`, `/` or `=`, so `\b`-anchored rules hold mid-token;
- the checksum is CRC32-IEEE over `anytype_<body>`, so a truncated or
  mangled candidate is cheap to reject offline;
- the published pattern is a length **range**, never a fixed length:

```
\banytype_[0-9A-Za-z]{40,60}_[0-9a-f]{8}\b
```

Coverage boundary (design spec §1b): only JSON-API keys mint in this
format. Legacy JSON-API keys retire by attrition, but `Limited`/gRPC
credentials (the web clipper's) KEEP minting in the unprefixed base64
format indefinitely — they are invisible to these rules by design, and "no
prefix" is an implicit type signal. Scanner coverage therefore never
reaches 100% of Anytype credentials; the spec's recommendation is to give
those keys their own prefix later rather than leaving them bare forever.

## The rules

| File | Tool | Usage |
| --- | --- | --- |
| `gitleaks.toml` | gitleaks ≥ 8 | `gitleaks dir --config docs/secret-scanning/gitleaks.toml <path>` |
| `trufflehog.yaml` | TruffleHog v3 custom detector | `trufflehog filesystem --config docs/secret-scanning/trufflehog.yaml <path>` |

Both match the FULL three-part shape and never the bare `anytype_` prefix:
that literal occurs in ordinary identifiers and prose in this very repo
(`anytype_mcp`, `anytype_profile_`, `anytype_backup`, …), and a prefix-only
rule would demand allowlist maintenance forever. One full-shape EXAMPLE key
does appear in this repo, in three files: the swagger `example:` tag in
`core/api/model/auth.go` and the two generated OpenAPI documents that embed
it (`core/api/docs/v1/openapi.{json,yaml}` — and the `v2` pair once it
regenerates). `gitleaks.toml` allowlists the annotation source and the
`docs/v[12]` documents, and `core/wallet/applink_scanner_rules_test.go`
walks the tracked tree asserting every full-shape match falls under that
allowlist — so a regeneration that moves the example breaks CI here, not
the first adopter's scan. Tests that need well-formed keys build them from
parts at runtime instead of embedding literals (the same test pins that the
rules match a freshly minted key and reject the repo's `anytype_…`
identifiers).

Two scan-noise caveats:

- TruffleHog custom detectors have no per-detector allowlist, so a scan of
  this repo reports the example-key sites above; exclude them with
  `--exclude-paths` (a file of path regexes) or triage them once.
- `gitleaks.toml` sets `[extend] useDefault = true` so the rule ADDS to the
  stock ruleset rather than replacing it; on this repo the stock
  `generic-api-key` rule flags a handful of synthetic fixtures in test
  files. That is the default ruleset's noise, not this rule's — drop
  `useDefault` if only the Anytype rule is wanted.

## Why detection-only

**GitHub's secret-scanning partner program is unavailable to a local-first
app.** The program requires a publicly reachable webhook endpoint that
receives leaked-candidate reports and a server-side API that can verify and
revoke the credential. Anytype has neither by design: keys are minted by
`anytype-heart` on the user's own device, the only party able to verify or
revoke one is that device's wallet, and there is no central service that
could answer for it. So these rules ship here, for anyone to run themselves,
instead of through the partner channel.

**TruffleHog cannot live-verify a localhost credential.** Its verifiers
work by calling the issuing service with the candidate secret; this
credential's issuer listens on the key owner's `127.0.0.1`, unreachable
from wherever a scan runs. That is exactly why the format carries the CRC32
tail: the checksum is the *offline* plausibility filter — a scanner (or a
human) can reject a mangled candidate without any network call. It is a
typo detector, not an authenticator: CRC32 over public bytes is trivially
forgeable, and "checksum valid" must never be read as "key valid".
