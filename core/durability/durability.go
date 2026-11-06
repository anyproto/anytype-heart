package durability

/*
AI generated

Name: Database Flush Coordinator
Scope: global

## Responsibility
- Triggers database WAL flushes on app state changes (background/closing)

## Documentation
StateChange behavior by state:
- AppClosingInitiated: Best-effort flush (3s timeout, no wait) - DB component does final flush later
- AppWentBackground: Blocking flush (10s timeout, wait for completion) - ensures data safety before suspend/hibernate

Flush order: space stores first (critical data), then objectstore (can be reindexed)
*/

import (
	"time"

	anystore "github.com/anyproto/any-store"
	anystorev2 "github.com/anyproto/any-store/v2"
	"github.com/anyproto/any-sync/app"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

const CName = "durability"

var log = logging.LoggerNotSugared(CName)

type Flusher interface {
	Flush(timeout time.Duration, waitPending bool, mode anystore.FlushMode)
}

// FlusherV2 is Flusher for the heart-owned any-store v2 databases (the provider).
type FlusherV2 interface {
	Flush(timeout time.Duration, waitPending bool, mode anystorev2.FlushMode)
}

type durability struct {
	spaceCore        Flusher   // space stores: any-store v1
	anystoreProvider FlusherV2 // objectstore, crdt: any-store v2
}

func New() app.Component {
	return new(durability)
}

func (c *durability) Name() (name string) {
	return CName
}

func (c *durability) Init(a *app.App) (err error) {
	c.spaceCore = a.MustComponent(spacecore.CName).(Flusher)
	c.anystoreProvider = a.MustComponent(anystoreprovider.CName).(FlusherV2)
	return nil
}

func (s *durability) StateChange(state int) {
	switch domain.CompState(state) {
	case domain.CompStateAppClosingInitiated:
		// waitPending=false because we need to do best effort without locking the app closing
		// db component will perform final flush on closing after all writes are done
		// flush space stores first, because others we can reindex without data loss
		s.spaceCore.Flush(time.Second*3, false, anystore.FlushModeCheckpointPassive)
		s.anystoreProvider.Flush(time.Second*3, false, anystorev2.FlushModeCheckpointPassive)
	case domain.CompStateAppWentBackground:
		// Blocking: iOS may suspend us right after this RPC returns, so committed data must be
		// power-safe before we let the call finish.
		// Fsync mode syncs the WAL only (sufficient at synchronous=normal) instead of a passive
		// checkpoint: background filesystem I/O is throttled on iOS and a checkpoint of every open
		// DB can grind for minutes without honoring the context (see GO-7393). WAL space
		// reclamation stays on the idle auto-flush and wal_autocheckpoint paths.
		start := time.Now()
		s.spaceCore.Flush(time.Second*10, true, anystore.FlushModeFsync)
		spaceCoreSpent := time.Since(start)
		start = time.Now()
		// TODO(any-store v2): no fsync-only flush mode in v2, so the provider gets a passive checkpoint
		s.anystoreProvider.Flush(time.Second*10, true, anystorev2.FlushModeCheckpointPassive)
		anystoreSpent := time.Since(start)
		if spaceCoreSpent+anystoreSpent > time.Second {
			log.With(zap.Int64("spaceCoreSpentMs", spaceCoreSpent.Milliseconds()), zap.Int64("anystoreSpentMs", anystoreSpent.Milliseconds())).Warn("flushing took too long")
		}

	}
}
