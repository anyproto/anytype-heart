# Exclusive cross-process account directory ownership

Date: 2026-08-31
Status: Proposed

## Decision

Anytype Heart must hold one OS-backed exclusive file lock for the entire time it can open or
mutate an account directory. The lock is acquired before the first account write, migration, DB
open, or Tantivy open, and is released only after all account components have closed and any
requested account-directory mutation has finished.

Use the already-declared `github.com/gofrs/flock` v0.12.1 dependency behind a small
`accountdirlock` package:

- macOS, iOS, Linux, and Android use the platform `flock(2)` implementation;
- Windows uses `LockFileEx`;
- acquisition is exclusive and bounded, never an indefinite wait;
- lock contention fails account startup with the existing "another process is running" error;
- lock syscall, permission, and unsupported-filesystem errors fail closed;
- the lock file is persistent and is never deleted during normal unlock or stale-lock cleanup;
- PID/owner metadata is diagnostic only and is never used to decide lock ownership or kill a
  process.

This is a **local-filesystem, cross-process lock**, not a distributed lock. Different operating
systems/devices do not share an account directory and therefore do not coordinate with each other.
If two processes can see the same local account storage, they must resolve the same lock file.

## Problem

There is no account-wide ownership boundary today.

- Current SQLite/anystore databases permit multiple processes to open the same files. SQLite
  serializes DB transactions, but it does not serialize Anytype's independent in-memory object
  caches, sync processors, queues, recovery paths, or derived indexes.
- The any-store `*.lock` files are dirty-shutdown sentinels, not held OS locks. One process can
  remove a sentinel while another process is still writing.
- Badger's legacy directory lock covers only the legacy store. The special handling in
  `pkg/lib/datastore/clientds/process.go` can kill a PID found in Badger's `LOCK` file and is not an
  account ownership protocol.
- Tantivy has a writer lock, but `ftSearch.tryToBuildSchema` currently handles every writer-open
  error by removing `fts_tantivy` and retrying. On Unix, an open locked file can be unlinked while
  its old file descriptor remains alive, so this can bypass the lock and split two processes onto
  different physical indexes.
- An account operation spans several stores and files. No individual DB lock can make the whole
  operation atomic or coherent across processes.

The required invariant is larger than "SQLite does not corrupt its pages": only one Heart process
may own an account's local state at a time.

## Goals

- Guarantee at most one cooperating Heart process owns a given local account directory.
- Work on desktop and mobile targets: macOS, Windows, Linux, iOS, and Android.
- Recover automatically after graceful exit, panic, crash, forced termination, or power loss;
  there must be no stale lock that requires PID cleanup.
- Cover normal account startup, account creation, legacy recovery/import, storage migration,
  restart, move, and local-data removal.
- Fail before opening any account DB or index when another process owns the account.
- Preserve the current client-visible `ANOTHER_ANYTYPE_PROCESS_IS_RUNNING` behavior where it
  already exists, while replacing error-string matching with typed errors.
- Provide useful best-effort diagnostics about the current/previous owner.
- Add defense in depth so Tantivy never deletes an index in response to lock contention.

## Non-goals

- A distributed lock across devices, VMs, containers with separate filesystems, or cloud-synced
  copies of an account.
- Making it safe for third-party processes that ignore the advisory lock to edit account files.
- Using this lock to coordinate different accounts under the same root. Different account IDs may
  be owned by different processes.
- Coordinating a custom external file-store directory independently of the main account repo. If
  sharing one custom file store between accounts is ever supported, it needs its own ownership
  contract.
- Replacing SQLite's transaction locks, Tantivy's internal writer lock, or any-store durability
  sentinels. Those remain lower-level mechanisms.

## Platform research and constraints

The design uses the common subset of platform semantics: one exclusive, non-blocking lock held by
an open file handle.

### Linux and Android

