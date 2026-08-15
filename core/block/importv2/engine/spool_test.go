package engine

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

func fileObjWithOpen(key, content string) *importv2.Object {
	return &importv2.Object{
		SourceKey: key,
		SbType:    coresb.SmartBlockTypeFileObject,
		Payload:   &importv2.Snapshot{Details: domain.NewDetails()},
		File: &importv2.FileSource{
			Name: key,
			Open: func(ctx context.Context) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader(content)), nil
			},
		},
	}
}

func durableSpool(t *testing.T) (Spool, string) {
	t.Helper()
	spillDir := t.TempDir()
	spool, err := runstore.OpenStandaloneSpool(context.Background(), spillDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = spool.Close() })
	return spool, spillDir
}

func TestSpoolPass(t *testing.T) {
	t.Run("pass 2 drains Open closures to the spill dir; pass 3 sees a plain path", func(t *testing.T) {
		// given — the unserializable-closure problem dissolves by eager
		// download (DM spec fact 3): the spooled object carries a path.
		fx := newEngineFixture(t)
		spool, spillDir := durableSpool(t)
		fx.deps.Spool = spool
		fx.deps.SpillDir = spillDir
		converter := &scriptConverter{objects: []*importv2.Object{
			fileObjWithOpen("img.png", "bytes"),
			pageObj("a.md", false),
		}}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		fx.persister.mu.Lock()
		path := fx.persister.filePaths["img.png"]
		openSeen := fx.persister.fileOpenSeen["img.png"]
		fx.persister.mu.Unlock()
		require.NotEmpty(t, path, "the file object must reach persist with a spill path")
		assert.Equal(t, spillDir, filepath.Dir(path), "the drained bytes live in the spill dir")
		assert.False(t, openSeen, "no closure may survive the spool round-trip")
	})

	t.Run("a failed download fails the file object, not the run (continue mode)", func(t *testing.T) {
		// given
		fx := newEngineFixture(t)
		spool, spillDir := durableSpool(t)
		fx.deps.Spool = spool
		fx.deps.SpillDir = spillDir
		broken := fileObjWithOpen("bad.png", "")
		broken.File.Open = func(ctx context.Context) (io.ReadCloser, error) {
			return nil, assert.AnError
		}
		converter := &scriptConverter{objects: []*importv2.Object{broken, pageObj("a.md", false)}}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then: the page still imports; the file is a loud failure
		require.NoError(t, result.Err)
		assert.Equal(t, int64(1), result.Created)
		assert.Equal(t, int64(1), result.Failed)
	})

	t.Run("no persist or journal effect occurs before Convert returns", func(t *testing.T) {
		// given — this pins the SHAPE of the clean-space property at the two
		// seams this fixture has (the persister call and the effect
		// journal); the full property — no uploads, installs, flag writes or
		// minting either — holds by construction (pass 3 owns those code
		// paths) and by the reviewed call-graph audit, which a unit fixture
		// cannot observe.
		fx := newEngineFixture(t)
		spool, spillDir := durableSpool(t)
		fx.deps.Spool = spool
		fx.deps.SpillDir = spillDir
		var converterDone atomic.Bool
		var persistBeforeConvertDone atomic.Bool
		fx.persister.observe = func() {
			if !converterDone.Load() {
				persistBeforeConvertDone.Store(true)
			}
		}
		converter := &observableConverter{
			inner: scriptConverter{objects: []*importv2.Object{
				pageObj("a.md", false), pageObj("b.md", false), pageObj("c.md", false),
			}},
			onDone: func() { converterDone.Store(true) },
		}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, int64(3), result.Created)
		assert.False(t, persistBeforeConvertDone.Load(),
			"no persist may run while the converter is still fetching")
	})
}

// observableConverter signals when Convert returns.
type observableConverter struct {
	inner  scriptConverter
	onDone func()
}

func (c *observableConverter) Name() string { return c.inner.Name() }

func (c *observableConverter) EnumerateIdentities(ctx context.Context, yield func(importv2.IdentityClaim) error) error {
	return c.inner.EnumerateIdentities(ctx, yield)
}

func (c *observableConverter) Convert(ctx context.Context, sink importv2.Sink) (importv2.RootSpec, error) {
	defer c.onDone()
	return c.inner.Convert(ctx, sink)
}

