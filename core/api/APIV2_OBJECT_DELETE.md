# API v2 object DELETE (plan 3.3) — creator provenance and the delete surface

Status: specification, not implemented. 2026-08-14, branch `go-7383-apiv2-phase0`.

Covers plan item 3.3 (`DELETE /v2/spaces/{spaceId}/objects/{objectId}`, specced
with Phase 1, never registered — APIV2.md §3, APIV2_PLAN.md Wave 3) together
with the requirement that makes it safe to ship:

> Deletion is permitted only for objects the calling API key created. The
> identity of the creating key must be recorded immutably — the way
> `createdDate` is — not in a detail, because a detail can be overwritten by
> anyone with write access, which would let a caller forge provenance and
> delete objects it did not create.

Everything below marked **traced** was established by reading the code paths
end to end on this branch (`7957f49ed`) and in `any-sync@v0.12.16` (the go.mod
pin); nothing here required executing probes — the claims are about data-flow
and wire formats, all statically checkable, and each carries its file:line.
The one deliberate scope rule, per direction received while this was written:
**lay the foundation shared with integration attribution, ship only what
DELETE needs, and do not build the integrations feature now.** §11 is the
minimal build; §12 is the proof that attribution can be added on top without
changing the on-wire format.

The self-test this spec applies to itself: *would adding integration
attribution later require changing the on-wire format?* Answer: **no** — the
field DELETE ships (§5) is byte-for-byte the field the attribution spec
already defined, and §12 lists what attribution adds around it (all additive).
If the recommendation of §4/§5 is rejected in favor of a hash or a root-change
field, that answer flips to yes, which is the main reason not to.

---

## 1. Prior art, and how this spec relates to each

Three documents border this work. This spec **consumes the first, supersedes
one paragraph of the second, and is disjoint from the third**:

1. **`docs/IntegrationAttribution.md`** (sibling worktree
   `../anytype-heart_integration`, branch `integration-attribution`; spec
   approved in discussion, unimplemented). It already defines the exact
   primitive this feature needs: an optional `integrationKey` field — a
   normalized slug of the paired app's name — stamped by heart into the
   change payloads it owns (`pb.Change`, `pb.ChangeNoSnapshot`,
   `pb.StoreChange`), under a self-asserted, member-signed trust model.
   **This is the shared foundation. This spec does not invent a parallel
   mechanism; it ships the change-level field and its stamping plumbing (the
   subset DELETE needs) and adds the first *enforcement* consumer.** The
   attribution features proper — the derived per-space integration object,
   `createdVia`/`lastModifiedVia` relations, the ChatMessage field, icons, UI
   — are explicitly **not built now** (§11/§12).

2. **`docs/superpowers/specs/2026-08-06-api-key-scoping-design.md`** §3
   "Deletes — recorded direction": delete-own-only via a **device-local
   `appHash` detail in objectstore**. That paragraph is superseded by this
   spec; §4 option C records the comparison and why the synced, signed record
   wins. Everything else in the scoping design stands, and most of its P0/P1
   machinery is already on this branch and is load-bearing here: the resolved
   session (`ApiSessionEntry` with `AppName`/`KeyId`/`Scope`/`Grant`,
   `core/api/server/middleware.go:131-139`), the ctx carriers
   (`middleware.go:186-195`), the `/v2` grant gate and route registry
   (`core/api/v2/authz.go`), and the conformance walk. Its open question —
   whether schema deletes (`DELETE /types`, `/properties`) follow the same
   own-output rule — **stays open** (§17).

3. **`docs/DerivedDeleteConsistency.md`** plans changes to *derived-object
   uninstall store semantics*. No interaction: DELETE /objects archives (§9)
   — it never calls `deleteDerivedObject`, `BeforeDelete`, or
   `spaceindex.DeleteObject`, and it refuses type/property ids with a steer
   to their dedicated routes. If `?permanent=true` ever ships (reserved,
   §9.7), it enters the real-deletion pipeline where that document's
   findings (the §12 favorite-guard bug, GO-7433 tombstones) apply.

## 2. How `createdDate` is actually immutable — the trace

The requirement says "exactly the way `createdDate` does", so first establish
what that way is. `createdDate` is **not** a synced detail. It is a local
projection of the any-sync **root change** (the tree header), re-derived at
open:

```go
// core/block/source/sourceimpl/source.go:371
func (s *treeSource) GetCreationInfo() (creatorObjectId string, createdDate int64, err error) {
	header := s.ObjectTree.UnmarshalledHeader()
	createdDate = header.Timestamp
	if header.Identity != nil {
		creatorObjectId = domain.NewParticipantId(s.spaceID, header.Identity.Account())
	}
	return
}
```

