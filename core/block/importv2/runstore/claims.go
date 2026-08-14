package runstore

import (
	"context"
	"fmt"
)

const (
	collPayloads  = "payloads"
	collIssues    = "issues"
	statusClaimed = "claimed"
)

// ClaimRecord is one pass-1 identity decision, durably recorded (08-13 spec
// §4.2/§5.2, pulled forward by DM-1): the minted id doubles as write-ahead
// intent — any minted id in the ledger that exists in the space is
// attributable to this run from claim time — and the retained create
// payload is what makes a later pass-3 restart mint nothing.
type ClaimRecord struct {
	SourceKey    string
	ObjectId     string
	Matched      bool
	PayloadRoot  []byte // RawTreeChangeWithId proto; nil for matched claims
	PayloadHeads []string
}

// RecordClaims writes one batch in a single transaction (spec §5.2: batched
// because an unflushed batch's loss is harmless — no side effects exist at
// claim time; measured 3-4x over per-claim commits).
func (s *Store) RecordClaims(ctx context.Context, claims []ClaimRecord) error {
	if len(claims) == 0 {
		return nil
	}
	tx, err := s.db.WriteTx(ctx)
	if err != nil {
		return fmt.Errorf("claims tx: %w", err)
	}
	txCtx := tx.Context()
	arena := s.arenas.Get()
	defer func() {
		arena.Reset()
		s.arenas.Put(arena)
	}()
	for _, claim := range claims {
		mode := modeMinted
		if claim.Matched {
			mode = modeMatched
		}
		row := arena.NewObject()
		row.Set("id", arena.NewString(claim.SourceKey))
		row.Set("objectId", arena.NewString(claim.ObjectId))
		row.Set("mode", arena.NewString(mode))
		row.Set("status", arena.NewString(statusClaimed))
		row.Set("rank", arena.NewNumberInt(int(s.rank.Add(1))))
		row.Set("incarnation", arena.NewNumberInt(1))
		if err = s.entries.UpsertOne(txCtx, row); err != nil {
			return fmt.Errorf("claim %q: %w", claim.SourceKey, rollback(tx.Rollback(), err))
		}
		arena.Reset()
		if !claim.Matched && len(claim.PayloadRoot) > 0 {
			payload := arena.NewObject()
			payload.Set("id", arena.NewString(claim.ObjectId))
			payload.Set("root", arena.NewBinary(claim.PayloadRoot))
			heads := arena.NewArray()
			for i, head := range claim.PayloadHeads {
				heads.SetArrayItem(i, arena.NewString(head))
			}
			payload.Set("heads", heads)
			if err = s.payloads.UpsertOne(txCtx, payload); err != nil {
				return fmt.Errorf("payload %q: %w", claim.ObjectId, rollback(tx.Rollback(), err))
			}
			arena.Reset()
		}
	}
	return tx.Commit()
}

func rollback(rollbackErr, err error) error {
	if rollbackErr != nil {
		return fmt.Errorf("%w (rollback: %s)", err, rollbackErr)
	}
	return err
}

// IssueRecord is one durable issue-ledger row: pass-2 issues must survive to
// pass 3's report page (DM spec §6.2). Flattened strings — the wire Issue's
// error chain does not round-trip and does not need to.
type IssueRecord struct {
	Severity  int
	Code      string
	SourceKey string
	ObjectId  string
	Message   string
	Error     string
}

// AppendIssue appends one row in arrival order, capped by the caller (the
// adapter enforces IssueCap, mirroring the in-memory ledger).
func (s *Store) AppendIssue(ctx context.Context, rec IssueRecord) error {
	arena := s.arenas.Get()
	defer func() {
		arena.Reset()
		s.arenas.Put(arena)
	}()
	row := arena.NewObject()
	row.Set("id", arena.NewString(fmt.Sprintf("%012d", s.issueSeq.Add(1))))
	row.Set("severity", arena.NewNumberInt(rec.Severity))
	row.Set("code", arena.NewString(rec.Code))
	row.Set("sourceKey", arena.NewString(rec.SourceKey))
	row.Set("objectId", arena.NewString(rec.ObjectId))
	row.Set("message", arena.NewString(rec.Message))
	row.Set("error", arena.NewString(rec.Error))
	return s.issues.UpsertOne(ctx, row)
}

// ReadIssues returns the durable issue ledger in arrival order.
func (s *Store) ReadIssues(ctx context.Context) ([]IssueRecord, error) {
	iter, err := s.issues.Find(nil).Sort("id").Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("iterate issues: %w", err)
	}
	defer iter.Close()
	var records []IssueRecord
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("read issue doc: %w", err)
		}
		v := doc.Value()
		records = append(records, IssueRecord{
			Severity:  v.GetInt("severity"),
			Code:      string(v.GetStringBytes("code")),
			SourceKey: string(v.GetStringBytes("sourceKey")),
			ObjectId:  string(v.GetStringBytes("objectId")),
			Message:   string(v.GetStringBytes("message")),
			Error:     string(v.GetStringBytes("error")),
		})
	}
	return records, nil
}
