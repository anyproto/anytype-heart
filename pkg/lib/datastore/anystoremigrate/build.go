package anystoremigrate

import (
	"context"
	"errors"
	"fmt"

	anystorev1 "github.com/anyproto/any-store"
	anystore "github.com/anyproto/any-store/v2"
	"github.com/anyproto/any-store/v2/anyenc"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
)

const (
	// A batch is one write transaction: up to batchDocs documents or batchBytes of document
	// bytes, whichever comes first. Large enough to amortize the commit, small enough that an
	// aborted batch loses little and one transaction doesn't hold much of the WAL.
	batchDocs  = 1000
	batchBytes = 8 << 20
	// walCheckpointBytes is the WAL size past which the build checkpoints (truncating) after the
	// batch or index build that took it there. The WAL then never exceeds it by more than one batch
	// or one index build, however large the collection; a small database is checkpointed once, at
	// its end, instead of paying a synced checkpoint per collection and index.
	walCheckpointBytes = 32 << 20
)

// legacyOrderIdSort walks a v1 chat messages collection by its _o.id index: in storage-key order,
// so the inserts into the v2 collection append.
const legacyOrderIdSort = chatmodel.OrderKey + ".id"

// build copies every collection of src into a new v2 database at path. It returns the number of
// legacy stubs it left out of each chat messages collection.
func (m *Migrator) build(ctx context.Context, src anystorev1.DB, path string, rep *Report) (stubs map[string]int, err error) {
	dst, err := anystore.Open(ctx, path, m.dstConfig())
	if err != nil {
		return nil, fmt.Errorf("create v2 database: %w", err)
	}
	defer func() {
		if closeErr := dst.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close v2 database: %w", closeErr)
		}
	}()

	names, err := src.GetCollectionNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	stubs = make(map[string]int)
	w := &walWatch{db: dst, path: path, peak: &rep.PeakWalBytes}
	for _, name := range names {
		n, dropped, err := m.copyCollection(ctx, src, dst, name, w)
		if err != nil {
			return nil, fmt.Errorf("collection %q: %w", name, err)
		}
		rep.Collections++
		rep.Documents += n
		if chatmodel.IsMessagesCollection(name) {
			rep.ChatCollections++
			rep.StubsDropped += dropped
			stubs[name] = dropped
			if dropped > 0 {
				log.Info("chat stubs dropped", zap.String("path", path), zap.String("collection", name), zap.Int("stubs", dropped))
			}
		}
		if err = w.checkpointIfLarge(ctx); err != nil {
			return nil, fmt.Errorf("collection %q: %w", name, err)
		}
	}
	// the database ends with an empty WAL
	if err = w.checkpoint(ctx); err != nil {
		return nil, err
	}
	return stubs, nil
}

