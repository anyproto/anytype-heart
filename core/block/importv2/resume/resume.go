// Package resume glues the durable run store onto the engine and identity
// seams — ONE implementation of the durable wiring (claim ledger, issue
// recorder) and of the pass-3 restart's rehydration (DM spec §8.1), shared
// by the adapter, the startup sweep and the test harnesses. Three review
// rounds of this work were consumed by rules fixed in one package and left
// broken in a sibling; this package exists so the restart rules cannot
// fork.
package resume

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/engine"
	"github.com/anyproto/anytype-heart/core/block/importv2/identity"
	"github.com/anyproto/anytype-heart/core/block/importv2/persist"
	"github.com/anyproto/anytype-heart/core/block/importv2/report"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("import-v2-resume")

// State is everything a pass-3 restart rehydrates from a run dir: the
// identity seeds, the engine's resume inputs, and the heal set. It is a
// pure function of the store — no source, no network, no token.
type State struct {
	Manifest runstore.Manifest
	// SpoolCount is the replay's row count (progress total for the resumed
	// incarnation).
	SpoolCount int
	// FilesDone counts completed uploads (the files-ledger rows) — the
	// status surface's separate file counter (§15.4).
	FilesDone int64
	// Engine seeds engine.Resume.
	Engine engine.ResumeState

	identityEntries []identity.RehydratedEntry
	identityFiles   []identity.RehydratedFile
	healKeys        map[string]struct{}
	compensation    runstore.CompensationInputs
}

// IdentityOption seeds a fresh identity.Service with the rehydrated index.
func (st *State) IdentityOption() identity.Option {
	return identity.WithRehydrated(st.identityEntries, st.identityFiles)
}

// SeedJournal pre-loads the resumed incarnation's journal with the
// ledger's compensation view, so IN-PROCESS compensation (a user cancel on
// the resumed run, a fatal under ALL_OR_NOTHING) covers every incarnation
// — the one compensation rule, in-process and sweep alike. Without it a
// resumed abort deleted only its own objects, reported Leaked: 0, and the
// dir — the only record of the rest — was dropped as settled.
func (st *State) SeedJournal(j *persist.Journal) {
	// CompensationInputs is newest-first; the journal appends in effect
	// order and reverses at Compensate — seed oldest-first so the merged
	// order stays newest-first overall.
	j.Seed(reverse(st.compensation.Created), reverse(st.compensation.OwnedFiles), st.compensation.Updated)
}

func reverse(ids []string) []string {
	out := make([]string, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		out = append(out, ids[i])
	}
	return out
}

// Heal is the persister's resumed-incarnation ErrTreeExists policy
// (persist.SetResumeHeal): true exactly for keys whose ledger row proves an
// interrupted create by this run (minted, non-terminal).
func (st *State) Heal() func(sourceKey string) bool {
	return func(sourceKey string) bool {
		_, ok := st.healKeys[sourceKey]
		return ok
	}
}

