package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2"
)

// ResumeCrawl is pass 2 restarted against the live source with the spool as
// the skip set: re-run pass 1 (claims are reuses), re-run the
// crawl skipping what is already recorded, then materialize the whole spool.
// The sink backstop makes the skip a correctness-free optimization: a
// converter that ignores the seam entirely re-emits everything and the
// engine drops the already-recorded rows before any download.

// skippableConverter is a scriptConverter implementing the 08-13 §6.3 seam:
// it consults Skip per object before "fetching" (emitting) it, and records
// the recovery set the engine hands it (the P0-A half of the seam).
type skippableConverter struct {
	scriptConverter
	skip      func(sourceKey string) bool
	skipCalls int
	fetched   []string
	recovered []string
}

func (c *skippableConverter) SetSkip(skip func(sourceKey string) bool) { c.skip = skip }

func (c *skippableConverter) SetRecover(keys []string) { c.recovered = keys }

func (c *skippableConverter) Convert(ctx context.Context, sink importv2.Sink) (importv2.RootSpec, error) {
	for _, o := range c.objects {
		if c.skip != nil {
			c.skipCalls++
			if c.skip(o.SourceKey) {
				continue // an already-recorded page is not fetched at all
			}
		}
		c.fetched = append(c.fetched, o.SourceKey)
		if err := sink.Object(ctx, o); err != nil {
			return importv2.RootSpec{}, err
		}
	}
	return c.rootSpec, nil
}

// spoolKeys reads the spool's current source keys through Replay.
func spoolKeys(t *testing.T, fx *engineFixture) []string {
	t.Helper()
	var keys []string
	require.NoError(t, fx.deps.Spool.Replay(context.Background(), func(o *importv2.Object) error {
		keys = append(keys, o.SourceKey)
		return nil
	}))
	return keys
}

func crawlState(spooled, prior []string) *CrawlResumeState {
	state := &CrawlResumeState{
		SpooledKeys: map[string]struct{}{},
		PriorClaims: map[string]struct{}{},
	}
	for _, key := range spooled {
		state.SpooledKeys[key] = struct{}{}
	}
	for _, key := range prior {
		state.PriorClaims[key] = struct{}{}
	}
	return state
}

func TestResumeCrawlSkipSeam(t *testing.T) {
	t.Run("a seam-aware converter never fetches recorded pages; the run converges whole", func(t *testing.T) {
		// given: incarnation 1 recorded a and b; the source still holds a-d
		fx := newEngineFixture(t)
		fillSpool(t, fx, pageObj("a", false), pageObj("b", false))
		converter := &skippableConverter{scriptConverter: scriptConverter{
			objects: []*importv2.Object{
				pageObj("a", false), pageObj("b", false), pageObj("c", false), pageObj("d", false),
			},
		}}

		// when
		result := ResumeCrawl(context.Background(), resumeRequest(), converter, fx.deps,
			crawlState([]string{"a", "b"}, []string{"a", "b"}))

		// then: only the new pages were fetched, every page materialized once
		require.NoError(t, result.Err)
		assert.Equal(t, []string{"c", "d"}, converter.fetched,
			"recorded pages must not be fetched — that is the whole point of the phase")
		assert.ElementsMatch(t, []string{"a", "b", "c", "d"}, fx.persister.persisted)
		assert.Equal(t, []string{"a", "b", "c", "d"}, spoolKeys(t, fx),
			"the spool must extend in emission order, old rows first")
		assert.Equal(t, int64(4), result.Created)
	})
}

