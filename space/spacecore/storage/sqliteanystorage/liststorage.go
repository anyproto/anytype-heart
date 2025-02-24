package sqliteanystorage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/headsync/headstorage"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/consensus/consensusproto"
	"github.com/mattn/go-sqlite3"
)

type listStorage struct {
	id          string
	readDb      *sql.DB
	writeDb     *sql.DB
	headStorage headstorage.HeadStorage

	table string

	stmt struct {
		insertRecord       *sql.Stmt
		insertRecordIgnore *sql.Stmt
		selectById         *sql.Stmt
		countById          *sql.Stmt
		selectAfterOrder   *sql.Stmt
		selectBeforeOrder  *sql.Stmt
	}
}

var _ list.Storage = (*listStorage)(nil)

func CreateListStorage(
	ctx context.Context,
	root *consensusproto.RawRecordWithId,
	headStorage headstorage.HeadStorage,
	readDb *sql.DB,
	writeDb *sql.DB,
) (list.Storage, error) {
	spaceId := headStorage.SpaceId()
	tableName := buildAclTableName(spaceId)
	if err := createListTable(writeDb, tableName); err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}
	st := &listStorage{
		id:          root.Id,
		readDb:      readDb,
		writeDb:     writeDb,
		headStorage: headStorage,
		table:       tableName,
	}
	if err := st.initStatements(); err != nil {
		return nil, err
	}
	rec := list.StorageRecord{
		RawRecord:  root.Payload,
		Id:         root.Id,
		Order:      1,
		ChangeSize: len(root.Payload),
		PrevId:     "",
	}
	tx, err := writeDb.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginTx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	stmtTx := tx.StmtContext(ctx, st.stmt.insertRecord)
	if _, execErr := stmtTx.ExecContext(ctx,
		rec.Id,
		rec.RawRecord,
		rec.Order,
		rec.ChangeSize,
		rec.PrevId,
		time.Now().Unix(),
	); execErr != nil {
		var sqErr sqlite3.Error
		if errors.As(execErr, &sqErr) && sqErr.ExtendedCode == sqlite3.ErrConstraintPrimaryKey {
			err = list.ErrRecordAlreadyExists
			return nil, err
		}
		err = execErr
		return nil, err
	}
	up := headstorage.HeadsUpdate{
		Id:    root.Id,
		Heads: []string{root.Id},
	}
	ctx = WithTx(ctx, tx)
	if upErr := headStorage.UpdateEntryTx(ctx, up); upErr != nil {
		err = upErr
		return nil, err
	}
	if cErr := tx.Commit(); cErr != nil {
		err = cErr
		return nil, err
	}
	return st, nil
}

func NewListStorage(
	ctx context.Context,
	id string,
	headStorage headstorage.HeadStorage,
	readDb *sql.DB,
	writeDb *sql.DB,
) (list.Storage, error) {
	spaceId := headStorage.SpaceId()
	tableName := buildAclTableName(spaceId)
	st := &listStorage{
		id:          id,
		readDb:      readDb,
		writeDb:     writeDb,
		headStorage: headStorage,
		table:       tableName,
	}
	if err := st.initStatements(); err != nil {
		return nil, err
	}
	_, err := st.Get(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("not found: %w", err)
		}
		return nil, err
	}

	return st, nil
}

func (l *listStorage) Id() string {
	return l.id
}

func (l *listStorage) Root(ctx context.Context) (list.StorageRecord, error) {
	return l.Get(ctx, l.id)
}

func (l *listStorage) Head(ctx context.Context) (string, error) {
	entry, err := l.headStorage.GetEntry(ctx, l.id)
	if err != nil {
		return "", err
	}
	if len(entry.Heads) > 0 {
		return entry.Heads[0], nil
	}
	return "", nil
}

func (l *listStorage) Has(ctx context.Context, id string) (bool, error) {
	row := l.stmt.countById.QueryRowContext(ctx, id)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (l *listStorage) Get(ctx context.Context, id string) (list.StorageRecord, error) {
	row := l.stmt.selectById.QueryRowContext(ctx, id)
	var (
		dbId       string
		raw        []byte
		orderVal   int
		changeSize int
		prevId     string
		added      float64
	)
	err := row.Scan(&dbId, &raw, &orderVal, &changeSize, &prevId, &added)
	if err != nil {
		return list.StorageRecord{}, err
	}
	return list.StorageRecord{
		Id:         dbId,
		RawRecord:  raw,
		Order:      orderVal,
		ChangeSize: changeSize,
		PrevId:     prevId,
	}, nil
}

func (l *listStorage) GetAfterOrder(ctx context.Context, order int, iter list.StorageIterator) error {
	rows, err := l.stmt.selectAfterOrder.QueryContext(ctx, order)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return err
		}
		cont, cbErr := iter(ctx, rec)
		if cbErr != nil {
			return cbErr
		}
		if !cont {
			break
		}
	}
	return rows.Err()
}