// copyCollection copies one collection, then builds its indexes. A chat messages collection is
// keyed by the storage key, gets the chat repository's indexes and loses its legacy stubs.
func (m *Migrator) copyCollection(ctx context.Context, src anystorev1.DB, dst anystore.DB, name string, w *walWatch) (copied, stubs int, err error) {
	srcColl, err := src.OpenCollection(ctx, name)
	if err != nil {
		return 0, 0, fmt.Errorf("open v1 collection: %w", err)
	}
	defer srcColl.Close()

	isChat := chatmodel.IsMessagesCollection(name)
	var (
		dstColl anystore.Collection
		query   = srcColl.Find(nil)
		indexes []anystore.IndexInfo
	)
	if isChat {
		dstColl, err = dst.CreateCollection(ctx, name, chatmodel.MessagesCollectionOptions())
		query = query.Sort(legacyOrderIdSort)
		indexes = chatmodel.MessagesCollectionIndexes()
	} else {
		dstColl, err = dst.CreateCollection(ctx, name)
		for _, idx := range srcColl.GetIndexes() {
			info := idx.Info()
			indexes = append(indexes, anystore.IndexInfo{Name: info.Name, Fields: info.Fields, Unique: info.Unique, Sparse: info.Sparse})
		}
	}
	if err != nil {
		return 0, 0, fmt.Errorf("create v2 collection: %w", err)
	}
	defer dstColl.Close()

	iter, err := query.Iter(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("iterate v1 collection: %w", err)
	}
	defer iter.Close()

	b := &batcher{db: dst, coll: dstColl, w: w}
	var (
		parser anyenc.Parser
		arena  anyenc.Arena
		buf    []byte
	)
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			b.abort()
			return 0, 0, fmt.Errorf("read v1 document: %w", err)
		}
		// v1-encoded anyenc bytes parse unchanged in v2; ParseOwned does not copy, and the value
		// is consumed by the insert before buf is reused
		buf = doc.Value().MarshalTo(buf[:0])
		val, err := parser.ParseOwned(buf)
		if err != nil {
			b.abort()
			return 0, 0, fmt.Errorf("parse document: %w", err)
		}
		if isChat {
			orderId := val.GetString(chatmodel.OrderKey, "id")
			if orderId == "" {
				// a legacy stub an old reaction upsert made: no content, no order
				stubs++
				continue
			}
			arena.Reset()
			val.Set(chatmodel.StorageKeyField, arena.NewString(chatmodel.StorageKey(orderId, val.GetString("id"))))
		}
		if err = b.insert(ctx, val, len(buf)); err != nil {
			return 0, 0, err
		}
		copied++
		if m.opts.failAfterDocs > 0 && copied >= m.opts.failAfterDocs {
			b.abort()
			return 0, 0, errInjected
		}
	}
	if err = iter.Err(); err != nil {
		b.abort()
		return 0, 0, fmt.Errorf("iterate v1 collection: %w", err)
	}
	if err = b.commit(); err != nil {
		return 0, 0, err
	}

	// indexes after the bulk load: one build over the loaded collection is far cheaper than
	// maintaining them on every insert. Each is a transaction of its own.
	for _, info := range indexes {
		if err = dstColl.CreateIndex(ctx, info); err != nil {
			return 0, 0, fmt.Errorf("create index %q: %w", info.Name, err)
		}
		if err = w.checkpointIfLarge(ctx); err != nil {
			return 0, 0, fmt.Errorf("after index %q: %w", info.Name, err)
		}
	}
	return copied, stubs, nil
}

var errInjected = errors.New("injected failure")

// batcher inserts documents in write transactions of a bounded size and checkpoints the WAL
// when it grew past walCheckpointBytes.
type batcher struct {
	db   anystore.DB
	coll anystore.Collection
	w    *walWatch

	tx          anystore.WriteTx
	docs, bytes int
}

func (b *batcher) insert(ctx context.Context, val *anyenc.Value, size int) (err error) {
	if b.tx == nil {
		if b.tx, err = b.db.WriteTx(ctx); err != nil {
			return fmt.Errorf("begin batch: %w", err)
		}
	}
	if err = b.coll.Insert(b.tx.Context(), val); err != nil {
		b.abort()
		return fmt.Errorf("insert document %q: %w", val.GetString("id"), err)
	}
	b.docs++
	b.bytes += size
	if b.docs < batchDocs && b.bytes < batchBytes {
		return nil
	}
	if err = b.commit(); err != nil {
		return err
	}
	return b.w.checkpointIfLarge(ctx)
}

func (b *batcher) commit() error {
	if b.tx == nil {
		return nil
	}
	tx := b.tx
	b.tx, b.docs, b.bytes = nil, 0, 0
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch: %w", err)
	}
	return nil
}

func (b *batcher) abort() {
	if b.tx != nil {
		_ = b.tx.Rollback()
		b.tx = nil
	}
}

// walWatch checkpoints the v2 WAL and records the largest size it saw it at.
type walWatch struct {
	db   anystore.DB
	path string
	peak *int64
}

// observe records the current WAL size and returns it.
func (w *walWatch) observe() int64 {
	size, err := filesSize(w.path + "-wal")
	if err != nil {
		return 0
	}
	*w.peak = max(*w.peak, size)
	return size
}

// checkpointIfLarge checkpoints once the WAL grew past walCheckpointBytes.
func (w *walWatch) checkpointIfLarge(ctx context.Context) error {
	if w.observe() < walCheckpointBytes {
		return nil
	}
	return w.checkpoint(ctx)
}

// checkpoint moves the WAL into the main file and truncates it. Nothing reads the database
// during the build, so no reader pins the WAL and the truncate always completes.
func (w *walWatch) checkpoint(ctx context.Context) error {
	w.observe()
	if err := w.db.Flush(ctx, 0, anystore.FlushModeCheckpointTruncate); err != nil {
		return fmt.Errorf("checkpoint wal: %w", err)
	}
	return nil
}
