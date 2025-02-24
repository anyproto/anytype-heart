package sqliteanystorage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/lexid"
	"github.com/mattn/go-sqlite3"

	"github.com/anyproto/any-sync/commonspace/headsync/headstorage"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
)

var lexId = lexid.Must(lexid.CharsAllNoEscape, 4, 100)

type treeStorage struct {
	id          string
	readDb      *sql.DB
	writeDb     *sql.DB
	headStorage headstorage.HeadStorage
	table       string

	// root is the initial "root" change for this tree
	root objecttree.StorageChange

	// Prepared statements
	stmt struct {
		insertChange       *sql.Stmt
		insertChangeIgnore *sql.Stmt
		selectById         *sql.Stmt
		countById          *sql.Stmt
		selectAfterOrder   *sql.Stmt
		deleteByTree       *sql.Stmt
	}
}

func CreateTreeStorage(
	ctx context.Context,
	rawRoot *treechangeproto.RawTreeChangeWithId,
	headStorage headstorage.HeadStorage,
	readDb *sql.DB,
	writeDb *sql.DB,
) (objecttree.Storage, error) {
	builder := objecttree.NewChangeBuilder(crypto.NewKeyStorage(), rawRoot)
	res, err := builder.Unmarshall(rawRoot, true)
	if err != nil {
		return nil, err
	}
	firstOrder := lexId.Next("")
	root := objecttree.StorageChange{
		RawChange:       rawRoot.RawChange,
		Id:              rawRoot.Id,
		SnapshotCounter: 1,
		SnapshotId:      "",
		OrderId:         firstOrder,
		TreeId:          rawRoot.Id,
		ChangeSize:      len(rawRoot.RawChange),
	}
	st := &treeStorage{
		id:          root.Id,
		readDb:      readDb,
		writeDb:     writeDb,
		headStorage: headStorage,
		root:        root,
		table:       headStorage.SpaceId() + "_changes",
	}
	if err := st.initStatements(); err != nil {
		return nil, err
	}
	tx, err := writeDb.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, execErr := tx.StmtContext(ctx, st.stmt.insertChange).ExecContext(
		ctx,
		root.Id,
		root.RawChange,
		root.SnapshotId,
		root.OrderId,
		root.ChangeSize,
		root.SnapshotCounter,
		strings.Join(root.PrevIds, ","),
		root.TreeId,
		time.Now().Unix(),
	); execErr != nil {
		var sqliteErr sqlite3.Error
		if errors.As(execErr, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintPrimaryKey {
			err = treestorage.ErrTreeExists
			return nil, err
		}
		err = execErr
		return nil, err
	}

	update := headstorage.HeadsUpdate{
		Id:             root.Id,
		Heads:          []string{root.Id},
		CommonSnapshot: &root.Id,
		IsDerived:      &res.IsDerived,
	}
	ctx = WithTx(ctx, tx)
	if upErr := headStorage.UpdateEntryTx(ctx, update); upErr != nil {
		err = upErr
		return nil, err
	}
	if cErr := tx.Commit(); cErr != nil {
		err = cErr
		return nil, err
	}

	return st, nil
}

func (t *treeStorage) createStatement(statement string) string {
	return fmt.Sprintf(statement, t.table)
}

func NewTreeStorage(
	ctx context.Context,
	id string,
	headStorage headstorage.HeadStorage,
	readDb *sql.DB,
	writeDb *sql.DB,
) (objecttree.Storage, error) {
	st := &treeStorage{
		id:          id,
		readDb:      readDb,
		writeDb:     writeDb,
		table:       headStorage.SpaceId() + "_changes",
		headStorage: headStorage,
	}
	if err := st.initStatements(); err != nil {
		return nil, err
	}
	rootChange, err := st.Get(context.Background(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, treestorage.ErrUnknownTreeId
		}
		return nil, fmt.Errorf("failed to retrieve root change: %w", err)
	}
	st.root = rootChange

	return st, nil
}

func (t *treeStorage) Id() string {
	return t.id
}

func (t *treeStorage) Root(ctx context.Context) (objecttree.StorageChange, error) {
	return t.root, nil
}

func (t *treeStorage) Heads(ctx context.Context) ([]string, error) {
	entry, err := t.headStorage.GetEntry(ctx, t.id)
	if err != nil {
		return nil, fmt.Errorf("heads: %w", err)
	}
	return entry.Heads, nil
}

func (t *treeStorage) CommonSnapshot(ctx context.Context) (string, error) {
	entry, err := t.headStorage.GetEntry(ctx, t.id)
	if err != nil {
		return "", fmt.Errorf("commonSnapshot: %w", err)
	}
	return entry.CommonSnapshot, nil
}

func (t *treeStorage) Has(ctx context.Context, id string) (bool, error) {
	row := t.stmt.countById.QueryRowContext(ctx, id)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (t *treeStorage) Get(ctx context.Context, id string) (objecttree.StorageChange, error) {
	row := t.stmt.selectById.QueryRowContext(ctx, id)
	var (
		rawChange       []byte
		snapshotId      string
		orderId         string
		changeSize      int
		snapshotCounter int
		prevIdsStr      string
		treeId          string
		added           float64
	)
	if err := row.Scan(
		&id,
		&rawChange,
		&snapshotId,
		&orderId,
		&changeSize,
		&snapshotCounter,
		&prevIdsStr,
		&treeId,
		&added,
	); err != nil {
		return objecttree.StorageChange{}, err
	}
	return objecttree.StorageChange{
		Id:              id,
		RawChange:       rawChange,
		SnapshotId:      snapshotId,
		OrderId:         orderId,
		ChangeSize:      changeSize,
		SnapshotCounter: snapshotCounter,
		PrevIds:         splitComma(prevIdsStr),
		TreeId:          treeId,
	}, nil
}

func (t *treeStorage) GetAfterOrder(ctx context.Context, orderId string, iter objecttree.StorageIterator) error {
	rows, err := t.stmt.selectAfterOrder.QueryContext(ctx, orderId, t.id)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id              string
			rawChange       []byte
			snapshotId      string
			dbOrderId       string
			changeSize      int
			snapshotCounter int
			prevIdsStr      string
			treeId          string
			added           float64
		)
		if err := rows.Scan(
			&id,
			&rawChange,
			&snapshotId,
			&dbOrderId,
			&changeSize,
			&snapshotCounter,
			&prevIdsStr,
			&treeId,
			&added,
		); err != nil {
			return err
		}
		ch := objecttree.StorageChange{
			Id:              id,
			RawChange:       rawChange,
			SnapshotId:      snapshotId,
			OrderId:         dbOrderId,
			ChangeSize:      changeSize,
			SnapshotCounter: snapshotCounter,
			PrevIds:         splitComma(prevIdsStr),
			TreeId:          treeId,
		}
		cont, iErr := iter(ctx, ch)
		if iErr != nil {
			return iErr
		}
		if !cont {
			break
		}
	}
	return rows.Err()
}

