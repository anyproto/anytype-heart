package objectstore

import (
	"fmt"
	"path"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/ristretto"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/huandu/skiplist"
	ds "github.com/ipfs/go-datastore"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
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

func (s *dsObjectStore) UpdateObjectDetails(id string, details *types.Struct) error {
	if details == nil {
		return nil
	}
	if details.Fields == nil {
		return fmt.Errorf("details fields are nil")
	}

	key := pagesDetailsBase.ChildString(id).Bytes()
	return s.db.Update(func(txn *badger.Txn) error {
		prev, ok := s.cache.Get(key)
		if !ok {
			it, err := txn.Get(key)
			if err != nil && err != badger.ErrKeyNotFound {
				return fmt.Errorf("get item: %w", err)
			}
			if err != badger.ErrKeyNotFound {
				prev, err = s.unmarshalDetailsFromItem(it)
				if err != nil {
					return fmt.Errorf("extract details: %w", err)
				}
			}
		}
		if prev != nil && proto.Equal(prev.(*types.Struct), details) {
			return ErrDetailsNotChanged
		}
		// Ensure ID is set
		details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
		s.sendUpdatesToSubscriptions(id, details)

		s.cache.Set(key, details, int64(details.Size()))
		val, err := proto.Marshal(details)
		if err != nil {
			return fmt.Errorf("marshal details: %w", err)
		}
		return txn.Set(key, val)
	})
}

func (s *dsObjectStore) DeleteDetails(id string) error {
	key := pagesDetailsBase.ChildString(id).Bytes()
	return s.db.Update(func(txn *badger.Txn) error {
		s.cache.Del(key)

		for _, k := range []ds.Key{
			pagesSnippetBase.ChildString(id),
			pagesDetailsBase.ChildString(id),
		} {
			if err := txn.Delete(k.Bytes()); err != nil {
				return fmt.Errorf("delete key %s: %w", k, err)
			}
		}

		return txn.Delete(key)
	})
}

func (s *dsObjectStore) Query(sch schema.Schema, q database.Query) ([]database.Record, int, error) {
	filters, err := s.buildQuery(sch, q)
	if err != nil {
		return nil, 0, fmt.Errorf("build query: %w", err)
	}
	recs, err := s.QueryRaw(filters, q.Limit, q.Offset)
	return recs, 0, err
}

func (s *dsObjectStore) QueryRaw(filters *database.Filters, limit int, offset int) ([]database.Record, error) {
	if filters == nil || filters.FilterObj == nil {
		return nil, fmt.Errorf("filter cannot be nil or unitialized")
	}
	skl := skiplist.New(order{filters.Order})

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = pagesDetailsBase.Bytes()
		iterator := txn.NewIterator(opts)
		defer iterator.Close()

		for iterator.Rewind(); iterator.Valid(); iterator.Next() {
			it := iterator.Item()
			details, err := s.extractDetailsFromItem(it)
			if err != nil {
				return err
			}

			rec := database.Record{Details: details}
			if filters.FilterObj != nil && filters.FilterObj.FilterObject(rec) {
				if offset > 0 {
					offset--
					continue
				}
				if limit > 0 && skl.Len() >= limit {
					break
				}
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

func (s *dsObjectStore) QueryById(ids []string) (records []database.Record, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		iterator := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iterator.Close()

		for iterator.Rewind(); iterator.Valid(); iterator.Next() {
			it := iterator.Item()
			details, err := s.extractDetailsFromItem(it)
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

func (s *dsObjectStore) extractDetailsFromItem(it *badger.Item) (*types.Struct, error) {
	key := it.Key()
	if v, ok := s.cache.Get(key); ok {
		return v.(*types.Struct), nil
	} else {
		return s.unmarshalDetailsFromItem(it)
	}
}

func (s *dsObjectStore) unmarshalDetailsFromItem(it *badger.Item) (*types.Struct, error) {
	var details *types.Struct
	err := it.Value(func(val []byte) error {
		var err error
		details, err = unmarshalDetails(detailsKeyToID(it.Key()), val)
		if err != nil {
			return fmt.Errorf("unmarshal details: %w", err)
		}
		s.cache.Set(it.Key(), details, int64(details.Size()))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get item value: %w", err)
	}
	return details, nil
}

func detailsKeyToID(key []byte) string {
	return path.Base(string(key))
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
