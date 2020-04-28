package localstore

import (
	"encoding/binary"
	"fmt"
	"sync"

	ds "github.com/ipfs/go-datastore"
)

var (
	// FileInfo is stored in db key pattern:
	// /migrations/<hash>
	migrationsPrefix                   = "migrations"
	migrationsPrefixKey                = ds.NewKey("/" + migrationsPrefix)
	_                   MigrationStore = (*dsMigrationStore)(nil)
)

type dsMigrationStore struct {
	ds ds.TxnDatastore
	l  sync.Mutex
}

func NewMigrationStore(ds ds.TxnDatastore) MigrationStore {
	return &dsMigrationStore{
		ds: ds,
	}
}

func (m *dsMigrationStore) Prefix() string {
	return "migration"
}

func (m *dsMigrationStore) Indexes() []Index {
	return []Index{}
}

func (m *dsMigrationStore) SaveVersion(version int) error {
	if version < 0 {
		return fmt.Errorf("version must be >=0")
	}

	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	var b = make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(version))

	err = m.ds.Put(migrationsPrefixKey.ChildString("version"), b)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func (m *dsMigrationStore) GetVersion() (int, error) {
	m.l.Lock()
	defer m.l.Unlock()

	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return 0, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	b, err := m.ds.Get(migrationsPrefixKey.ChildString("version"))
	if err != nil && err != ds.ErrNotFound {
		return 0, err
	}

	if b == nil {
		return 0, nil
	}

	version := binary.LittleEndian.Uint32(b)

	return int(version), nil
}