func (l *listStorage) GetBeforeOrder(ctx context.Context, order int, iter list.StorageIterator) error {
	rows, err := l.stmt.selectBeforeOrder.QueryContext(ctx, order)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return err
		}
		cont, cbErr := iter(ctx, rec)
		if cbErr != nil {
			return cbErr
		}
		if !cont {
			break
		}
	}
	return rows.Err()
}

func (l *listStorage) AddAll(ctx context.Context, records []list.StorageRecord) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := l.writeDb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("AddAll: begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	stmtInsert := tx.StmtContext(ctx, l.stmt.insertRecord)

	for _, r := range records {
		if _, execErr := stmtInsert.ExecContext(ctx,
			r.Id,
			r.RawRecord,
			r.Order,
			r.ChangeSize,
			r.PrevId,
			time.Now().Unix(),
		); execErr != nil {
			err = execErr
			return err
		}
	}

	lastId := records[len(records)-1].Id
	update := headstorage.HeadsUpdate{
		Id:    l.id,
		Heads: []string{lastId},
	}
	if upErr := l.headStorage.UpdateEntryTx(ctx, update); upErr != nil {
		err = upErr
		return err
	}
	err = tx.Commit()
	return err
}

func (l *listStorage) createStatement(statement string) string {
	return fmt.Sprintf(statement, l.table)
}

func createListTable(db *sql.DB, tableName string) error {
	sqlCreate := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		id TEXT PRIMARY KEY,
		raw_record BLOB,
		order_id INTEGER,
		change_size INTEGER,
		prev_id TEXT,
		added REAL
	);
	CREATE INDEX IF NOT EXISTS idx_%s_order_id ON %s(order_id);
	`, tableName, tableName, tableName)
	if _, err := db.Exec(sqlCreate); err != nil {
		return err
	}
	return nil
}

func buildAclTableName(spaceId string) string {
	return spaceId + "_" + "acl"
}

func (l *listStorage) initStatements() error {
	var err error
	insertSQL := fmt.Sprintf(`
	INSERT INTO %s (
		id,
		raw_record,
		order_id,
		change_size,
		prev_id,
		added
	) VALUES (?, ?, ?, ?, ?, ?);
	`, l.table)
	if l.stmt.insertRecord, err = l.writeDb.Prepare(l.createStatement(insertSQL)); err != nil {
		return err
	}

	// Insert ignoring duplicates
	insertIgnoreSQL := fmt.Sprintf(`
	INSERT OR IGNORE INTO %s (
		id,
		raw_record,
		order_id,
		change_size,
		prev_id,
		added
	) VALUES (?, ?, ?, ?, ?, ?);
	`, l.table)
	if l.stmt.insertRecordIgnore, err = l.writeDb.Prepare(l.createStatement(insertIgnoreSQL)); err != nil {
		return err
	}

	// Select by ID
	selectByIdSQL := fmt.Sprintf(`
	SELECT
		id,
		raw_record,
		order_id,
		change_size,
		prev_id,
		added
	FROM %s
	WHERE id = ?;
	`, l.table)
	if l.stmt.selectById, err = l.readDb.Prepare(l.createStatement(selectByIdSQL)); err != nil {
		return err
	}

	// Count by ID
	countByIdSQL := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE id = ?;`, l.table)
	if l.stmt.countById, err = l.readDb.Prepare(l.createStatement(countByIdSQL)); err != nil {
		return err
	}

	// SELECT records with order >= ?
	selectAfterOrderSQL := fmt.Sprintf(`
	SELECT
		id,
		raw_record,
		order_id,
		change_size,
		prev_id,
		added
	FROM %s
	WHERE order_id >= ?
	ORDER BY order_id;
	`, l.table)
	if l.stmt.selectAfterOrder, err = l.readDb.Prepare(l.createStatement(selectAfterOrderSQL)); err != nil {
		return err
	}

	// SELECT records with order <= ?
	selectBeforeOrderSQL := fmt.Sprintf(`
	SELECT
		id,
		raw_record,
		order_id,
		change_size,
		prev_id,
		added
	FROM %s
	WHERE order_id <= ?
	ORDER BY order_id;
	`, l.table)
	if l.stmt.selectBeforeOrder, err = l.readDb.Prepare(l.createStatement(selectBeforeOrderSQL)); err != nil {
		return err
	}

	return nil
}

// scanRecord is a helper to read a row from QueryContext.
func scanRecord(rows *sql.Rows) (list.StorageRecord, error) {
	var (
		id       string
		raw      []byte
		orderVal int
		sizeVal  int
		prevVal  string
		added    float64
	)
	if err := rows.Scan(&id, &raw, &orderVal, &sizeVal, &prevVal, &added); err != nil {
		return list.StorageRecord{}, err
	}
	return list.StorageRecord{
		Id:         id,
		RawRecord:  raw,
		Order:      orderVal,
		ChangeSize: sizeVal,
		PrevId:     prevVal,
	}, nil
}