Linux `flock(2)` provides exclusive and non-blocking modes. Locks are associated with an open file
description and are released by explicit unlock or when all associated file descriptors close.
They are advisory on a normal local filesystem. See the
[Linux `flock(2)` manual](https://man7.org/linux/man-pages/man2/flock.2.html).

### macOS and iOS

Darwin provides BSD `flock(2)` with the same exclusive/non-blocking model for cooperating
processes. See the
[Apple `flock(2)` manual](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/flock.2.html).

### Windows

`LockFileEx` supports exclusive locks and `LOCKFILE_FAIL_IMMEDIATELY`. Windows releases outstanding
locks when the owning handle closes or process terminates, although cleanup after forced
termination may not be instantaneous. See
[Microsoft's `LockFileEx` documentation](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-lockfileex).

### Library choice

`github.com/gofrs/flock` v0.12.1 is already a direct dependency. Its
[`TryLock`](https://github.com/gofrs/flock/tree/v0.12.1) API is non-blocking; v0.12.1 uses
`unix.Flock` on the Unix targets and `LockFileEx` on Windows. Keeping this behind an internal
interface avoids spreading library-specific behavior through application code.

Only exclusive locks are used. Shared locks and lock upgrades are deliberately excluded because
their behavior varies between platforms and the account has no supported read-only process mode.

### Local filesystems only

NFS and SMB lock behavior depends on client/server versions and mount options. The Linux manual
documents substantial NFS/SMB semantic variation. Account DB directories on network or
synchronization filesystems are unsupported for this guarantee. If the lock syscall reports an
unsupported or I/O error, Heart fails closed rather than continuing without a lock.

No attempt is made to infer ownership from a PID file. PIDs are reusable, metadata can be stale,
and "check then create" is not an ownership primitive.

## Invariants

1. Before a Heart process mutates an account directory or opens any DB/index inside it, it owns
   that account's lease.
2. At most one live lease exists for a `(canonical root path, account ID)` pair.
3. A lease remains held throughout application startup, runtime, reverse component shutdown, and
   any immediately-following move/delete operation.
4. Account restart retains the existing lease; it does not unlock between close and reopen.
5. Lock contention and lock-mechanism failure are different errors, but both prevent account open.
6. File existence, file contents, PID liveness, and owner metadata never prove ownership. Only a
   successful OS lock acquisition does.
7. The stable lock file is never removed or replaced. This is load-bearing: on Unix, unlinking a
   locked file removes its pathname while the old open file remains alive, allowing a newly-created
   file at the same path to receive a second independent lock. See the
   [`unlink(2)` semantics](https://man7.org/linux/man-pages/man2/unlink.2.html).
8. Components do not acquire the account lock independently. The application service owns exactly
   one lease and passes the ownership boundary around the whole component graph.

## Lock identity and filesystem layout

Given:

```text
rootPath  = /path/to/Anytype
accountID = A5...canonical account id...
accountDir = rootPath/accountID
```

the lock lives at:

```text
rootPath/.anytype-locks/<accountID>.lock
rootPath/.anytype-locks/<accountID>.owner.json
```

The lock file intentionally lives **outside** `accountDir` but under the same root.

- `AccountStop(removeData=true)` may remove `accountDir` without unlinking the held lock.
- Failed account creation can clean and recreate `accountDir` without changing the lock inode.
- Repair code cannot accidentally remove the ownership lock with a recursive account subdirectory
  cleanup.
- The root scopes identical account IDs in genuinely distinct local repositories.
- A hidden root-level directory avoids treating lock files as account data.

The filename contains the full canonical account ID so operators can identify the owner without
cross-referencing a hash. Before using it as a path component, validate it with the Any-Sync
account-address decoder and require canonical round-trip encoding. Do not accept an arbitrary RPC
string after checking only that it is non-empty. A canonical account ID uses the existing
filesystem-safe account-address encoding and cannot contain path separators, `.`/`..`, Windows
reserved names, or platform separators. Invalid/non-canonical IDs return bad input before any lock
or account path is constructed.

Before deriving the path:

1. create `rootPath` with mode `0700` if the calling flow is allowed to create it;
2. resolve `filepath.Abs(rootPath)`;
3. resolve symlinks with `filepath.EvalSymlinks` after root creation, failing closed if resolution
   fails;
4. create `.anytype-locks` with mode `0700`;
5. open/create the lock file with `O_CREATE|O_RDWR`, mode `0600`.

`O_RDWR` gives consistent exclusive-lock behavior on the supported library backends and avoids the
known requirement for writable descriptors on some Unix/network implementations.

Path canonicalization improves diagnostics and in-process identity checks. The OS lock on the
resolved lock-file inode remains authoritative.

## Package API

Add `core/application/accountdirlock` with no dependency on the application service:

```go
package accountdirlock

var (
    ErrLocked      = errors.New("account directory is locked")
    ErrUnavailable = errors.New("account directory lock is unavailable")
)

type Owner struct {
    LeaseID           string     `json:"leaseId"`
    AccountID         string     `json:"accountId"`
    PID               int        `json:"pid"`
    Executable        string     `json:"executable,omitempty"`
    MiddlewareVersion string     `json:"middlewareVersion,omitempty"`
    Hostname          string     `json:"hostname,omitempty"`
    ProcessStartedAt  *time.Time `json:"processStartedAt,omitempty"`
    AcquiredAt        time.Time  `json:"acquiredAt"`
    ReleasedAt        *time.Time `json:"releasedAt,omitempty"`
    State             string     `json:"state"` // "holding" or "released"
}

type Lease struct {
    // unexported; owns *flock.Flock and immutable identity
}

func Acquire(ctx context.Context, rootPath, accountID, middlewareVersion string) (*Lease, error)
func (l *Lease) Release() error
func (l *Lease) AccountDir() string
func (l *Lease) LockPath() string
```

`Acquire` behavior:

1. Validate `rootPath`, decode and canonicalize `accountID`, and derive the stable paths above.
2. Construct `flock.New(lockPath, flock.SetFlag(os.O_CREATE|os.O_RDWR),
   flock.SetPermissions(0600))`.
3. Try immediately, then retry every 50 ms for at most one second, bounded by the caller context.
   This absorbs an orderly handoff and Windows' potentially delayed forced-exit cleanup without
   allowing an indefinite startup wait. Timing is injectable in tests.
4. If acquisition returns `false` through the internal deadline, read `owner.json` best-effort and
   return a typed error wrapping `ErrLocked`.
5. If the syscall/open fails, return a typed error wrapping `ErrUnavailable`. Do not start in an
   unlocked compatibility mode.
6. After acquiring, atomically write best-effort owner metadata with a random lease ID and
   `state=holding`. Metadata failure is logged but does not invalidate a successfully-held OS lock.

`Release` is idempotent. While still holding the lock, it atomically updates matching owner
metadata to `state=released` with `releasedAt`; it then calls `Unlock`, which closes the underlying
handle. The metadata file is not deleted. A new owner overwrites it only after successfully
acquiring the OS lock.

Owner metadata is deliberately separate from the locked file. Windows byte-range locks can deny
access through another handle, and diagnostics must not require rewriting or replacing the stable
lock inode.

The contender may display/log owner metadata, but it must label it unverified: a crash can leave
`state=holding`, and a race can make any read stale immediately.

## Application ownership lifecycle

Add the current lease and its identity to `application.Service`, protected by the existing service
mutex:

```go
type Service struct {
    // existing fields...
    accountLease *accountdirlock.Lease
}
```

Separate "close the component app" from "release account ownership". The current `stop()` helper
mixes several call-site intentions; after this change, call sites explicitly choose whether a
lease survives component shutdown.

```text
UNOWNED
  -> acquire lease
OWNED / STARTING
  -> StartNewApp succeeds
OWNED / RUNNING
  -> close components
OWNED / CLOSED
  -> restart/move/delete while still owned, or release
UNOWNED
```

If startup fails, `app.Start` already closes initialized/running components in reverse order. The
caller releases the lease only after `StartNewApp` returns. If the process crashes or panics before
explicit release, the OS closes the handle and releases the lock.

### Selecting or switching accounts

For `AccountSelect`:

1. Resolve the requested account identity.
2. If it is already the running account owned by this service, keep the lease and retain current
   same-account behavior.
3. Otherwise acquire the target account lease **before** stopping the currently-running account.
4. If target acquisition fails, leave the current account running and return the contention/error.
5. Once target ownership succeeds, close the old app and release its old lease.
6. Open/start the target while holding its lease.
7. On target startup failure, release the target lease after startup cleanup completes.

Acquiring the target first prevents a failed switch from unnecessarily logging the user out. The
acquisition is bounded/non-blocking, so two processes simultaneously switching A -> B and B -> A
cannot deadlock; both retain their original account if target acquisition fails.

### Operation coverage

| Operation | Acquire/retain boundary | Release boundary |
|---|---|---|
| `AccountSelect` | Before `WalletInitRepo`, migration mutation, DB/index open | After app close, or after failed-start cleanup |
| `AccountCreate` | Before `WalletInitRepo` and config/account-dir writes | After app close; on failed creation after cleanup |
| legacy export recovery | Before creating/importing account contents | After resulting app closes; on failure after cleanup |
| account-store migration | Before opening either old or new store | After migrator app closes and all rename/delete work ends |
| network/config restart | Retain the existing lease across close/start | Not released during restart |
| `AccountStop(removeData=false)` | Already held | After all components close |
| `AccountStop(removeData=true)` | Already held | After component close and `os.RemoveAll(accountDir)` completes |
| `AccountMove` | Already held | After app close and all copy/config/remove work completes |
| process shutdown | Already held | After app close; OS release is the crash fallback |

Every direct `StartNewApp` caller in `core/application` must go through one ownership-aware helper.
This includes `AccountCreate`, `AccountSelect`, and legacy-export recovery; leaving one bypass would
invalidate the guarantee.

Wallet create/recover only derives keys and creates the root directory, not the account directory,
so it does not need an account lease. Once the account ID is known and per-account files are about
to be created/opened, the lease is mandatory.

### Deletion and close failures

Local-data removal holds the sibling lock while deleting `accountDir`. The persistent lock file
must not be included in the recursive delete target.

If component close panics, process termination releases the OS lock. If close returns errors after
attempting all components, report the errors and release only after the reverse close loop has
returned. Components must continue to satisfy their existing contract that no account-file access
survives `Close`.

## Errors and client behavior

Replace the Badger error substring check with typed errors:

```go
if errors.Is(err, accountdirlock.ErrLocked) {
    return nil, errors.Join(ErrAnotherProcessIsRunning, err)
}
```

- `ErrLocked`: another cooperating process owns the account. Map it to
  `RpcAccountSelectResponseError_ANOTHER_ANYTYPE_PROCESS_IS_RUNNING` and equivalent errors for
  create/recovery/migration entry points.
- `ErrUnavailable`: Heart could not establish the safety mechanism due to permission, unsupported
  filesystem, path, or OS error. Return a distinct internal/start failure and include the cause;
  never misreport it as ordinary contention and never continue unlocked.
- Context cancellation: return the caller's cancellation unless the internal one-second acquisition
  deadline expired, in which case return `ErrLocked`.

Logs include the canonical account ID and path, lock path, acquisition duration, and best-effort
owner fields. Mnemonic/key material is never included.

## Tantivy defense in depth

The account lock is the primary ownership boundary, but Tantivy must remain safe against old Heart
versions that do not yet acquire it.

Change full-text startup as follows:

1. If the consistency report says `.tantivy-writer.lock` or `.tantivy-meta.lock` is actively held,
   return a typed `ErrIndexInUse`. Do not run GC, remove files, or retry by rebuilding.
2. `tryToBuildSchema` must not treat an arbitrary `NewTantivyContextWithSchema` error as corruption.
   Propagate lock, permission, I/O, cancellation, and unknown errors unchanged.
3. Rebuild only after a positive, classified consistency/schema-corruption result and only while
   holding the account lease.
4. Prefer quarantining a corrupt derived index to a timestamped sibling and creating a new index;
   remove the quarantine asynchronously only after the replacement is open. Never replace the
   account lock file or `.anytype-locks` directory.

This protects a new process when an older process already owns Tantivy. It cannot protect a new
process from an older binary started afterward because the old binary ignores the new account lock
and contains the destructive retry. That mixed-version limitation disappears once all shipped
binaries contain this change.

## Legacy Badger behavior

Remove the process-killing behavior from `clientds.RemoveExpiredLocks`. A process must never kill a
PID merely because it appears in a file and has the same executable basename.

During migration:

- acquire the account lease first;
- let Badger's own directory lock remain defense in depth;
- map Badger contention to the same typed ownership error where possible;
- never delete or override a live Badger lock.

## Testing

File-lock behavior must be tested with subprocesses. A same-process-only test can miss differences
in file-description and handle semantics.

### `accountdirlock` subprocess tests

Run on macOS, Windows, and Linux CI; compile the package for Android and iOS as part of mobile CI.

1. **Mutual exclusion:** process A acquires and signals readiness; process B receives `ErrLocked`.
2. **Graceful release:** A calls `Release`; B acquires the same account.
3. **Crash release:** forcibly terminate A; B acquires within a bounded retry window. Allow the
   documented Windows cleanup delay.
4. **Persistent file:** release does not delete or replace the lock file; inode/file identity stays
   stable where the platform exposes it.
5. **Different accounts:** A and B can concurrently acquire different account IDs under one root.
6. **Different roots:** identical account IDs under distinct roots do not contend.
7. **Path alias:** relative/absolute and symlink aliases to one root contend (skip symlink cases when
   Windows test permissions do not permit creating one).
8. **Account deletion:** deleting/recreating `root/accountID` while A holds the sibling lock does not
   let B acquire.
9. **Owner metadata:** contention survives missing, malformed, stale, and unwritable metadata.
10. **Account ID validation:** non-canonical IDs, separators, traversal components, and malformed
    account addresses are rejected before constructing a filesystem path.
11. **Permissions/unsupported errors:** acquisition fails closed with `ErrUnavailable`.
12. **Context cancellation and timeout:** no goroutine, timer, or descriptor leaks.
13. **Race test:** concurrent `Release` calls are idempotent under `go test -race`.

### Application integration tests

1. Start/select the same migrated account in two `grpcserver` subprocesses on different ports; the
   second returns `ANOTHER_ANYTYPE_PROCESS_IS_RUNNING` before opening objectstore, space stores, or
   Tantivy.
2. After graceful shutdown of the first server, the second selects successfully.
3. After forced termination of the first server, selection succeeds without deleting a stale file.
4. Repeat for legacy Badger and SQLite-to-anystore migration fixtures.
5. Verify a failed switch to a locked target leaves the currently-running account alive.
6. Verify network/config restart retains ownership continuously.
7. Verify local-data removal holds ownership through recursive deletion.
8. Instrument DB/Tantivy constructors in a test graph and assert none are called when lease
   acquisition fails.
9. Hold only Tantivy's writer lock (simulating an old binary) and verify startup fails without
   modifying `fts_tantivy`.

## Observability

Add structured log fields and counters:

- `account_lock_acquire_total{result=acquired|contended|error}`;
- `account_lock_acquire_ms`;
- `account_lock_release_error_total`;
- canonical root/account ID, but not secret key material;
- best-effort owner PID/version/start time on contention.

Emit a critical diagnostic report if `Unlock` fails. The process should not attempt to open that
account again through a fresh lease after an unlock failure; termination remains the safe recovery.

## Rollout

1. Add and subprocess-test `accountdirlock` without changing lifecycle behavior.
2. Integrate it into migration and every `StartNewApp` path.
3. Refactor stop/restart/delete/move call sites so lease release is explicit and correctly ordered.
4. Harden Tantivy error classification and remove destructive retry on lock contention.
5. Remove Badger PID-killing cleanup and error-string-based account ownership detection.
6. Add the two-server integration test to CI.

During rollout, new binaries are safe when they encounter an already-running old binary through
Tantivy's defense-in-depth check. An old binary launched after a new one does not honor the new
lock, so release notes and desktop process supervision should still avoid mixed-version concurrent
servers until the old binary population ages out.

## Rejected alternatives

### Rely on SQLite WAL locks

SQLite protects individual DB transactions, not the account-wide sequence across source stores,
derived stores, Tantivy, files, caches, and sync engines. Opening two processes remains valid to
SQLite and invalid to Anytype.

### Rely on Tantivy or Badger

They cover only their own directory and are acquired too late. Current Tantivy recovery also
defeats its writer lock. Migrated accounts may never open Badger.

### PID file or `O_CREATE|O_EXCL` sentinel

Both leave stale files after crash. PID reuse and non-atomic liveness checks make cleanup unsafe.
The existing Badger cleanup demonstrates the worst outcome: killing an unrelated or still-valid
process.

### Delete the lock file on unlock

Unsafe. A process can unlink a locked file while another process still holds its old inode and
then create a new file at the same pathname, producing two independent locks. The stable file must
remain.

### Put the lock inside the account directory

Account removal, recovery, and repair recursively replace that directory. The lock pathname would
be removed while held and could be recreated by a contender. A sibling lock under the stable root
survives those operations.

### Bind a well-known TCP port

Ports identify a host/process endpoint, not an account path. They collide across unrelated
accounts, interact with firewall/network policy, and do not protect non-server/mobile entry points.

### Lock the whole root directory

Overly broad. It would prevent safe concurrent ownership of distinct accounts under one root and
would make account switching and migration unnecessarily coupled.

### Distributed lock service

There is no shared account directory across devices in scope. A remote lease adds availability and
recovery failure modes without protecting local filesystem access by an offline process.