`injectCreationInfo` (`core/block/editor/smartblock/detailsinject.go:104-151`)
copies these into the `creator`/`createdDate` details at state build. Both
relations are `source: derived` in `pkg/lib/bundle/relations.json` — they live
in **local details**, never in the synced detail set, so no member can push an
overwrite of them through sync. The detail is a cache; the authority is the
header.

The header itself is immutable for three separately-enforced reasons, all in
`any-sync@v0.12.16`:

**(a) The object id is a hash of the root change.**
`changeBuilder.BuildRoot`
(`commonspace/object/tree/objecttree/changebuilder.go:154-193`) marshals

```go
change := &treechangeproto.RootChange{
	AclHeadId: …, Timestamp: payload.Timestamp, Identity: identity,
	ChangeType: …, ChangePayload: payload.ChangePayload, SpaceId: …, Seed: …,
}
```

signs it (`payload.PrivKey.Sign(marshalledChange)`), wraps payload+signature
in `RawTreeChange`, and the **object id is
`cidutil.NewCidFromBytes(marshalledRawChange)`**. Altering any byte of the
root change — timestamp, identity, payload — produces a different id, i.e. a
different object. `cidutil.VerifyCid` is re-checked on every unmarshal
(`changebuilder.go:95`).

**(b) Every change is signature-verified against its declared identity.**
`changebuilder.go:119`: `ch.Identity.Verify(raw.Payload, raw.Signature)` →
`ErrIncorrectSignature`. Nobody can emit a change claiming an identity whose
key they do not hold.

**(c) Every peer checks the identity's ACL write permission.**
`objecttreevalidator.go` `validateChange`:

```go
perms, err = state.PermissionsAtRecord(c.AclHeadId, c.Identity)
…
if !perms.CanWrite() { err = list.ErrInsufficientPermissions; return }
```

run by `ValidateFullTree`/`ValidateNewChanges` on every device and node that
ingests the tree.

Two more facts the design below leans on (both traced):

- **Non-root changes share the immutability class.** `changeBuilder.Build`
  signs and CID-addresses every change the same way
  (`changebuilder.go:227-284`); the tree is an append-only DAG — a change can
  be *followed*, never rewritten or removed from replicas. So "the first
  content change of the tree" is exactly as tamper-proof as the root, with
  one difference in *visibility*: the root change is stored and synced
  **plaintext** (nodes must read `SpaceId`/`ChangeType`/`Identity` to route
  and validate; `BuildRoot` involves no read key), while non-root change
  payloads are **encrypted with the space read key**
  (`changebuilder.go:250`, `payload.ReadKey.Encrypt(payload.Content)`) —
  readable by space members only, opaque to infrastructure.
- **Derived trees have no signed root.** `BuildDerivedRoot`
  (`changebuilder.go:196`) emits a root with no identity and no signature
  (that is what makes ids convergent across members), and `validateChange`
  skips `IsDerived` changes. Types, relations and relation options created
  in heart are derived trees (`objectcreator/smartblock.go:143-149`);
  **regular objects are created trees** (`smartblock.go:160` →
  `CreateTreeObject` → `createPayload` with the account `SignKey` and a
  random seed, `core/block/object/objectcache/payload.go:50-64`). For a
  regular object the root header therefore always carries the creating
  *account's* identity — this is the `creator` detail's source, and half of
  the DELETE check comes for free from it.

**The precise immutability statement**: a root-change field is unforgeable by
*everyone* (the id commits to it); a non-root change field is unforgeable by
*everyone except the signing identity itself* (b+c reject any other author),
and un-rewritable even by the author after the fact (append-only + CID). "The
signing identity itself" here means the user's own account key on the user's
own device — which is the party doing the enforcing, so self-forgery is
outside the threat model by construction (a local process that could forge
the stamp could equally call the archive RPC directly; see §6).

## 3. The shared primitive (what both features actually need)

From the attribution spec's own data model, the genuinely shared piece is
exactly one thing: **an identifier of the writing agent, carried on the
change payload, stamped server-side by heart from the authenticated session —
never accepted from the request body.** Both features need from it:

- stamped only by heart, from the session (unforgeable via the API surface);
- signed into the member's change (unforgeable by other members);
- stable across key re-issue for the same integration (rotation, §8);
- resolvable to the calling credential for comparison.

What only DELETE needs: the read-back at delete time (§10), the 403 surface
(§9), and the route. What only attribution needs: per-change stamping on
*every* labeled write (DELETE needs only the creating change), the derived
integration object + icon, `createdVia`/`lastModifiedVia` relations, the
ChatMessage/StoreChange fields, history surfacing, UI. Nothing in the second
list touches the wire representation of the first (§12).

