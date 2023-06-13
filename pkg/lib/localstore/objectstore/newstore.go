package objectstore

import (
	"fmt"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/ristretto"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/huandu/skiplist"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type newstore struct {
	cache    *ristretto.Cache
	db       *badger.DB
	onUpdate func(*types.Struct)
}

func newNewstore(path string) (*newstore, error) {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 10_000_000,
		MaxCost:     100_000_000,
		BufferItems: 64,
	})
	if err != nil {
		return nil, fmt.Errorf("create cache: %w", err)
	}

	db, err := badger.Open(badger.DefaultOptions(path))
	if err != nil {
		return nil, fmt.Errorf("open badgerdb: %w", err)
	}

	return &newstore{cache: cache, db: db}, nil
}

func (s *newstore) GetDetails(id string) (*types.Struct, error) {
	key := []byte(id)
	var details *types.Struct
	err := s.db.View(func(txn *badger.Txn) error {
		it, err := txn.Get(key)
		if err != nil {
			return fmt.Errorf("get item: %w", err)
		}
		details, err = s.extractDetails(it)
		return err
	})
	return details, err
}

func (s *newstore) UpdateDetails(id string, details *types.Struct) error {
	key := []byte(id)
	return s.db.Update(func(txn *badger.Txn) error {
		prev, ok := s.cache.Get(key)
		if !ok {
			it, err := txn.Get(key)
			if err != nil && err != badger.ErrKeyNotFound {
				return fmt.Errorf("get item: %w", err)
			}
			if err != badger.ErrKeyNotFound {
				prev, err = s.extractDetailsFromItem(it)
				if err != nil {
					return fmt.Errorf("extract details: %w", err)
				}
			}
		}

		if prev != nil && proto.Equal(prev.(*types.Struct), details) {
			return nil
		}
		if s.onUpdate != nil {
			s.onUpdate(details)
		}

		s.cache.Set(key, details, int64(details.Size()))
		val, err := proto.Marshal(details)
		if err != nil {
			return fmt.Errorf("marshal details: %w", err)
		}
		return txn.Set(key, val)
	})
}

func (s *newstore) DeleteDetails(id string) error {
	key := []byte(id)
	return s.db.Update(func(txn *badger.Txn) error {
		s.cache.Del(key)
		return txn.Delete(key)
	})
}

func (s *newstore) Query(sch schema.Schema, q database.Query) ([]database.Record, error) {
	filters, err := database.NewFilters(q, sch, nil)
	if err != nil {
		return nil, fmt.Errorf("create filters: %w", err)
	}
	return s.QueryRaw(filters)
}

func (s *newstore) QueryRaw(filters *database.Filters) ([]database.Record, error) {
	skl := skiplist.New(order{filters.Order})

	err := s.db.View(func(txn *badger.Txn) error {
		iterator := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iterator.Close()

		for iterator.Rewind(); iterator.Valid(); iterator.Next() {
			it := iterator.Item()
			details, err := s.extractDetails(it)
			if err != nil {
				return err
			}

			rec := database.Record{Details: details}
			if filters.FilterObj != nil && filters.FilterObj.FilterObject(rec) {
				skl.Set(rec, nil)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	records := make([]database.Record, 0, skl.Len())
	for it := skl.Front(); it != nil; it = it.Next() {
		records = append(records, it.Key().(database.Record))
	}

	return records, nil
}

func (s *newstore) QueryById(ids []string) (records []database.Record, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		iterator := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iterator.Close()

		for iterator.Rewind(); iterator.Valid(); iterator.Next() {
			it := iterator.Item()
			details, err := s.extractDetails(it)
			if err != nil {
				return err
			}

			if lo.Contains(ids, pbtypes.GetString(details, "id")) {
				rec := database.Record{Details: details}
				records = append(records, rec)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (s *newstore) extractDetails(it *badger.Item) (*types.Struct, error) {
	key := it.Key()
	if v, ok := s.cache.Get(key); ok {
		return v.(*types.Struct), nil
	} else {
		return s.extractDetailsFromItem(it)
	}
}

func (s *newstore) extractDetailsFromItem(it *badger.Item) (*types.Struct, error) {
	details := &types.Struct{}
	verr := it.Value(func(val []byte) error {
		uerr := proto.Unmarshal(val, details)
		if uerr != nil {
			return uerr
		}
		s.cache.Set(it.Key(), details, int64(details.Size()))
		return nil
	})
	if verr != nil {
		return nil, fmt.Errorf("get iterator value: %w", verr)
	}
	return details, nil
}

type order struct {
	filter.Order
}

func (o order) Compare(lhs, rhs interface{}) (comp int) {
	le := lhs.(database.Record)
	re := rhs.(database.Record)

	if o.Order != nil {
		comp = o.Order.Compare(le, re)
	}
	// when order isn't set or equal - sort by id
	if comp == 0 {
		if pbtypes.GetString(le.Details, "id") > pbtypes.GetString(re.Details, "id") {
			return 1
		} else {
			return -1
		}
	}
	return comp
}

func (o order) CalcScore(key interface{}) float64 {
	return 0
}