func TestRunMemoryBoundDurableSpool(t *testing.T) {
	t.Run("the pipeline bound holds through the disk-backed spool", func(t *testing.T) {
		// given — the §5 invariant, re-proven for the split: pass 2 holds
		// O(1) objects (spooling is transient residency), pass 3 the same
		// 2C+K the lanes always bounded. The disk absorbs the rest.
		fx := newEngineFixture(t)
		spool, spillDir := durableSpool(t)
		fx.deps.Spool = spool
		fx.deps.SpillDir = spillDir
		var inFlight, maxInFlight atomic.Int64
		fx.deps.Gauge = func(delta int) {
			now := inFlight.Add(int64(delta))
			for {
				max := maxInFlight.Load()
				if now <= max || maxInFlight.CompareAndSwap(max, now) {
					break
				}
			}
		}
		objects := make([]*importv2.Object, 2000)
		for i := range objects {
			objects[i] = pageObj(fmt.Sprintf("p-%04d.md", i), false)
		}
		converter := &scriptConverter{objects: objects}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, int64(2000), result.Created)
		assert.LessOrEqual(t, maxInFlight.Load(), int64(2*channelCapacity+workerCount+2),
			"heavy-object residency must stay bounded by the pipeline, not the source")
	})
}

// lateClaimConverter models second-chance discovery: mid-pass-2 it claims an
// entity it only just found, then emits that entity's object.
type lateClaimConverter struct {
	scriptConverter
	late *importv2.Object
}

func (c *lateClaimConverter) Convert(ctx context.Context, sink importv2.Sink) (importv2.RootSpec, error) {
	if _, err := c.scriptConverter.Convert(ctx, sink); err != nil {
		return importv2.RootSpec{}, err
	}
	if err := sink.Claim(ctx, importv2.IdentityClaim{SourceKey: c.late.SourceKey, SbType: c.late.SbType}); err != nil {
		return importv2.RootSpec{}, err
	}
	if err := sink.Object(ctx, c.late); err != nil {
		return importv2.RootSpec{}, err
	}
	return c.rootSpec, nil
}

// notingSpool logs every append into the identity fake's merged event log.
type notingSpool struct {
	Spool
	identity *fakeIdentity
}

func (s *notingSpool) Append(ctx context.Context, o *importv2.Object) error {
	if err := s.Spool.Append(ctx, o); err != nil {
		return err
	}
	s.identity.note("append:" + o.SourceKey)
	return nil
}

func TestLateClaimDurabilityOrder(t *testing.T) {
	t.Run("a pass-2 late claim reaches the ledger BEFORE its object reaches the spool", func(t *testing.T) {
		// given — the P0-D shape: spool.Append commits immediately while the
		// claim batch used to flush only at the end of pass 2. A process kill
		// between the two left a spool row the claim ledger never got, and the
		// crawl-resumed pass 3 then failed the whole import on
		// 'object was not claimed in pass 1'. The write-ahead rule: a claim is
		// durable before any spool row that depends on it.
		fx := newEngineFixture(t)
		fx.deps.Spool = &notingSpool{Spool: fx.deps.Spool, identity: fx.identity}
		converter := &lateClaimConverter{
			scriptConverter: scriptConverter{objects: []*importv2.Object{pageObj("a.md", false)}},
			late:            pageObj("late.md", false),
		}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then: claim:late.md … flush … append:late.md, in that order
		require.NoError(t, result.Err)
		events := fx.identity.eventLog()
		claimAt, flushAfterClaim, appendAt := -1, -1, -1
		for i, event := range events {
			switch {
			case event == "claim:late.md":
				claimAt = i
			case event == "flush" && claimAt >= 0 && flushAfterClaim < 0:
				flushAfterClaim = i
			case event == "append:late.md":
				appendAt = i
			}
		}
		require.GreaterOrEqual(t, claimAt, 0, "the late claim must be recorded: %v", events)
		require.GreaterOrEqual(t, appendAt, 0, "the late object must be spooled: %v", events)
		require.GreaterOrEqual(t, flushAfterClaim, 0,
			"the late claim must flush before its spool row, not at pass end: %v", events)
		assert.Less(t, flushAfterClaim, appendAt,
			"the claim must be durable BEFORE the spool row exists: %v", events)
	})
}
