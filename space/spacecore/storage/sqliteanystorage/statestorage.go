package sqliteanystorage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mattn/go-sqlite3"

	"github.com/anyproto/any-sync/commonspace/headsync/statestorage"
)

type stateStorage struct {
	spaceId    string
	readDb     *sql.DB
	writeDb    *sql.DB
	observer   statestorage.Observer
	settingsId string
	aclId      string

	table string

	stmt struct {
		insertState *sql.Stmt
		selectState *sql.Stmt
		updateHash  *sql.Stmt
	}
}

var _ statestorage.StateStorage = (*stateStorage)(nil) // Ensure interface compliance

func CreateStateStorage(ctx context.Context, state statestorage.State, readDb *sql.DB, writeDb *sql.DB) (statestorage.StateStorage, error) {
	tableName := buildStateTableName(state.SpaceId)
	if err := createStateTable(writeDb, tableName); err != nil {
		return nil, fmt.Errorf("failed to create state table: %w", err)
	}
	st := &stateStorage{
		spaceId:    state.SpaceId,
		readDb:     readDb,
		writeDb:    writeDb,
		table:      tableName,
		settingsId: state.SettingsId,
		aclId:      state.AclId,
	}
	if err := st.initStatements(); err != nil {
		return nil, err
	}
	tx, err := writeDb.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateSqliteStateStorage begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	stmtInsert := tx.StmtContext(ctx, st.stmt.insertState)
	if _, execErr := stmtInsert.ExecContext(ctx,
		state.SpaceId,
		state.Hash,
		state.SettingsId,
		state.AclId,
		state.SpaceHeader,
	); execErr != nil {
		var sqliteErr sqlite3.Error
		if errors.As(execErr, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintPrimaryKey {
			err = fmt.Errorf("spaceId %s already exists", state.SpaceId)
			return nil, err
		}
		err = execErr
		return nil, err
	}
	if cErr := tx.Commit(); cErr != nil {
		err = cErr
		return nil, err
	}
	return st, nil
}

func NewStateStorage(ctx context.Context, spaceId string, readDb *sql.DB, writeDb *sql.DB) (statestorage.StateStorage, error) {
	tableName := buildStateTableName(spaceId)
	st := &stateStorage{
		spaceId: spaceId,
		readDb:  readDb,
		writeDb: writeDb,
		table:   tableName,
	}
	if err := st.initStatements(); err != nil {
		return nil, err
	}
	got, err := st.GetState(ctx)
	if err != nil {
		return nil, fmt.Errorf("NewSqliteStateStorage: get state: %w", err)
	}
	st.settingsId = got.SettingsId
	st.aclId = got.AclId
	return st, nil
}

func (s *stateStorage) GetState(ctx context.Context) (statestorage.State, error) {
	row := s.stmt.selectState.QueryRowContext(ctx, s.spaceId)
	var (
		dbSpaceId     string
		dbHash        string
		dbSettingsId  string
		dbAclId       string
		dbSpaceHeader []byte
	)
	err := row.Scan(&dbSpaceId, &dbHash, &dbSettingsId, &dbAclId, &dbSpaceHeader)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return statestorage.State{}, err
		}
		return statestorage.State{}, err
	}

	return statestorage.State{
		SpaceId:     dbSpaceId,
		Hash:        dbHash,
		SettingsId:  dbSettingsId,
		AclId:       dbAclId,
		SpaceHeader: dbSpaceHeader,
	}, nil
}

func (s *stateStorage) createStatement(statement string) string {
	return fmt.Sprintf(statement, s.table)
}

func (s *stateStorage) SettingsId() string {
	return s.settingsId
}

func (s *stateStorage) SetHash(ctx context.Context, newHash string) (err error) {
	defer func() {
		if s.observer != nil && err == nil {
			s.observer.OnHashChange(newHash)
		}
	}()

	tx, txErr := s.writeDb.BeginTx(ctx, nil)
	if txErr != nil {
		return fmt.Errorf("SetHash: begin tx: %w", txErr)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmtUpdate := tx.StmtContext(ctx, s.stmt.updateHash)
	res, execErr := stmtUpdate.ExecContext(ctx, newHash, s.spaceId)
	if execErr != nil {
		err = execErr
		return err
	}

	// Optionally check rows affected if you want to ensure the row existed
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		err = fmt.Errorf("SetHash: row for space_id %s not found", s.spaceId)
		return err
	}

	err = tx.Commit()
	return err
}

func (s *stateStorage) SetObserver(observer statestorage.Observer) {
	s.observer = observer
}

func (s *stateStorage) initStatements() error {
	var err error

	insertSQL := fmt.Sprintf(`
		INSERT INTO %s (
			space_id,
			hash,
			settings_id,
			acl_id,
			space_header
		) VALUES (?, ?, ?, ?, ?);
	`, s.table)
	if s.stmt.insertState, err = s.writeDb.Prepare(s.createStatement(insertSQL)); err != nil {
		return err
	}

	selectSQL := fmt.Sprintf(`
		SELECT
			space_id,
			hash,
			settings_id,
			acl_id,
			space_header
		FROM %s
		WHERE space_id = ?;
	`, s.table)
	if s.stmt.selectState, err = s.readDb.Prepare(s.createStatement(selectSQL)); err != nil {
		return err
	}

	updateHashSQL := fmt.Sprintf(`
		UPDATE %s
		SET hash = ?
		WHERE space_id = ?;
	`, s.table)
	if s.stmt.updateHash, err = s.writeDb.Prepare(s.createStatement(updateHashSQL)); err != nil {
		return err
	}

	return nil
}

func createStateTable(db *sql.DB, tableName string) error {
	query := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		space_id TEXT PRIMARY KEY,
		hash TEXT,
		settings_id TEXT,
		acl_id TEXT,
		space_header BLOB
	);`, tableName)
	_, err := db.Exec(query)
	return err
}

func buildStateTableName(spaceId string) string {
	id := strings.Split(spaceId, ".")[0]
	return id + "_state"
}