func TestResumeCrawlRecoverSeam(t *testing.T) {
	t.Run("the engine hands the converter exactly the prior claims the spool never got", func(t *testing.T) {
		// given — review P0-A: an entity discoverable ONLY through a parent's
		// block tree (the GO-5273 class) is claimed on sight; if the crash
		// lands before its spool row, the resumed crawl skips the recorded
		// parent, never re-walks its blocks, and the entity is silently lost
		// — misreported as source drift. The claim key IS the source id, so
		// the converter can re-fetch it directly; the engine's half is
		// handing over the set: PriorClaims \ SpooledKeys, sorted.
		fx := newEngineFixture(t)
		fillSpool(t, fx, pageObj("a", false))
		converter := &skippableConverter{scriptConverter: scriptConverter{
			objects: []*importv2.Object{pageObj("a", false), pageObj("b", false)},
		}}

		// when: a spooled, b re-enumerating, c and z claimed-but-unrecorded
		result := ResumeCrawl(context.Background(), resumeRequest(), converter, fx.deps,
			crawlState([]string{"a"}, []string{"a", "z", "c", "b"}))

		// then: the unrecorded claims arrive sorted; the converter filters
		// re-encountered ones itself (b re-enumerates and converts normally)
		require.NoError(t, result.Err)
		assert.Equal(t, []string{"b", "c", "z"}, converter.recovered,
			"every prior claim without a spool row must be offered for recovery")
	})
}

func TestResumeCrawlSinkBackstop(t *testing.T) {
	t.Run("a converter ignoring the seam is merely slower, never incorrect", func(t *testing.T) {
		// given — 08-13 §6.2 item 5: the engine enforces spool dedup at the
		// sink regardless of converter cooperation.
		fx := newEngineFixture(t)
		fillSpool(t, fx, pageObj("a", false))
		converter := &scriptConverter{objects: []*importv2.Object{
			pageObj("a", false), pageObj("b", false),
		}}

		// when
		result := ResumeCrawl(context.Background(), resumeRequest(), converter, fx.deps,
			crawlState([]string{"a"}, []string{"a"}))

		// then: no duplicate spool row, both pages persisted exactly once
		require.NoError(t, result.Err)
		assert.Equal(t, []string{"a", "b"}, spoolKeys(t, fx),
			"a re-emitted recorded row must not be appended twice")
		assert.ElementsMatch(t, []string{"a", "b"}, fx.persister.persisted)
	})

	t.Run("the backstop precedes the file drain: a recorded file is not re-downloaded", func(t *testing.T) {
		// given: a file recorded (and spilled) by incarnation 1, re-emitted by
		// a seam-ignorant converter with a source path that NO LONGER EXISTS —
		// if the backstop sat after the drain, the open would fail the object.
		fx := newEngineFixture(t)
		recorded := fileObj("img.png")
		fillSpool(t, fx, recorded, pageObj("a", false))
		ghost := &importv2.Object{
			SourceKey: "img.png",
			SbType:    recorded.SbType,
			Payload:   &importv2.Snapshot{},
			File:      &importv2.FileSource{Path: "/nonexistent/img.png", Name: "img.png"},
		}
		converter := &scriptConverter{objects: []*importv2.Object{ghost, pageObj("a", false)}}

		// when
		result := ResumeCrawl(context.Background(), resumeRequest(), converter, fx.deps,
			crawlState([]string{"img.png", "a"}, []string{"a"}))

		// then
		require.NoError(t, result.Err)
		assert.Empty(t, result.Issues, "the recorded file must be dropped before any download")
		assert.ElementsMatch(t, []string{"img.png", "a"}, fx.persister.persisted)
	})
}