## 4. Where the record lives — the carriers compared

| | (A) root `ChangePayload` (extend `model.ObjectChangePayload`) | (B) first content change (`pb.Change.integrationKey`) — **recommended** | (C) device-local `appHash` detail (scoping design §3) | (D) synced detail | (E) per-key ACL identities |
|---|---|---|---|---|---|
| Immutable | yes — id commits to it (§2a) | yes — signed, CID-addressed, append-only (§2) | no CRDT record at all; objectstore row, wiped/rebuilt with the store | **no — any member with write access can overwrite; this is the forgery Roman's requirement excludes** | yes, cryptographically per key |
| Forgeable by other members | no | no (signature+ACL, §2b/c) | n/a (local) | yes | no |
| Survives store rebuild / reindex | yes | yes | **no — provenance lost, objects become undeletable by their own creator** | yes | yes |
| Survives account recovery on a new device | yes | yes | **no** | yes | yes |
| Visible to sync nodes (infrastructure) | **yes — root changes are plaintext (§2)** | no — encrypted with the space read key | no | no | partially (ACL is plaintext) |
| Visible to space members | yes | yes | no | yes | yes |
| Read cost at delete time | zero (`UnmarshalledHeader()` is in hand) | one history-tree read from local storage (§10) | one store lookup | one store lookup | zero |
| Covers derived trees (types/properties) if ever wanted | **no — derived roots are unsigned and their bytes determine the convergent id; adding a per-creator field would fork ids or be meaningless** | yes — their creating change is signed like any other | yes | yes | yes |
| Compatible with the attribution spec's field | no — different slot; attribution would still add the change field → two mechanisms | **identical — it IS the attribution field** | no — second mechanism | n/a | no — attribution explicitly non-goals ACL sub-keys (any-sync + coordinator work, new key-distribution UX) |
| Old clients | unknown field inside `ObjectChangePayload`, ignored; new-object ids simply computed over more bytes | unknown proto field, ignored; changes are never re-marshalled by other clients (append-only) | n/a | n/a | protocol change, migration |

**Recommendation: (B).** It is the only carrier that is simultaneously
immutable, member-signed, store-rebuild-proof, invisible to infrastructure,
and *the same field the attribution spec already defined* — the reuse the
steer asks for costs literally nothing because the two features wanted the
same bytes. (A) deserves one more sentence, because the requirement names the
root change explicitly: the root header **already carries the account
identity and timestamp** — `creator` and `createdDate` derive from it, and
the DELETE check consumes that root identity as its first clause (§10). What
the root cannot carry cheaply is *which key*: the payload bytes are plaintext
to nodes (a per-integration identifier would leak to infrastructure — the
one place the attribution trust model does not disclose it), and the slot
cannot cover derived trees at all. So the account half of provenance stays in
the root change, exactly as today; the key half lives one change deeper, in
the same immutability class, behind the space's encryption. That satisfies
the requirement's substance — immutable, unforgeable-by-members, not a detail
— while deliberately not putting the new field in the literal root; this
paragraph is the flag, per the brief, that the literal reading was considered
and set aside for cause.

(C) is not merely weaker, it fails the stated requirement twice: provenance
dies with the local store (fail-closed, but permanently — re-pairing cannot
repair it), and it can never serve attribution (nothing syncs), so shipping
it would guarantee a second mechanism later. (D) is the premise of the
requirement. (E) is the only *stronger* option — per-key cryptographic
identity — and is out of scope for the same reasons the attribution spec
non-goaled it; nothing in (B) blocks layering it later (a sub-key identity
would land in `Change.Identity` itself, orthogonal to the payload field).

## 5. One representation for both: the slug, not a hash

The steer's crux: DELETE wants *unforgeable and comparable*; attribution
wants *resolvable and human-meaningful*. These do not conflict here, because
**the unforgeability of the record does not come from the value — it comes
from the signature and the ACL check on the change that carries it** (§2).
A hash of the key would add nothing DELETE can use: the comparison is
performed by the local heart, which holds the calling key's cleartext
metadata either way; and a bare hash of a low-entropy secret is a guessable
secret, so the hash would have to be salted-per-account to be safe to sync —
at which point it is just an opaque per-integration identifier that is
*still* stable across every object (same privacy surface as the slug, §7)
while defeating attribution's display/join needs and breaking rotation
continuity (§8). The two requirements are reconciled by one field:

**The value is the attribution spec's `integrationKey`: a normalized slug of
the app link's `AppName`** — lowercase, `[a-z0-9_-]`, other runs collapsed to
`-`, trimmed, capped at 64 — derived by one shared function (proposed:
`core/domain/integrationkey.go`, `IntegrationKeyFromAppName(string) string`),
the same function the attribution feature will later use to derive the
integration object's unique key (`integration-<key>`). Properties, decided
deliberately:

- **Granularity is the integration, not the key instance.** Two keys whose
  AppName normalizes to the same slug are the same integration. The
  attribution spec calls this merge desired ("single Linear object"); for
  DELETE it is what makes rotation and re-pairing survivable (§8). The
  sharper alternative — stamping `appHash` for exact-key granularity — is
  rejected: it is a persistent pseudonymous identifier synced to every
  member with no display value, it makes every re-issue orphan the key's
  entire output, and attribution would need the slug anyway (two fields, two
  mechanisms).
- **The value is stamped by heart from the session, never read from the
  request.** There is no API surface through which a caller chooses its own
  slug; the only way to write a given slug is to hold a key whose AppName
  produces it (§6 covers who can mint such a key).
- **Empty AppName ⇒ no stamp.** A key with no recorded name (possible for
  old app links) creates unprovenanced objects and can never DELETE (§8).
  The refusal message names re-pairing with a name as the repair. The mint
  paths already require `AppName` on the challenge flow; `CreateApp` should
  reject empty names when this ships (one-line guard, listed in §11).

## 6. The unforgeability statement — and its honest limits

The enforcement predicate (§10) is a conjunction, and each clause has a
distinct guarantor:

1. **"This object was created by this account"** — root `Identity` (created
   trees), guaranteed by CID + signature + ACL validation on every peer
   (§2a-c). Another space member cannot produce an object whose root claims
   this account, and cannot alter an existing root. **Unforgeable,
   full stop.**
2. **"…via the integration named X"** — `integrationKey` on the first
   account-signed content change. Another member can stamp `"linear"` in
   *their own* changes (it is just a string), but their changes carry *their*
   identity and fail clause 1. Within this account's own objects, only this
   account's devices can have written the stamp. **Unforgeable by anyone
   outside the account's trust domain.**

What remains inside the trust domain, stated plainly rather than hidden:

- **A local process with `Full`/gRPC access** can archive anything directly,
  mint keys, or stamp arbitrary slugs. It always could; the attribution
  spec's trust-model paragraph applies verbatim: this record is provenance
  within the member's trust domain, and enforcement of what a *key* may do
  is exactly what the scoping track + this rule provide against *scoped*
  callers.