func (t *treeStorage) AddAll(ctx context.Context, changes []objecttree.StorageChange, heads []string, commonSnapshot string) error {
	tx, err := t.writeDb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("AddAll: begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmtInsert := tx.StmtContext(ctx, t.stmt.insertChange)
	for _, ch := range changes {
		ch.TreeId = t.id
		if _, execErr := stmtInsert.ExecContext(ctx,
			ch.Id,
			ch.RawChange,
			ch.SnapshotId,
			ch.OrderId,
			ch.ChangeSize,
			ch.SnapshotCounter,
			strings.Join(ch.PrevIds, ","),
			ch.TreeId,
			time.Now().Unix(),
		); execErr != nil {
			err = execErr
			return err
		}
	}

	upd := headstorage.HeadsUpdate{
		Id:             t.id,
		Heads:          heads,
		CommonSnapshot: &commonSnapshot,
	}
	ctx = WithTx(ctx, tx)
	if upErr := t.headStorage.UpdateEntryTx(ctx, upd); upErr != nil {
		err = upErr
		return err
	}

	err = tx.Commit()
	return err
}

func (t *treeStorage) AddAllNoError(ctx context.Context, changes []objecttree.StorageChange, heads []string, commonSnapshot string) error {
	tx, err := t.writeDb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("AddAllNoError: begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmtInsertIgnore := tx.StmtContext(ctx, t.stmt.insertChangeIgnore)
	for _, ch := range changes {
		ch.TreeId = t.id
		if _, execErr := stmtInsertIgnore.ExecContext(ctx,
			ch.Id,
			ch.RawChange,
			ch.SnapshotId,
			ch.OrderId,
			ch.ChangeSize,
			ch.SnapshotCounter,
			strings.Join(ch.PrevIds, ","),
			ch.TreeId,
			time.Now().Unix(),
		); execErr != nil {
			var sqliteErr sqlite3.Error
			if errors.As(execErr, &sqliteErr) &&
				sqliteErr.ExtendedCode == sqlite3.ErrConstraintPrimaryKey {
				// ignore
			} else {
				err = execErr
				return err
			}
		}
	}

	upd := headstorage.HeadsUpdate{
		Id:             t.id,
		Heads:          heads,
		CommonSnapshot: &commonSnapshot,
	}
	ctx = WithTx(ctx, tx)
	if upErr := t.headStorage.UpdateEntryTx(ctx, upd); upErr != nil {
		err = upErr
		return err
	}

	err = tx.Commit()
	return err
}

func (t *treeStorage) Delete(ctx context.Context) error {
	tx, err := t.writeDb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete: begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmtDel := tx.StmtContext(ctx, t.stmt.deleteByTree)
	if _, execErr := stmtDel.ExecContext(ctx, t.id); execErr != nil {
		err = execErr
		return err
	}
	err = tx.Commit()
	return err
}

func (t *treeStorage) Close() error {
	return nil
}

func createChangesTable(db *sql.DB, spaceId string) error {
	const sqlCreate = `
	CREATE TABLE IF NOT EXISTS %s (
		id TEXT PRIMARY KEY,
		raw_change BLOB,
		snapshot_id TEXT,
		order_id TEXT,
		change_size INTEGER,
		snapshot_counter INTEGER,
		prev_ids TEXT,
		tree_id TEXT,
		added REAL
	);
	CREATE INDEX IF NOT EXISTS idx_changes_tree_id ON changes(tree_id);
	CREATE INDEX IF NOT EXISTS idx_changes_order_id ON changes(order_id);
`
	statement := fmt.Sprintf(sqlCreate, spaceId)
	_, err := db.Exec(statement)
	return err
}

func (t *treeStorage) initStatements() error {
	var err error

	insertSQL := `
	INSERT INTO %s (
		id,
		raw_change,
		snapshot_id,
		order_id,
		change_size,
		snapshot_counter,
		prev_ids,
		tree_id,
		added
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
	`
	if t.stmt.insertChange, err = t.writeDb.Prepare(t.createStatement(insertSQL)); err != nil {
		return err
	}

	insertIgnoreSQL := `
	INSERT OR IGNORE INTO %s (
		id,
		raw_change,
		snapshot_id,
		order_id,
		change_size,
		snapshot_counter,
		prev_ids,
		tree_id,
		added
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
	`
	if t.stmt.insertChangeIgnore, err = t.writeDb.Prepare(t.createStatement(insertIgnoreSQL)); err != nil {
		return err
	}

	selectByIdSQL := `
	SELECT
		id,
		raw_change,
		snapshot_id,
		order_id,
		change_size,
		snapshot_counter,
		prev_ids,
		tree_id,
		added
	FROM %s
	WHERE id = ?;
	`
	if t.stmt.selectById, err = t.readDb.Prepare(t.createStatement(selectByIdSQL)); err != nil {
		return err
	}

	// Count by id (for Has).
	countByIdSQL := `SELECT COUNT(*) FROM %s WHERE id = ?;`
	if t.stmt.countById, err = t.readDb.Prepare(t.createStatement(countByIdSQL)); err != nil {
		return err
	}

	selectAfterOrderSQL := `
	SELECT
		id,
		raw_change,
		snapshot_id,
		order_id,
		change_size,
		snapshot_counter,
		prev_ids,
		tree_id,
		added
	FROM %s
	WHERE order_id >= ? AND tree_id = ?
	ORDER BY order_id;
	`
	if t.stmt.selectAfterOrder, err = t.readDb.Prepare(t.createStatement(selectAfterOrderSQL)); err != nil {
		return err
	}

	// Delete all by tree_id
	deleteByTreeSQL := `
	DELETE FROM %s
	WHERE tree_id = ?;
	`
	if t.stmt.deleteByTree, err = t.writeDb.Prepare(t.createStatement(deleteByTreeSQL)); err != nil {
		return err
	}

	return nil
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	return parts
}
