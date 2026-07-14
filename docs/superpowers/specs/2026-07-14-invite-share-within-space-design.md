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
- `RemoveExisting` clears both objects and deletes the invite file when a cid was found.
- `Change` (permissions) applies to whichever object actually holds the invite; on a device that
  holds only the marker it returns `ErrInviteNotExists`.

### Callers that must stop assuming a non-empty cid

- `aclService.GenerateInvite` (`core/acl/aclservice.go:769`) short-circuits on "an invite of the
  requested type already exists". It must additionally require a non-empty cid **and** a matching
  sharing mode — otherwise a member holding only the marker would get it handed back as a live
  invite, and switching a space between shared and owner-held would be a no-op.
- `core/publish/service.go:230` formats `InviteLinkUrlTemplate` from the cid and key. On a member's
  device with an owner-held invite both are empty; it must skip the invite link instead of
  publishing `.../<empty>/<empty>`.

### RPC (`core/space.go`)

- `SpaceInviteGenerate` passes `req.ShareWithinSpace` through.
- `SpaceInviteGetCurrent` sets `heldByOwner` from `InviteInfo.HeldByOwner`.

## Switching an existing invite

Generate always goes through `aclClient.ReplaceInvite`, which revokes the previous invite, and then
writes to exactly one object while clearing the other. Switching a space from shared to owner-held
therefore removes the cid and key from the workspace for good, and members lose the link they had
— which is the point.

## Compatibility

- Invites created before this change keep working: they sit in the workspace with no marker, so
  `GetCurrent` resolves them at step 2 with `HeldByOwner = false`. No migration.
- An old client that never sets `shareWithinSpace` gets owner-held invites. It still receives the
  cid and key in the `InviteGenerate` response, so its own share flow works.
- An old client of a **member** calling `InviteGetCurrent` for an owner-held invite gets success
  with an empty cid instead of `NO_ACTIVE_INVITE`. Accepted: clients are updated in step with this.

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
- `core/acl`: `GenerateInvite` does not short-circuit across a sharing-mode change, and does not
  treat a marker-only workspace as a live invite.