func TestResumeCrawlStaleClaims(t *testing.T) {
	t.Run("a prior claim whose entity disappeared warns instead of failing the run", func(t *testing.T) {
		// given — 08-13 §5.4: a page deleted from the source between sessions
		// is expected drift on a resumed run, not a converter bug. It was
		// claimed by incarnation 1, never spooled, and no longer enumerates.
		fx := newEngineFixture(t)
		fillSpool(t, fx, pageObj("a", false))
		// the rehydrated index holds the prior claim (as identity.WithRehydrated would)
		fx.identity.claims = append(fx.identity.claims, importv2.IdentityClaim{SourceKey: "vanished"})
		converter := &scriptConverter{objects: []*importv2.Object{pageObj("a", false)}}

		// when
		result := ResumeCrawl(context.Background(), resumeRequest(), converter, fx.deps,
			crawlState([]string{"a"}, []string{"a", "vanished"}))

		// then: the run succeeds with a loud data-loss warning, not a failure
		require.NoError(t, result.Err)
		assert.Zero(t, result.Failed)
		require.Len(t, result.Issues, 1)
		assert.Equal(t, importv2.SeverityWarning, result.Issues[0].Severity)
		assert.Equal(t, importv2.IssueDataLoss, result.Issues[0].Code)
		assert.Equal(t, "vanished", result.Issues[0].SourceKey)
		// The wording states what the engine KNOWS — the claim was not found
		// again — never a deletion it cannot establish (review P0-A: for a
		// converter with incomplete enumeration the entity may still exist;
		// such converters carry their own recovery and their own precise
		// messages, and this fallback must stay honest for the rest).
		assert.NotContains(t, result.Issues[0].Message, "disappeared",
			"the fallback warning must not assert a deletion the engine cannot establish")
	})

	t.Run("a claim RE-ENUMERATED this incarnation and then dropped is still a converter bug", func(t *testing.T) {
		// given: the source still holds the entity (pass 1 re-claims it), the
		// converter then silently never emits it — the invariant's teeth stay.
		fx := newEngineFixture(t)
		fillSpool(t, fx, pageObj("a", false))
		converter := &gapConverter{
			scriptConverter: scriptConverter{objects: []*importv2.Object{pageObj("a", false)}},
			gapKeys:         []string{"dropped"},
		}

		// when
		result := ResumeCrawl(context.Background(), resumeRequest(), converter, fx.deps,
			crawlState([]string{"a"}, []string{"a", "dropped"}))

		// then
		require.NoError(t, result.Err) // continue-on-error mode
		assert.Equal(t, int64(1), result.Failed)
		require.NotEmpty(t, result.Issues)
		assert.Equal(t, importv2.IssueInvariant, result.Issues[0].Code,
			"a silent drop of a live entity must stay fatal-shaped whatever incarnation claims it")
	})
}

func TestResumeCrawlSeedsIssues(t *testing.T) {
	t.Run("prior incarnations' issues ride the result without re-aborting", func(t *testing.T) {
		// given
		fx := newEngineFixture(t)
		fillSpool(t, fx, pageObj("a", false))
		converter := &scriptConverter{objects: []*importv2.Object{pageObj("a", false)}}

		// when
		result := ResumeCrawl(context.Background(), resumeRequest(), converter, fx.deps,
			&CrawlResumeState{
				SpooledKeys: map[string]struct{}{"a": {}},
				PriorClaims: map[string]struct{}{"a": {}},
				Issues: []importv2.Issue{{
					Severity: importv2.SeverityWarning,
					Code:     importv2.IssueDataLoss,
					Message:  "recorded in a previous incarnation",
				}},
			})

		// then
		require.NoError(t, result.Err)
		require.Len(t, result.Issues, 1)
		assert.NotEmpty(t, result.ReportObjectId, "seeded issues must reach the report")
	})
}

func TestResumeCrawlRequiresDurableSpool(t *testing.T) {
	t.Run("a nil spool fails inert: a memory spool cannot hold incarnation 1's rows", func(t *testing.T) {
		// given — the Resume sibling rule (review P1-D): the run never
		// started, so nothing may be undone and the dir must survive.
		fx := newEngineFixture(t)
		fx.deps.Spool = nil
		converter := &scriptConverter{objects: []*importv2.Object{pageObj("a", false)}}

		// when
		result := ResumeCrawl(context.Background(), resumeRequest(), converter, fx.deps,
			crawlState([]string{"a"}, []string{"a"}))

		// then
		require.Error(t, result.Err)
		assert.False(t, result.CompensationRan)
		assert.Empty(t, fx.persister.persisted)
	})
}