- **A user can be talked into pairing a malicious app named "Linear"** (the
  challenge flow shows the name; approval is the user's). That app then
  shares delete rights over linear-created objects. Same-user consent, same
  device, recorded as accepted.
- **Legacy unscoped keys can archive anything via `/v1`** (`DeleteObject` →
  `ObjectSetIsArchived`, `core/api/service/object.go:218-234`, grandfathered
  by the migration stance). For legacy keys the v2 rule is therefore a
  property of the v2 surface, not a global guarantee. For **scoped** keys it
  is a real boundary: they are refused on `/v1` wholesale
  (`v1_not_available_for_scoped_keys`), and on `/v2` the only archive
  channels are this route (gated) and `setProperties` — where `isArchived`
  is **output-only** and refused
  (`core/api/v2/service/stateops.go:811-814`, pinned by
  `TestIsOutputOnlyProperty`, `core/api/v2/model/model_test.go:113`). Traced:
  no other v2 route reaches `ObjectSetIsArchived` for regular objects.
- **Enforcement is local.** No peer rejects a *delete* — peers reject forged
  *changes*. The property shipped is: the record cannot be falsified by
  anyone who could profit from falsifying it, and the enforcing heart reads
  the record from cryptographically validated storage, never from a detail.

## 7. Privacy — decided

The slug syncs inside encrypted change payloads to **every space member**,
forever (version history). Space members can, with a modified client, read
which integration created each of this account's objects before any
attribution UI exists. Decision: **intended.** This is precisely the
disclosure the attribution feature exists to make ("attribution must be
visible to all space members, survive sync, persist in history" — its stated
goal), under the trust model already approved there; shipping the field
before the UI changes *when* members can see it, not *whether*. What this
spec deliberately does **not** disclose: nothing is visible to sync nodes
(the field rides encrypted payloads — the root-change option was rejected
partly for leaking to infrastructure, §4), and nothing names the *key*
(no hash, no id — a leaked slug says "a thing called linear", not which
credential). Objects created via API additionally already carry the synced
`origin: api` detail today (`core/api/objectcreateadapter.go:43`), so "this
object came from an API" is not new member-visible information — only
"which integration" is.

## 8. Rotation, re-issue, legacy objects

- **Re-issue, same name** (revoke key, pair again as "Linear"): same slug →
  old objects remain deletable by the new key. This is the rotation story,
  and it is the decisive argument for the slug over any per-key value.
  Works across devices and across account recovery, since the record is in
  the synced tree.
- **Re-issue, different name**: new slug → old objects are no longer
  deletable by the new key, permanently (the record is immutable by design —
  there is deliberately no re-point). The refusal message names the recorded
  slug so the repair ("re-pair under the old name") is discoverable.
  Accepted; documenting "keep the app name stable" costs one sentence in the
  key-management docs.
- **Legacy objects** (everything created before this ships — the vast
  majority of every account, including today's v2-created eval fixtures) and
  **objects created by other members, by the apps themselves, by import, or
  by any unstamped path**: no record → **DELETE refused, fail-closed**, for
  every key including legacy unscoped ones (uniform rule on v2; Roman's
  requirement is unqualified, and v1 remains the grandfathered escape hatch
  for legacy keys). There is no backfill and must not be one: a backfill
  would be a detail-grade assertion of exactly the kind the requirement
  bans. Consequence stated honestly: the plan's "only way to clean up test
  data" motivation (3.3) is served **going forward only**; the two existing
  eval documents still need one manual archive.
- **No separate capability is specced** for "delete anything" (e.g. a grant
  bit). Recorded as an open product decision (§17) rather than designed
  here — the requirement's whole point is that shipping without it is safe.

## 9. The DELETE surface

**9.1 Semantics: archive.** `DELETE /v2/spaces/{spaceId}/objects/{objectId}`
archives (Bin, reversible in the app) via `ObjectSetIsArchived` — v1 parity
(`core/api/service/object.go:224`) and v2 uniformity (`DeleteType`/
`DeleteProperty`, `core/api/v2/service/schema_write.go:580,792`). Real
deletion stays out (`?permanent=true` remains reserved, §9.7). Archive being
reversible does not soften the requirement: mass-archival by an agent hides
data from every view and every other agent, and reversal is manual per
object.

**9.2 Route + middleware**, exactly the established v2 DELETE stack
(`core/api/v2/router.go:324-329` pattern):

```go
v2.DELETE("/spaces/:space_id/objects/:object_id",
	deps.WriteRateLimit,
	idempotencyMW,                       // C8 — uniform across every v2 DELETE
	deps.AnalyticsEvent("V2DeleteObject"),
	v2handler.DeleteObjectV2Handler(deps.Service),
)
```

plus the registry entry `routeKey(http.MethodDelete,
"/v2/spaces/:space_id/objects/:object_id"): {Verb: RouteVerbWrite}` in
`core/api/v2/authz.go` — the conformance walk fails the build without it, so
the route cannot ship unclassified.

**9.3 Authorization is a conjunction, not an alternative.** For scoped keys
the existing gates run first and unchanged: space ∈ `grant.spaces`
(`space_not_granted`) and `perms == readwrite` (`write_not_granted`). The
creator check is **in addition to** the write grant — a readwrite grant
means "create and edit broadly, destroy only your own output", which is the
recorded direction of the scoping design and Airtable's loudest lesson. For
unscoped keys the creator check is the only gate beyond auth.

**9.4 Service flow** (`V2Service.DeleteObject(ctx, spaceId, objectId,
dryRun)`):

1. `ensureSpace` (existing backstop; grant-aware).
2. Resolve the object row; absent or `isDeleted` → 404 `not_found`.
3. Layout steer: type/property/tag-option targets → 400 `validation_failed`
   naming `DELETE /v2/spaces/{id}/types/{key}` (resp. `/properties`) — those
   routes exist and carry their own semantics; participants, space views and
   other restriction-protected objects → let `ObjectSetIsArchived`'s
   restriction error map to 403 `forbidden` with the restriction named.
4. **Provenance check** (§10). Mismatch → 403 `not_created_by_this_key`.
5. Already archived → 200 with `warnings: [{message: "already archived"}]`
   (idempotent no-op, consistent with C8 retry semantics).
6. `ObjectSetIsArchived{IsArchived: true}`; RPC failure → 500
   `internal_error`.
7. Receipt: the uniform v2 delete shape (`v2model.CreateResult` as
   `DeleteType` returns it): `{id, type, dry_run?, warnings?}`, HTTP 200.

**9.5 The refusal** — C6, and the message must name the actual repair (the
four review rounds' standing rule). New code
`CodeNotCreatedByThisKey = "not_created_by_this_key"`, HTTP 403. Three
variants, each naming what IS recorded:

- created by no key (apps/import/legacy): `DELETE is limited to objects this
  API key created, and no API key is recorded as this object's creator
  (created by the Anytype app or before provenance existed). To remove it,
  archive it in the Anytype app.`
- created via a different integration: `…this object was created via
  'linear', not via this key ('claude-desktop'). Use the linear key, or
  archive it in the app.`
- created by another space member: `…this object was created by another
  space member. Ask them, or archive it in the app if your role permits.`

Each carries `issues: [{path: "objectId", hint: "probe deletability without
writing via DELETE …?dry_run=true; the created_date/creator properties on
GET show who created the object"}]`, and the standard `WWW-Authenticate:
Bearer error="insufficient_scope"` channel stays untouched (this is not a
scope failure).

**9.6 `?dry_run=true` (C9) is the deletability probe.** A dry run executes
steps 1–5 — including the provenance verdict — and skips only the archive:
allowed → 200 `{id, dry_run: true}`; refused → the same 403. This answers
"which of my objects can I clean up?" without a queryable index, which the
minimal build deliberately lacks: the query-side sugar (`createdVia` as a
filterable detail) is attribution's, is advisory-only by construction (a
detail — §2), and is not needed to ship (agents track their own created ids
from create receipts; C8 makes those receipts reliable under retry).

**9.7 Reserved, unchanged:** `?permanent=true` (hard delete through the bin
pipeline) — when it comes, it consumes the same provenance under the same
rule and inherits the DerivedDeleteConsistency findings; not designed here.

## 10. The enforcement read — algorithm and cost

At delete time, read provenance from validated storage, **never from
details**:

1. Build the tree from local storage the way version history does:
   `spc.TreeBuilder().BuildHistoryTree(ctx, objectId,
   objecttreebuilder.HistoryTreeOpts{Heads: current, Include: true})`
   (`core/history/history.go:549` is the shipped pattern; the in-memory live
   tree may be snapshot-reduced and MUST not be used — the creating change
   can predate its base snapshot).
2. Root clause: `root := tree.UnmarshalledHeader()`; require
   `root.Identity != nil && root.Identity.Account() == ownAccount` — the
   §2-grade guarantee that this account created the object. (Derived trees
   fail here by construction; they were steered away in §9.4-3.)
3. Key clause: iterate from the root in tree order; take the **first
   non-root change**; require its `Identity` to equal the root identity, and
   unmarshal its payload (`sourceimpl.UnmarshalChange`, decrypted by the
   history tree exactly as `BuildState` does) as `pb.Change`; require
   `change.IntegrationKey != "" && change.IntegrationKey ==
   IntegrationKeyFromAppName(ctxKeyInfo.Name)`. The calling key's name is
   already in the request context on this branch
   (`util.CtxWithApiKeyInfo`, `core/api/server/middleware.go:188-194`).
4. Any clause failing → the §9.5 refusal, with the recorded slug (or its
   absence) folded into the message.

Edge, recorded: between tree creation and the first content push, another
device of the *same account* could theoretically append the first non-root
change (the tree syncs from the root). Then step 3's identity still matches
but the slug is absent → fail-closed refusal of a genuinely-owned object.
The window is the milliseconds between `PutTree` and the init `Apply`
(`objectcache/tree.go:56-67`), on an object the other device cannot yet
know exists; accepted as vanishingly rare and safe-direction.

Cost: one full-history read of one tree from local SQLite — the same class
as opening that object's version history, a shipped user-facing operation;
on a DELETE endpoint (human-scale frequency, write-rate-limited) this is
acceptable without optimization. If it ever matters, the storage layer's
order index allows reading just root + first change; noted, not specced.

**Retention dependency, flagged:** this read requires the creating change to
exist in storage. Today heart syncs and retains full tree history on every
device (version history is built on it — traced through
`BuildHistoryTree`/`buildState`; there is no pruning path). If history
truncation ever ships, creating-change provenance must be explicitly carried
forward (the way `OriginalCreatedTimestamp` rides snapshots —
`source.go:451` — is the precedent, though a snapshot-carried copy is only
detail-grade: §2 — a truncation feature would need to preserve the original
signed change, which any credible truncation of a CRDT with version history
likely must anyway). This is the design's one long-term structural
assumption.

## 11. The minimal build for 3.3 (steer question 3)

Everything DELETE needs, nothing attribution-specific. Each item is
independently reviewable; together they are one shippable PR train:

1. **Proto** (`pb/protos/changes.proto` + regen): `string integrationKey =
   10;` on `Change` **and** `ChangeNoSnapshot` — the two messages share wire
   numbers by design (content 3, fileKeys 6, timestamp 7, version 8,
   changeType 9; 1/2/5 are historical — do not reuse), and the read-side
   conversion at `sourceimpl/source.go:114-121` must copy the new field.
   `StoreChange` (chat) gets nothing now; its field is additive whenever
   attribution lands.
2. **Slug derivation**: `core/domain` (proposed `integrationkey.go`),
   `IntegrationKeyFromAppName` with the §5 normalization + property tests.
   One function, later shared by attribution's unique-key derivation.
3. **Neutral ctx carrier**: a `domain`-level (not `core/api/util`-level)
   `CtxWithIntegrationKey`/`CtxIntegrationKey`, installed by the API auth
   middleware next to the existing carriers (`middleware.go:186-195`),
   computed once per session from `AppName`. Attribution's future gRPC
   interceptor installs the same carrier — that is the shared plumbing
   seam, and it is one function each side.
4. **Stamping**: `source.PushChangeParams` gains `IntegrationKey string`
   (`core/block/source/interface.go:96`); `treeSource.buildChange`
   (`sourceimpl/source.go:434`) copies it onto the `pb.Change`. Fill site
   for the minimal slice: the **creation path only** — `objectcreator`
   has the request ctx in hand end-to-end
   (`CreateSmartBlockFromState` → `CreateTreeObject(ctx…)` →
   `InitContext.Ctx`, `core/block/editor/smartblock/smartblock.go:212-225`),
   so the creating Apply can carry the value into `PushChangeParams`.
   Implementation caution, pinned by test: the value must be **per-apply**,
   never persisted on the state object — a later UI edit on this device
   must NOT inherit the stamp (assert: second Apply's change carries no
   key). Widening to every labeled change (attribution's `lastModifiedVia`
   feed) is the same param filled from the session/state ctx later — more
   fill sites, zero wire change.
5. **Provenance read port**: a small `apicore` port (implemented beside the
   existing adapters in package `api`, which already owns the heart-internal
   composition) exposing `CreatorProvenance(ctx, spaceId, objectId)
   (accountMatch bool, integrationKey string, err error)` per §10.
6. **Surface**: `V2Service.DeleteObject` (§9.4), handler, route + authz
   registry entry + `V2DeleteObject` analytics id, the
   `not_created_by_this_key` code + constructors in `v2model`, OpenAPI
   annotations, `make openapi`.
7. **Guards**: `CreateApp`/challenge reject empty `AppName` (§5); no other
   issuance change.
8. **Docs**: APIV2.md §3 build-item closure note; SKILL.md gets the
   delete verb + the dry-run probe idiom; scoping design §3 paragraph gets a
   superseded-by pointer.

Explicitly **not built** (attribution's, later): `SmartBlockTypeIntegration`
+ derived object + icon pipeline, `createdVia`/`lastModifiedVia` relations,
`StoreChange`/`ChatMessage` fields, history surfacing, any UI, gRPC session
labeling, restrictions registration.

## 12. The extension test — attribution later, wire diff: zero

Claim to check: adding integration attribution on top of §11 changes no
shipped wire format. The attribution spec's needs, item by item:

| Attribution piece | What it consumes/adds | Wire change to §11's format? |
|---|---|---|
| Per-change field on object trees | `pb.Change.integrationKey = 10` — **already shipped by §11.1, same field, same number, same value** | none |
| Stamping every labeled change (not just creation) | more fill sites for the same `PushChangeParams.IntegrationKey` (session ctx chain per its "Identity plumbing" section) | none |
| Chat | new additive fields (`StoreChange.integrationKey = 2`, `ChatMessage.createdVia = 18`) — new surfaces, not changes to shipped ones | additive only |
| Integration object | derived from `integration-<slug>` via the **same** `IntegrationKeyFromAppName` (§11.2) — ids converge with what DELETE recorded | none |
| `createdVia` detail | set at creation from the same creating-change value §10 reads; display/query sugar, never enforcement | none |
| `lastModifiedVia` | mirrors `SetLastModified` from per-change stamps | none |
| History rows | reads `integrationKey` from each `pb.Change` — the field already there | none |
| Icons, wallet, UI, restrictions | orthogonal | none |

Conversely, if §5's representation were a hash or §4's carrier the root
payload, attribution would require a second field (resolvable slug) or a
second slot (change-level) respectively — i.e. **only** the recommended
shape passes this test, which is the concrete content of "the foundation
costs nothing extra now and demonstrably prevents a format change later."
DELETE-created objects from the interim are then *retroactively* fully
attributed (their creating changes already carry the slug), which is a small
free win the narrow design would forfeit.

## 13. Cost and compatibility

- **Bytes**: `integrationKey` ≈ slug length + 2 (tag+len) — `claude-desktop`
  = 16 bytes — on the creating change only (minimal slice), and only for
  API-key-authored creations. Against a typical creating snapshot change
  (KB-scale; hard cap 10 MiB, `sourceimpl/source.go:47`) this is noise;
  sync-volume delta likewise. CPU: one string copy at build, one slug
  normalization per session (cached with the session entry).
- **Old clients**: the field is an unknown proto field inside the
  **encrypted, heart-owned** change payload — any-sync and the nodes never
  parse it; old hearts ignore it on unmarshal; and because trees are
  append-only, no old client ever re-marshals an existing change, so there
  is no lossy-rewrite path. The root-change format is untouched — nothing
  any-sync-visible changes at all. No migration; no backfill (§8).
- **New clients reading old objects**: absent field → unprovenanced →
  fail-closed refusal (§8). Deterministic, no heuristics.
- **Failure surface added to creation**: none — stamping cannot fail
  (string copy); an empty slug degrades to today's behavior.

## 14. What argues against, collected

Stated as prominently as the case for, per the brief:

1. **The guarantee is asymmetric for legacy keys** (§6): until legacy
   unscoped keys sunset, a legacy holder archives anything via v1. The rule
   is airtight only for scoped keys — which are exactly the keys agents
   should hold, but the spec must not be read as "no API caller can archive
   others' objects".
2. **Fail-closed strands real use-cases**: today's v2-created objects, and
   any "agent, clean up this space" request over human-created objects, get
   a 403 whose only repair is the app. If product wants agent-driven
   cleanup of arbitrary objects, that needs the explicit capability §17
   leaves open — this design deliberately cannot express it.
3. **The retention assumption** (§10): provenance lives in full tree
   history; a future history-truncation feature inherits a hard constraint
   from this spec.
4. **Slug-granularity is same-user-forgeable** (§6): pairing a same-named
   app transfers delete rights. Within the stated trust model this is
   consent, not forgery, but it is the weakest link and the reason the word
   "unforgeable" is always qualified with "by other members" in this spec.
5. **Member-visible forever** (§7): the disclosure is approved by the
   attribution trust model, but it ships *before* the UI that explains it.
   Anyone auditing raw changes sees integration slugs from day one.

None of these, in this spec's judgment, outweigh shipping: 1 shrinks by
attrition and is the scoping design's explicit stance; 2 is the requirement
working as intended; 3–5 are recorded costs of the only design that passes
§12.

## 15. Testing plan

- **Slug**: normalization property tests; same AppName across devices/keys →
  same slug (§11.2).
- **Stamping**: creation via ctx carrying a key → creating change carries
  the slug (assert at the `pb.Change` level, the E′8 lesson: change-set
  assertions, not just green Applies); creation without ctx (UI path,
  indexer, import) → no field; **second Apply on the same object without
  ctx → no field** (the §11.4 leak guard); `ChangeNoSnapshot` roundtrip
  preserves it; old-proto unmarshal ignores it.
- **Provenance read**: fixture trees — created-by-this-account+slug (allow);
  same account, no slug (legacy shape — refuse); same account, different
  slug (refuse, message names it); derived root (refuse via steer);
  snapshot-reduced live tree vs history read (the §10 must-not-use case).
- **Surface**: table-driven handler/service tests per the house fixture
  pattern — 404 / steer / 403×3 variants / already-archived idempotence /
  dry-run both verdicts / C8 replay / scoped-key conjunction
  (space_not_granted and write_not_granted still fire first); conformance
  walk picks up the registry entry (fails if omitted).
- **E2E** (rides the Q1 charter): create via scoped key → DELETE → gone from
  default queries; DELETE of a pre-existing object → 403; dry-run writes
  nothing (C9's real-not-writing assertion).

## 16. Where this ships, and issue keys

The spec lands on this branch (GO-7383) as the plan-3.3 design record. The
implementation is two separable PR trains and should carry its own issue
number(s) at implementation time: the provenance foundation (§11.1–11.4,
core change-pipeline territory, reviewable by the sync owners) and the API
surface (§11.5–11.8, `core/api` territory) — the foundation must merge
first or in the same release; the route without the record would be a 403
for everything, which is shippable but pointless. If the attribution epic
gets its GO number first, the foundation train belongs under it — it is
that spec's §3, built early.

## 17. Open decisions (recorded, not blocking)

1. Schema deletes (`DELETE /types`, `/properties`): adopt the same own-output
   rule, or stay plain readwrite? (Scoping design's open question;
   unchanged. The provenance read works for derived trees via their signed
   creating change if the answer is ever yes.)
2. A "delete any" capability for keys that should manage whole spaces
   (grant bit? bot accounts?) — product call; nothing here precludes it.
3. Whether `whoami` should surface the key's slug (cheap, helps agents
   predict deletability against a future `createdVia` read) — lean yes,
   one field, but it is attribution-adjacent disclosure and can wait.
4. Relation key names and the rest of the attribution spec's own open
   questions — untouched here, owned there.
