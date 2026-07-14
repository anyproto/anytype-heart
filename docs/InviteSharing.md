# Invite sharing — client integration guide

GO-7222. Middleware branch `go-7222-invite-cleanup`.

## What changed

Until now, generating an invite wrote its cid **and its file key** into the space's **workspace
object**, which every member of the space syncs. Anyone with the invite file's cid and key can fetch
the invite payload and join the space, so in effect every member could hand the space out to anyone.

Now the invite is, by default, **held by the owner**: its cid and key are written to the owner's
**spaceView** in their tech space, which only the owner's own devices sync. The workspace object —
the one members read — gets nothing but a boolean marker. Members learn that an invite exists and
that the owner is the one to ask for it, and never hold the link.

An owner who wants the old behaviour asks for it explicitly, with the new `shareWithinSpace` flag.

Nothing about the invite link, the join flow, or guest invites changes.

## RPC surface

### `Rpc.Space.InviteGenerate`

| field | |
|---|---|
| `spaceId` | |
| `inviteType` | `Member` (a join must be approved by the owner) or `WithoutApprove` (anyone with the link is in) |
| `permissions` | the permissions a joiner gets |
| **`shareWithinSpace`** *(new, field 4)* | `false` (default) — the invite is held by the owner. `true` — the invite is stored in the space, and every member can read and share it. |

**A client that does not set the flag gets an owner-held invite.** That is the safe default and it is
deliberate: an old client keeps working (it still gets the cid and key back in the response and can
show the link), it just no longer hands the link to every member.

New error codes:

