# Owner-held invites (`shareWithinSpace`)

GO-7222, branch `go-7222-invite-cleanup`.

## Problem

Today `SetInviteFileInfo` writes the invite's cid **and** its file key into the workspace object
(`core/block/editor/workspaces.go:190`). The workspace object is synced to every member of the
space, so every member holds the material needed to reconstruct the invite link and hand it out.
The space owner has no way to create an invite that only they can share.

## Solution

`Rpc.Space.InviteGenerate` gets an explicit `shareWithinSpace` flag.

| | `shareWithinSpace = true` | `shareWithinSpace = false` (default) |
|---|---|---|
| workspace object (synced to members) | `spaceInviteFileCid`, `spaceInviteFileKey`, `spaceInviteType`, `spaceInvitePermissions` — today's behaviour | only the marker `spaceInviteHeldByOwner: true` |
| owner's spaceView (techspace, synced only across the owner's own devices) | — | `spaceInviteFileCid`, `spaceInviteFileKey`, `spaceInviteType`, `spaceInvitePermissions` |

The marker is what members' clients read to show "the invite is held by the space owner — ask them
for the link", instead of silently showing no invite (which would invite them to generate a new
one and revoke the owner's).

The default is the private one: a client that does not set the flag gets an owner-held invite.

The existing `spaceInvite*` relation keys are reused on the spaceView. No new keys are needed there
— their descriptions in `relations.json` already say "It stored in SpaceView", which is where they
were originally meant to live.

## Changes

### Proto (`pb/protos/commands.proto`, regenerate with `make protos-go`)

- `Rpc.Space.InviteGenerate.Request`: `bool shareWithinSpace = 4;`
- `Rpc.Space.InviteGetCurrent.Response`: `bool heldByOwner = 6;`
- `Rpc.Space.InviteGenerate.Response.Error.Code`: `INVITE_ALREADY_SHARED = 106;`,
  `INVITE_NOT_SHAREABLE = 107;`
- `Rpc.Space.InviteChange.Response.Error.Code`: `INVITE_NOT_SHAREABLE = 107;`

### Relation (`pkg/lib/bundle/relations.json`, regenerate with `make relations`)

- `spaceInviteHeldByOwner` — checkbox, hidden, readonly, `source: details`. Lives on the workspace
  object.

### `domain` (`core/domain/invite.go`)

- `InviteInfo` gains `HeldByOwner bool`.
- `InviteObject` (the interface `Workspaces` satisfies) is unchanged in shape; `SpaceView` gains the
  same three invite methods so the invite service can address either object through one interface.

### Editor (`core/block/editor/`)

The four invite accessors in `workspaces.go:190-220` are extracted into helpers over
`*state.State` / `*domain.Details`, and `SpaceView` (`spaceview.go`) implements the same
`SetInviteFileInfo` / `GetExistingInviteInfo` / `RemoveExistingInviteInfo`. `techspace.SpaceView`
exposes them.

`Workspaces.SetInviteFileInfo` branches on `info.HeldByOwner`:

- `HeldByOwner == false`: write cid/key/type/permissions, remove the marker.
- `HeldByOwner == true`: write only the marker, remove cid/key/type/permissions.

`Workspaces.RemoveExistingInviteInfo` removes the marker alongside the other four details.

### `inviteservice` (`core/inviteservice/inviteservice.go`)

- `GenerateInviteParams` gains `ShareWithinSpace bool`.
- `Generate` stores the invite file, then writes the info to the target object and clears the other
  one, so the two can never disagree:
  - shared: workspace gets the full info (which clears the marker); spaceView is cleared.
  - owner-held: spaceView gets the full info; workspace gets the marker only.
- `GetCurrent` resolves in order:
  1. spaceView has a cid → return it with `HeldByOwner = true` (this is the owner, on any of their
     devices).
  2. workspace has a cid → return it with `HeldByOwner = false` (a shared invite; also every invite
     created before this change).
  3. workspace has only the marker → return an empty cid/key with `HeldByOwner = true` and **no
     error**. The client reads `heldByOwner` as the cue to ask the owner for the link.
  4. otherwise → `ErrInviteNotExists`.
- `ShareWithinSpace` publishes the invite the owner holds into the workspace: it clears the space
  view and writes the same cid, key, type and permissions to the workspace. The invite itself is
  untouched.
- `RemoveExisting` clears both objects and deletes the invite file when a cid was found.
- `Change` (permissions) applies to whichever object actually holds the invite; on a device that
  holds only the marker it returns `ErrInviteNotExists`.

### Callers that must stop assuming a non-empty cid

- `aclService.GenerateInvite` (`core/acl/aclservice.go:769`) short-circuits on "an invite of the
  requested type already exists". It must additionally require a non-empty cid — otherwise a member
  holding only the marker would get it handed back as a live invite — and then decide on the sharing
  mode as described under *Switching an existing invite*.
- `core/publish/service.go:230` formats `InviteLinkUrlTemplate` from the cid and key. On a member's
  device with an owner-held invite both are empty; it must skip the invite link instead of
  publishing `.../<empty>/<empty>`.

### RPC (`core/space.go`)