// Load reads a run dir's ledger into a restart seed. Strict by design: a
// row Load cannot classify fails the resume (the sweep's attempt cap then
// routes the run to compensation) rather than silently replaying wrong.
func Load(ctx context.Context, store *runstore.Store) (*State, error) {
	manifest, err := store.Manifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	rootSpec, _, err := store.ReadRootSpec(ctx)
	if err != nil {
		return nil, err
	}
	spool, err := store.Spool(ctx)
	if err != nil {
		return nil, err
	}
	spoolKeys, spoolCount, err := spool.SourceKeys(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := store.ReadEntries(ctx)
	if err != nil {
		return nil, err
	}
	files, err := store.ReadFiles(ctx)
	if err != nil {
		return nil, err
	}
	issueRecords, err := store.ReadIssues(ctx)
	if err != nil {
		return nil, err
	}
	// The ledger's compensation view seeds the resumed journal (SeedJournal)
	// — read here so the seed is part of the same load, not a second scan at
	// abort time.
	compensation, err := store.CompensationInputs(ctx)
	if err != nil {
		return nil, err
	}

	st := &State{
		Manifest:     manifest,
		SpoolCount:   spoolCount,
		healKeys:     map[string]struct{}{},
		compensation: compensation,
	}
	skip := map[string]struct{}{}
	var created, updated int64
	var rootCandidateId string
	rootCandidateRank := -1
	var reportId string
	for _, entry := range entries {
		_, inSpool := spoolKeys[entry.SourceKey]
		if entry.Late && !inSpool {
			// A finalize-stage row (root collection, report page): it has no
			// spool row to reconcile against and is never re-claimed.
			if !entry.Terminal {
				// The interrupted finalize claim is dropped: the resumed
				// finalize re-claims a FRESH key (the collection name is
				// date-suffixed). The abandoned minted id stays in the
				// ledger — delete-tolerated if the run later aborts.
				continue
			}
			switch {
			case entry.SourceKey == report.SourceKey:
				reportId = entry.ObjectId
			case !entry.Matched && entry.Rank > rootCandidateRank:
				// The root collection is the highest-rank late minted row
				// (finalize runs last; the report row is excluded by key).
				rootCandidateId = entry.ObjectId
				rootCandidateRank = entry.Rank
			}
			continue
		}
		if !entry.Matched && !entry.Terminal && len(entry.PayloadRoot) == 0 {
			// A minted claim without its payload cannot be replayed — the id
			// is the hash of exactly those bytes, and re-minting would break
			// every spooled reference. RecordClaims writes both in one tx,
			// so this shape is corruption: fail the resume loudly (the
			// sweep's attempt cap then routes the run to compensation).
			return nil, fmt.Errorf("minted claim %q has no create payload; the run cannot be resumed", entry.SourceKey)
		}
		st.identityEntries = append(st.identityEntries, identity.RehydratedEntry{
			SourceKey:    entry.SourceKey,
			ObjectId:     entry.ObjectId,
			Matched:      entry.Matched,
			Terminal:     entry.Terminal,
			PayloadRoot:  entry.PayloadRoot,
			PayloadHeads: entry.PayloadHeads,
		})
		if entry.Terminal {
			skip[entry.SourceKey] = struct{}{}
			switch entry.Action {
			case "created":
				created++
			case "updated":
				updated++
			}
			continue
		}
		if !entry.Matched {
			st.healKeys[entry.SourceKey] = struct{}{}
		}
	}
	for _, file := range files {
		st.identityFiles = append(st.identityFiles, identity.RehydratedFile{
			SourceKey: file.SourceKey,
			ObjectId:  file.ObjectId,
		})
		skip[file.SourceKey] = struct{}{}
		created++ // persistFile reports every completed upload as created
		st.FilesDone++
	}

	issues := make([]importv2.Issue, 0, len(issueRecords))
	for _, record := range issueRecords {
		if record.Code == string(importv2.IssueCancelled) {
			// The interrupted incarnation's own abort record (suspend or
			// cancel classified fatal) — lifecycle noise, not content: it
			// must not reach the resumed run's report as if an object had a
			// problem.
			continue
		}
		issue := importv2.Issue{
			Severity:  importv2.Severity(record.Severity),
			Code:      importv2.IssueCode(record.Code),
			SourceKey: record.SourceKey,
			ObjectId:  record.ObjectId,
			Message:   record.Message,
		}
		if record.Error != "" {
			issue.Err = errors.New(record.Error)
		}
		issues = append(issues, issue)
	}

	st.Engine = engine.ResumeState{
		RootSpec:         rootSpec,
		ConverterName:    manifest.Converter,
		SkipKeys:         skip,
		RootCollectionId: rootCandidateId,
		ReportObjectId:   reportId,
		Created:          created,
		Updated:          updated,
		Issues:           issues,
	}
	return st, nil
}

// ledgerWriteTimeout bounds one detached durable write (the P0-1 rule:
// intent and issues must land even when the run context is dying).
const ledgerWriteTimeout = 10 * time.Second

// claimLedger adapts identity's claim records onto the run store.
type claimLedger struct {
	store *runstore.Store
}

func (l *claimLedger) RecordClaims(ctx context.Context, claims []identity.ClaimLedgerRecord) error {
	// Intent must land even when the run context is dying: detach, bounded.
	ctx, cancel := context.WithTimeout(context.Background(), ledgerWriteTimeout)
	defer cancel()
	records := make([]runstore.ClaimRecord, 0, len(claims))
	for _, claim := range claims {
		records = append(records, runstore.ClaimRecord{
			SourceKey:    claim.SourceKey,
			ObjectId:     claim.ObjectId,
			Matched:      claim.Matched,
			PayloadRoot:  claim.PayloadRoot,
			PayloadHeads: claim.PayloadHeads,
		})
	}
	return l.store.RecordClaims(ctx, records)
}

// ClaimLedgerOption wires the durable claim ledger into an identity
// service.
func ClaimLedgerOption(store *runstore.Store) identity.Option {
	return identity.WithClaimLedger(&claimLedger{store: store})
}

// IssueRecorder returns the engine's OnIssue hook writing every retained
// issue to the durable ledger (pass-2 issues must survive to the pass-3
// report, DM spec §6.2), capped like the in-memory list. Errors degrade to
// a log line: an issue-ledger problem must never abort a run that is
// otherwise fine.
func IssueRecorder(store *runstore.Store) func(importv2.Issue) {
	var count atomic.Int64
	return func(issue importv2.Issue) {
		if count.Add(1) > importv2.IssueCap {
			return
		}
		record := runstore.IssueRecord{
			Severity:  int(issue.Severity),
			Code:      string(issue.Code),
			SourceKey: issue.SourceKey,
			ObjectId:  issue.ObjectId,
			Message:   issue.Message,
		}
		if issue.Err != nil {
			record.Error = issue.Err.Error()
		}
		if err := store.AppendIssue(context.Background(), record); err != nil {
			log.Errorf("append issue to run ledger: %s", err)
		}
	}
}
