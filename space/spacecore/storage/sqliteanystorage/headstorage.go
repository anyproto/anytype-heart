package sqliteanystorage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/headsync/headstorage"
)

// Import headstorage types via type alias.
type HeadsEntry = headstorage.HeadsEntry
type HeadsUpdate = headstorage.HeadsUpdate
type DeletedStatus = headstorage.DeletedStatus

type headStorage struct {
	readDb    *sql.DB
	writeDb   *sql.DB
	table     string
	stmts     stmts
	spaceId   string
	observers []headstorage.Observer
}

// stmts wraps our prepared statements.
type stmts struct {
	createTable   *sql.Stmt
	selectByID    *sql.Stmt
	selectDeleted *sql.Stmt
	selectNotDel  *sql.Stmt
	upsertHeads   *sql.Stmt
	deleteByID    *sql.Stmt
}

type txContextKey struct{}

func WithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

func TxFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(txContextKey{}).(*sql.Tx)
	return tx, ok
}

func getOrBeginTx(ctx context.Context, db *sql.DB) (*sql.Tx, bool, error) {
	if tx, ok := TxFromContext(ctx); ok && tx != nil {
		return tx, false, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin transaction: %w", err)
	}
	return tx, true, nil
}

func (h *headStorage) createStatement(statement string) string {
	return fmt.Sprintf(statement, h.table)
}

func NewHeadStorage(readDb *sql.DB, writeDb *sql.DB, spaceId string) (headstorage.HeadStorage, error) {
	id := strings.Split(spaceId, ".")[0]
	h := &headStorage{
		readDb:  readDb,
		writeDb: writeDb,
		table:   id + "_heads",
		spaceId: spaceId,
	}
	createTable := h.createStatement(`
	CREATE TABLE IF NOT EXISTS %s (
	  id TEXT PRIMARY KEY,
	  heads TEXT,
	  common_snapshot TEXT,
	  deleted_status INT,
	  is_derived BOOLEAN
	);`)
	if _, err := writeDb.Exec(createTable); err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}
	if err := h.initStatements(); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *headStorage) SpaceId() string {
	return h.spaceId
}

func (h *headStorage) initStatements() error {
	var err error

	if h.stmts.selectByID, err = h.readDb.Prepare(h.createStatement(`
		SELECT id, heads, common_snapshot, deleted_status, is_derived
		FROM %s
		WHERE id = ?
	`)); err != nil {
		return fmt.Errorf("prepare selectByID: %w", err)
	}

	if h.stmts.selectDeleted, err = h.readDb.Prepare(h.createStatement(`
		SELECT id, heads, common_snapshot, deleted_status, is_derived
		FROM %s
		WHERE deleted_status >= 1
		ORDER BY id
	`)); err != nil {
		return fmt.Errorf("prepare selectDeleted: %w", err)
	}

	if h.stmts.selectNotDel, err = h.readDb.Prepare(h.createStatement(`
		SELECT id, heads, common_snapshot, deleted_status, is_derived
		FROM %s
		WHERE deleted_status < 1
		ORDER BY id
	`)); err != nil {
		return fmt.Errorf("prepare selectNotDel: %w", err)
	}

	if h.stmts.upsertHeads, err = h.writeDb.Prepare(h.createStatement(`
	INSERT INTO %s (id, heads, common_snapshot, deleted_status, is_derived)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
	  heads = COALESCE(EXCLUDED.heads, heads),
	  common_snapshot = COALESCE(EXCLUDED.common_snapshot, common_snapshot),
	  deleted_status = COALESCE(EXCLUDED.deleted_status, deleted_status),
	  is_derived = COALESCE(EXCLUDED.is_derived, is_derived)
`)); err != nil {
		return fmt.Errorf("prepare upsertHeads: %w", err)
	}

	if h.stmts.deleteByID, err = h.writeDb.Prepare(h.createStatement(`
		DELETE FROM %s
		WHERE id = ?
	`)); err != nil {
		return fmt.Errorf("prepare deleteByID: %w", err)
	}

	return nil
}

func (h *headStorage) IterateEntries(ctx context.Context, iterOpts headstorage.IterOpts, iterFunc headstorage.EntryIterator) error {
	var rows *sql.Rows
	var err error

	if iterOpts.Deleted {
		rows, err = h.stmts.selectDeleted.QueryContext(ctx)
	} else {
		rows, err = h.stmts.selectNotDel.QueryContext(ctx)
	}
	if err != nil {
		return fmt.Errorf("IterateEntries: query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id             string
			headsStr       sql.NullString
			commonSnapshot sql.NullString
			deletedStatus  sql.NullInt64
			isDerived      sql.NullBool
		)
		if err = rows.Scan(&id, &headsStr, &commonSnapshot, &deletedStatus, &isDerived); err != nil {
			return fmt.Errorf("IterateEntries scan: %w", err)
		}
		entry := HeadsEntry{
			Id:             id,
			Heads:          parseHeadsCommaSeparated(headsStr.String),
			CommonSnapshot: commonSnapshot.String,
			DeletedStatus:  DeletedStatus(deletedStatus.Int64),
			IsDerived:      isDerived.Bool,
		}
		cont, err := iterFunc(entry)
		if err != nil {
			return err
		}
		if !cont {
			break
		}
	}
	return rows.Err()
}