- `SpaceInviteGenerate` passes `req.ShareWithinSpace` through.
- `SpaceInviteGetCurrent` sets `heldByOwner` from `InviteInfo.HeldByOwner`.

## What may be shared within the space

`domain.ShareableWithinSpace(inviteType, permissions)` is the single rule, and both `GenerateInvite`
and `ChangeInvite` go through it:

| invite type | permissions | shareable within the space |
|---|---|---|
| request to join (`Member`) | any | yes — a join still needs the owner's approval |
| anyone can join (`WithoutApprove`) | reader | yes |
| anyone can join (`WithoutApprove`) | writer and above | **no** — `ErrInviteNotShareable` |

Nobody approves a join made through an anyoneCanJoin link: whoever holds it is in the space, with the
permissions the invite carries. A reader's link is as much as a member is trusted to give away.

The rule is enforced in three places, because there are three ways to arrive at a shared invite that
grants too much:

- `aclService.GenerateInvite` — refuses to generate one.
- `inviteService.ShareWithinSpace` — refuses to publish one. The invite being published is the one
  the owner *holds*, whose permissions need not be the ones the request carried.
- `aclService.ChangeInvite` — refuses to raise a shared invite above read access, which would
  otherwise turn a shared reader's link into a writer's link in every member's hands.

## Switching an existing invite

When `InviteGenerate` is called for a space that already has an invite **of the requested type**,
`aclService.GenerateInvite` decides on the sharing mode before it touches the acl:

- **Same mode** — the existing invite is returned. Unchanged behaviour.
- **Owner-held → shared** (`shareWithinSpace: true`) — the invite is *published*: it moves out of
  the owner's space view and into the workspace through `inviteService.ShareWithinSpace`. It is the
  same invite. No acl record is replaced, no new file is stored, and the link the owner already
  handed out keeps working.
- **Shared → owner-held** (`shareWithinSpace: false`) — refused with `ErrInviteAlreadyShared`
  (`INVITE_ALREADY_SHARED = 106`). The workspace's *change history* has already handed this invite's
  cid and key to every member of the space; removing the details from the workspace's current state
  would not take the invite back. The client has to revoke the invite and generate a new one, which
  is a decision for the user to make — the members lose their link either way, and this way they
  lose an invite that no longer works rather than one that still does.

An invite of a *different* type still goes through `aclClient.ReplaceInvite` as before, and the new
invite is stored wherever the request asks for it. Its cid was never in the workspace's history, so
an owner-held invite generated this way is genuinely private.

## Compatibility

- Invites created before this change keep working: they sit in the workspace with no marker, so
  `GetCurrent` resolves them at step 2 with `HeldByOwner = false`. No migration.
- An old client that never sets `shareWithinSpace` gets owner-held invites. It still receives the
  cid and key in the `InviteGenerate` response, so its own share flow works.
- An old client of a **member** calling `InviteGetCurrent` for an owner-held invite gets success
  with an empty cid instead of `NO_ACTIVE_INVITE`. Accepted: clients are updated in step with this.

## Resolved during review

- **The background cleanup sweep does not cover owner-held invite files — by design.**
  `invitecleanup` discovers candidate files by walking the **workspace's** change history, where an
  owner-held invite's cid never appears. That sweep is a one-off backfill for **legacy** invites that
  predate the proper delete-invite flow. Every invite created under this feature — owner-held or
  shared — is removed through the proper path (`onInviteRevoked` → coordinator delete-invite), not
  the sweep. So an owner-held invite being absent from the sweep is correct, not a gap. No change.
- **Publishing no longer embeds a no-approval writer link into a public page.** A published page is
  public, so `applyInviteLink` (`core/publish/service.go`) now embeds the invite link only when
  `domain.ShareableWithinSpace(inviteType, permissions)` holds — i.e. a request-to-join invite (any
  permissions; a join still needs approval) or an anyoneCanJoin **reader** invite. An anyoneCanJoin
  invite that grants write access is skipped: nobody would approve the joins, so a public page must
  not carry it. This is the same rule that gates sharing within the space, applied to the broadest
  distribution there is.

## Not handled

- **Stale spaceView invite after a revoke from an unsynced device.** If the owner revokes from a
  device whose spaceView has not yet caught up, the acl records the revocation but that device
  cannot clear the spaceView entry or delete the file, and the invite lingers in the owner's
  spaceView while being dead in the acl. Only the owner can revoke or create invites (admins lose
  that ability in a separate PR), and their spaceView syncs across their own devices, so this is a
  narrow window. `invitecleanup` does not scan spaceView history — owner-held invites are deleted
  eagerly on replace/revoke by the device that holds them.

## Tests

- `core/inviteservice`: generate owner-held → spaceView holds cid/key, workspace holds only the
  marker; generate shared → the reverse; `GetCurrent` on each of the four resolution branches;
  switching modes clears the old object; `RemoveExisting` clears both.
- `core/block/editor`: `Workspaces` marker/full-info branching, `SpaceView` invite accessors.
- `core/acl`: `GenerateInvite` publishes an owner-held invite instead of replacing it, refuses to
  take a shared one back, refuses to share an anyoneCanJoin invite above read access, and does not
  treat a marker-only workspace as a live invite. `ChangeInvite` refuses to raise a shared invite
  above read access.