| code | when |
|---|---|
| `INVITE_ALREADY_SHARED = 106` | `shareWithinSpace: false` was asked for, but the space's current invite of that type is already shared within the space. It cannot be taken back — see [Un-sharing](#un-sharing-is-not-possible). |
| `INVITE_NOT_SHAREABLE = 107` | `shareWithinSpace: true` was asked for on an anyoneCanJoin invite that grants more than read access — see [What may be shared](#what-may-be-shared). |

### `Rpc.Space.InviteGetCurrent`

| field | |
|---|---|
| `inviteCid`, `inviteFileKey` | the link — **empty on a member's device when the invite is held by the owner** |
| `inviteType`, `permissions` | **also empty in that case**: the workspace carries only the marker |
| **`heldByOwner`** *(new, field 6)* | `true` when the invite is kept in the owner's account |

This is the one call every device makes, and it answers differently depending on who is asking:

| who | invite | response |
|---|---|---|
| owner (any of their devices) | owner-held | `NULL`, cid + key + type + permissions, `heldByOwner: true` |
| owner | shared within the space | `NULL`, cid + key + type + permissions, `heldByOwner: false` |
| member | owner-held | **`NULL`**, empty cid + key, `heldByOwner: true` |
| member | shared within the space | `NULL`, cid + key + type + permissions, `heldByOwner: false` |
| anyone | no invite | `NO_ACTIVE_INVITE` |

Note the third row: a member gets a **success** response with an empty cid. Check `heldByOwner`
before you check the cid, and never render an empty link.

### `Rpc.Space.InviteChange`

Unchanged, except that it now returns `INVITE_NOT_SHAREABLE = 107` when the current invite is shared
within the space and the requested permissions are above read access.

## What may be shared

Nobody approves a join made through an **anyoneCanJoin** link: whoever holds it is in the space, with
the permissions the invite carries. Read access is as much as a member is trusted to give away.

| invite type | permissions | can be shared within the space |
|---|---|---|
| `Member` (request to join) | any | **yes** — a join still needs the owner's approval |
| `WithoutApprove` (anyone can join) | Reader | **yes** |
| `WithoutApprove` (anyone can join) | Writer and above | **no** → `INVITE_NOT_SHAREABLE` |

The rule is enforced on generating an invite, on publishing one, and on raising the permissions of
one that is already shared. There is no sequence of calls that ends with a writer-granting
no-approval link in every member's hands.

## UX flow

### Creating an invite

The invite dialog gets one control: a toggle, **"Everyone in the space can share this invite"**,
**off** by default.

- **Toggle off** → `shareWithinSpace: false`. The invite is the owner's to hand out. Members see
  "held by the owner".
- **Toggle on** → show a confirmation first: *"All space members will be able to see this invite
  link and share it with anyone."* On confirm, call with `shareWithinSpace: true`.
- **Toggle disabled** when the selected type is *anyone can join* **and** the selected permission is
  above Reader. Explain in one line why: *"Anyone with this link joins as an editor, without
  approval. Only you can share it."* Selecting Reader re-enables the toggle.

If the request is somehow made anyway, the middleware refuses it with `INVITE_NOT_SHAREABLE`.

### Sharing an invite that already exists

An owner who kept an invite to themselves can publish it later. Call `InviteGenerate` again with the
**same `inviteType`** and `shareWithinSpace: true`.

This **publishes the very same invite**: same cid, same key, same link. No acl record is replaced, no
new invite file is stored, and the link the owner has already handed out keeps working. The members
simply start seeing it.

Two things to get right:

1. **The permissions in this request are ignored.** What gets published is the invite the owner
   holds, exactly as it stands. If that invite is anyoneCanJoin with Writer permissions, the call is
   refused with `INVITE_NOT_SHAREABLE` — even if you passed `Reader` in the request. Lower the
   permissions first (see below), then publish.
2. **Do not revoke first.** Revoking and generating a new invite would invalidate the link the owner
   has already sent to people.

### Un-sharing is not possible

Asking for `shareWithinSpace: false` while the space's current invite of that type is shared returns
`INVITE_ALREADY_SHARED`. This is not a validation quirk to work around: the workspace's **change
history** has already given that invite's cid and key to every member of the space, and removing the
details from the workspace today does not take them out of the history every member has synced.

So do not offer a toggle-off on an invite that is already shared. Offer **"Revoke and create a new
invite"** instead, and say what it costs: everyone who has the old link loses it, including the
people the owner meant to invite.

### Members

A member's `InviteGetCurrent` comes back with `heldByOwner: true` and an empty cid. Show something
like *"The invite link is held by the space owner. Ask them to share it."* Do not show a copy button,
do not show a QR code, and do not offer to generate an invite — members cannot generate one (invite
rights are the owner's; admins lose them in a separate change).

A member of a space whose invite **is** shared gets the full cid and key, exactly as today, and can
copy and send the link.

### Changing the permissions of an existing invite

`InviteChange` applies to anyoneCanJoin invites only (as before). What it may do now depends on where
the invite is held:

| current invite | requested permissions | result |
|---|---|---|
| held by the owner | any | changed |
| shared within the space | Reader | changed |
| shared within the space | Writer and above | `INVITE_NOT_SHAREABLE` |

In the UI: on a **shared** anyoneCanJoin invite, only Reader may be offered. If the owner wants a
shared invite that grants write access, they cannot have one — that is the point of the restriction.

**Lowering an owner-held invite so it can be published**: `InviteChange(Reader)` first, then
`InviteGenerate(same type, shareWithinSpace: true)`. The link does not change.

## Publishing a space page

When a page is published with `joinSpace: true`, the middleware embeds the space's current invite
link into the public page — but only when the invite is safe to make public:

| current invite | embedded in the public page |
|---|---|
| request to join (`Member`) | yes — a join still needs the owner's approval |
| anyone can join, Reader | yes |
| anyone can join, Writer or above | **no** — silently skipped |
| held by the owner, on a member's device | no (there is no link there) |

A no-approval writer link is never written into a public page. Clients need not special-case this;
it is enforced middleware-side. If you show the user whether their published page carries an invite,
mirror this rule so the UI does not promise a link that will not be there.

## Existing spaces

Every invite created before this change lives in the workspace, and therefore resolves as
**shared** (`heldByOwner: false`). Nothing migrates, and nothing breaks: those invites keep working
and members keep seeing them.

Two consequences worth knowing:

- An owner cannot make an existing invite private, for the reason above. They have to revoke it and
  create a new one.
- A legacy anyoneCanJoin invite that grants Writer permissions stays as it is, but it can no longer
  be *raised* to Writer once lowered, and a shared one can no longer be raised above Reader at all.

## Checklist

- [ ] Handle `heldByOwner` on `InviteGetCurrent` **before** reading `inviteCid`. Never render an
      empty link.
- [ ] Members: show "held by the space owner", no copy/QR/generate.
- [ ] Invite dialog: the share toggle, off by default, with the confirmation popup when turned on.
- [ ] The toggle is disabled for anyoneCanJoin above Reader, with the one-line reason.
- [ ] "Share this invite with the space" action on an existing owner-held invite → `InviteGenerate`
      with the same type and `shareWithinSpace: true`. The link stays the same.
- [ ] No un-share affordance. `INVITE_ALREADY_SHARED` → offer revoke + create new, and say what is
      lost.
- [ ] `INVITE_NOT_SHAREABLE` → the invite grants too much to be shared; lower it to Reader first.
- [ ] Permission editing on a shared anyoneCanJoin invite offers Reader only.
