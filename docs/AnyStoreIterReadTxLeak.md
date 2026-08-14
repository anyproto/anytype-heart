# any-store: `Iter` leaks its read transaction when `conn.Query` fails

Issue-ready note for github.com/anyproto/any-store. Affected: v0.4.7 and
current `main` (checked at v0.4.7-0.20260623155310-114f4ee59557 — the
code is unchanged). Verified on go1.26.5 darwin/arm64.

## The bug

`query.go` `(*collQuery).Iter`:

```go
tx, err := q.c.db.getReadTx(ctx)
if err != nil {
	qb.Close()
	return
}
stmt, err := tx.conn().Query(ctx, sqlRes)
if err != nil {
	qb.Close()
	return // ← tx is never committed or rolled back
}
```

When `conn.Query` fails, the acquired read tx is abandoned: `readTx.Commit`
is the only thing that calls `ConnManager.ReleaseRead`, so the connection
stays `isActive` forever. Every later `GetRead` on that store then spins in
the manager's retry loop (`internal/driver/manager.go` `GetRead`: wait for
`readCh` / 1-second retry) until its own ctx dies. With
`ReadConnections: 1` a single leak makes the store **permanently
unreadable**; with N connections, N leaks do.

`doReadTx` in `db.go`, immediately adjacent, handles the same shape
correctly — it commits on both the error and the success path. `Iter` (and
only `Iter`) is the outlier.

## How `conn.Query` fails in practice

`internal/driver/conn.go` `Query` arms `SetInterrupt(ctx.Done())` and then
prepares eagerly. go-sqlite's `prepare` starts with an `interrupted()`
check, so a ctx cancellation that lands **after `getReadTx` succeeded but
before the prepare** makes `Query` return `sqlite: prepare: interrupted` —
the leak branch. Any cancellable read races this window; anytype-heart hit
it in production shape (a suspend cancelling a run context mid-read wedged
the import run store — diagnosed by stack capture during the importv2
fix round, see `core/block/importv2/runstore/runstore.go` `opCtx`).

One repro caveat, for honesty: a *stale-handle* failure (`Drop` the
collection, then `Find(nil).Iter` on the old handle) does **not** hit the
branch on this driver — the missing-table error surfaces at *step* time,
an iterator is returned, and a well-behaved `iter.Close()` releases the
tx. The reachable trigger is the cancellation window above (plus anything
else that fails `Query` eagerly, e.g. a closed connection).

## Minimal repro (plain API, racing cancel)

```go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	anystore "github.com/anyproto/any-store"
)

func main() {
	ctx := context.Background()
	dir, _ := os.MkdirTemp("", "anystore-race-*")
	defer os.RemoveAll(dir)

	db, _ := anystore.Open(ctx, filepath.Join(dir, "test.db"), &anystore.Config{ReadConnections: 1})
	defer db.Close()
	coll, _ := db.Collection(ctx, "docs")

	for attempt := 1; attempt <= 200000; attempt++ {
		runCtx, cancel := context.WithCancel(ctx)
		go cancel() // races Iter's getReadTx-then-prepare window
		iter, err := coll.Find(nil).Iter(runCtx)
		if err == nil {
			_ = iter.Close()
			continue
		}
		// "prepare: interrupted" proves getReadTx had already acquired
		// the tx (a BEGIN failure reads "step: interrupted" instead).
		if strings.Contains(err.Error(), "prepare: interrupted") {
			fmt.Printf("attempt %d: %v\n", attempt, err)
			readCtx, rcancel := context.WithTimeout(ctx, 3*time.Second)
			defer rcancel()
			_, err = coll.Count(readCtx)
			fmt.Printf("victim read err = %v\n", err) // deadline exceeded: wedged
			return
		}
	}
}
```

Observed output (attempt count varies; single digits are typical):

```
attempt 5: sqlite: prepare: interrupted
victim read err = context deadline exceeded
```

A deterministic variant — a ctx whose `Done()` reports open for the first
probes (the BEGIN) and closed from the SELECT-prepare probe on — wedges on
every run; the racing version above is the natural-API demonstration.

## The fix (one line)

Release the tx in the second error branch, as `doReadTx` does:

```go
stmt, err := tx.conn().Query(ctx, sqlRes)
if err != nil {
	_ = tx.Commit() // releases the read connection
	qb.Close()
	return
}
```

(`doReadTx` also maps the error through `replaceInterruptErr` — worth
mirroring for consistency, but the `Commit` is the bug fix.)

## Blast radius in anytype-heart

Every any-store handle whose reads run on cancellable contexts is exposed;
the pool size only sets how many leaks a wedge takes.

- `core/block/importv2/runstore` — `ReadConnections: 1` (both `Open`
  sites). One leak wedges the run store. Already mitigated in-repo: every
  op runs on a detached, bounded `opCtx`, so cancellation can no longer
  land mid-op — but any future read on a live ctx re-exposes it.
- `pkg/lib/datastore/anystoreprovider` — `ReadConnections` unset →
  library default `runtime.NumCPU()`. Serves the common db, the per-space
  index dbs and the crdt dbs. Heavy `Iter` traffic on cancellable
  contexts: `pkg/lib/localstore/objectstore/spaceindex` (queries.go,
  links.go, sync.go, indexer.go), `objectstore` (indexer_store.go,
  virtual_space_store.go), `core/files/filesync/filequeue`,
  `util/persistentqueue`, `util/keyvaluestore`,
  `core/block/chats/chatrepository`,
  `core/block/editor/personalfavorites`, `core/indexer/reindex.go`,
  `space/spacecore/storage/migrator/verifier.go`. A long desktop session
  accumulates leaks silently until NumCPU of them wedge the object store;
  only a restart recovers.
- `space/spacecore/storage/anystorage` — `ReadConnections: 4`. The
  storage service itself does not call `Iter`, but it hands the db to
  any-sync, so exposure depends on any-sync's query paths.

Until the upstream fix lands, the runstore `opCtx` pattern (detach the
ctx per op, bound it with a timeout) is the working mitigation for any
store that cannot tolerate a wedge.