func (h *headStorage) GetEntry(ctx context.Context, id string) (HeadsEntry, error) {
	row := h.stmts.selectByID.QueryRowContext(ctx, id)
	var (
		headsStr       sql.NullString
		commonSnapshot sql.NullString
		deletedStatus  sql.NullInt64
		isDerived      sql.NullBool
	)
	err := row.Scan(&id, &headsStr, &commonSnapshot, &deletedStatus, &isDerived)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return HeadsEntry{}, fmt.Errorf("GetEntry: no such entry: %s %w", id, anystore.ErrDocNotFound)
		}
		return HeadsEntry{}, err
	}

	return HeadsEntry{
		Id:             id,
		Heads:          parseHeadsCommaSeparated(headsStr.String),
		CommonSnapshot: commonSnapshot.String,
		DeletedStatus:  DeletedStatus(deletedStatus.Int64),
		IsDerived:      isDerived.Bool,
	}, nil
}

func (h *headStorage) UpdateEntry(ctx context.Context, update HeadsUpdate) (err error) {
	tx, isNew, err := getOrBeginTx(ctx, h.writeDb)
	if err != nil {
		return fmt.Errorf("UpdateEntry: %w", err)
	}
	if isNew {
		ctx = WithTx(ctx, tx)
	}
	if err = h.UpdateEntryTx(ctx, update); err != nil {
		if isNew {
			_ = tx.Rollback()
		}
		return fmt.Errorf("UpdateEntry: %w", err)
	}
	if isNew {
		if commitErr := tx.Commit(); commitErr != nil {
			return fmt.Errorf("UpdateEntry: commit: %w", commitErr)
		}
	}
	return nil
}

func (h *headStorage) UpdateEntryTx(ctx context.Context, update HeadsUpdate) (err error) {
	defer func() {
		if err == nil {
			for _, observer := range h.observers {
				observer.OnUpdate(update)
			}
		}
	}()
	tx, ok := TxFromContext(ctx)
	if !ok || tx == nil {
		return fmt.Errorf("UpdateEntryTx: no transaction in context")
	}

	var headsVal interface{}
	if update.Heads != nil {
		headsVal = strings.Join(update.Heads, ",")
	} else {
		headsVal = nil
	}

	// For common_snapshot: pass the dereferenced value if provided; else nil.
	var snapVal interface{}
	if update.CommonSnapshot != nil {
		snapVal = *update.CommonSnapshot
	} else {
		snapVal = nil
	}

	// For deleted_status: convert to int64 if provided; else nil.
	var delVal interface{}
	if update.DeletedStatus != nil {
		delVal = int64(*update.DeletedStatus)
	} else {
		delVal = nil
	}

	// For is_derived: pass the bool if provided; else nil.
	var derivedVal interface{}
	if update.IsDerived != nil {
		derivedVal = *update.IsDerived
	} else {
		derivedVal = nil
	}

	// Bind the upsert prepared statement to the transaction.
	stmtTx := tx.StmtContext(ctx, h.stmts.upsertHeads)
	_, err = stmtTx.ExecContext(ctx,
		update.Id,  // id is required
		headsVal,   // heads (comma-separated string or nil)
		snapVal,    // common_snapshot (or nil)
		delVal,     // deleted_status (or nil)
		derivedVal, // is_derived (or nil)
	)
	if err != nil {
		return fmt.Errorf("UpdateEntryTx upsert: %w", err)
	}

	return nil
}

func (h *headStorage) DeleteEntryTx(ctx context.Context, id string) error {
	tx, isNew, err := getOrBeginTx(ctx, h.writeDb)
	if err != nil {
		return fmt.Errorf("DeleteEntryTx: %w", err)
	}
	if isNew {
		ctx = WithTx(ctx, tx)
	}

	stmtTx := tx.StmtContext(ctx, h.stmts.deleteByID)
	_, err = stmtTx.ExecContext(ctx, id)
	if err != nil {
		if isNew {
			_ = tx.Rollback()
		}
		return fmt.Errorf("DeleteEntryTx: %w", err)
	}

	if isNew {
		if commitErr := tx.Commit(); commitErr != nil {
			return fmt.Errorf("DeleteEntryTx: commit: %w", commitErr)
		}
	}
	return nil
}

func (h *headStorage) AddObserver(observer headstorage.Observer) {
	h.observers = append(h.observers, observer)
}

func parseHeadsCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	return parts
}
