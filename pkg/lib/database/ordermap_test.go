package database

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestOrderMap_BuildOrder(t *testing.T) {
	buf := make([]byte, 0)

	t.Run("build order for options", func(t *testing.T) {
		om := &OrderMap{
			sortKeys: []domain.RelationKey{bundle.RelationKeyOrderId, bundle.RelationKeyName},
			data: map[string]*domain.Details{
				"tag1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyOrderId: domain.String("VaVa"),
					bundle.RelationKeyName:    domain.String("Earth"),
				}),
				"tag2": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyOrderId: domain.String("vZvZ"),
					bundle.RelationKeyName:    domain.String("Mars"),
				}),
				"tag3": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyOrderId: domain.String(""),
					bundle.RelationKeyName:    domain.String("Venus"),
				}),
			},
		}

		buf = om.BuildOrder(buf, "tag1", "tag2")
		assert.Equal(t, "VaVa"+"vZvZ"+"Earth"+"Mars", string(buf))
		buf = om.BuildOrder(buf, "")
		assert.Equal(t, "", string(buf))
		buf = om.BuildOrder(buf, "tag3")
		assert.Equal(t, "Venus", string(buf))
	})
}

func BenchmarkOrderMap_BuildOrder(b *testing.B) {
	var (
		key  = domain.RelationKey("key")
		data = make(map[string]*domain.Details, 100)
		ids  = make([]string, 100)
	)

	for i := 0; i < 100; i++ {
		ids[i] = fmt.Sprintf("id%d", i)
		data[ids[i]] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			key: domain.String(fmt.Sprintf("%d", rand.Int63())),
		})
	}

	var (
		buf  = make([]byte, 0)
		rng  = rand.New(rand.NewSource(132211))
		swap = func(i, j int) { ids[i], ids[j] = ids[j], ids[i] }
		om   = &OrderMap{data: data, sortKeys: []domain.RelationKey{key}}
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rng.Shuffle(100, swap)
		buf = om.BuildOrder(buf, ids...)
	}
}

func TestOrderMap_Update(t *testing.T) {
	coll := collate.New(language.Und, collate.IgnoreCase)
	collBuf := &collate.Buffer{}

	t.Run("nil OrderMap", func(t *testing.T) {
		var om *OrderMap
		details := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeyName:    domain.String("Updated Name"),
				bundle.RelationKeyOrderId: domain.String("DDDD"),
			}),
		}
		updated := om.Update(details)
		assert.False(t, updated)
	})

	t.Run("empty data", func(t *testing.T) {
		om := &OrderMap{data: make(map[string]*domain.Details)}
		details := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeyName:    domain.String("Updated Name"),
				bundle.RelationKeyOrderId: domain.String("DDDD"),
			}),
		}
		updated := om.Update(details)
		assert.False(t, updated)
	})

	t.Run("update existing object with new orderId", func(t *testing.T) {
		collatedName := coll.KeyFromString(collBuf, "Original Name")
		collBuf.Reset()
		original := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String(string(collatedName)),
			bundle.RelationKeyOrderId: domain.String("BBBB"),
		})
		om := &OrderMap{
			collator:       coll,
			collatorBuffer: collBuf,
			data: map[string]*domain.Details{
				"id1": original,
			},
		}

		details := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeyName:    domain.String("Original Name"),
				bundle.RelationKeyOrderId: domain.String("CCCC"),
			}),
		}

		updated := om.Update(details)
		assert.True(t, updated)
		assert.Equal(t, "CCCC", original.GetString(bundle.RelationKeyOrderId))
	})

	t.Run("update existing object with new name", func(t *testing.T) {
		collatedName := coll.KeyFromString(collBuf, "Updated Name")
		collBuf.Reset()
		original := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String("Original Name"),
			bundle.RelationKeyOrderId: domain.String("BBBB"),
		})
		om := &OrderMap{
			collator:       coll,
			collatorBuffer: collBuf,
			data: map[string]*domain.Details{
				"id1": original,
			},
		}

		details := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeyName:    domain.String("Updated Name"),
				bundle.RelationKeyOrderId: domain.String("BBBB"),
			}),
		}

		updated := om.Update(details)
		assert.True(t, updated)
		assert.Equal(t, string(collatedName), original.GetString(bundle.RelationKeyName))
	})

	t.Run("update existing object with no changes", func(t *testing.T) {
		collatedName := coll.KeyFromString(collBuf, "Same Name")
		collBuf.Reset()
		original := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String(string(collatedName)),
			bundle.RelationKeyOrderId: domain.String("BBBB"),
		})
		om := &OrderMap{
			collator:       coll,
			collatorBuffer: collBuf,
			data: map[string]*domain.Details{
				"id1": original,
			},
		}

		details := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeyName:    domain.String("Same Name"),
				bundle.RelationKeyOrderId: domain.String("BBBB"),
			}),
		}

		updated := om.Update(details)
		assert.False(t, updated)
	})

	t.Run("update non-existing object", func(t *testing.T) {
		om := &OrderMap{
			collator:       coll,
			collatorBuffer: collBuf,
			data: map[string]*domain.Details{
				"id1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyName: domain.String("Existing"),
				}),
			},
		}

		details := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:   domain.String("id2"),
				bundle.RelationKeyName: domain.String("Non-existing"),
			}),
		}

		updated := om.Update(details)
		assert.False(t, updated)
		assert.Len(t, om.data, 1) // Should still have only the original object
	})

	t.Run("update multiple objects", func(t *testing.T) {
		collatedName1 := string(coll.KeyFromString(collBuf, "Updated Name1"))
		collBuf.Reset()
		collatedName2 := coll.KeyFromString(collBuf, "Name2")
		collBuf.Reset()
		obj1 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String("Name1"),
			bundle.RelationKeyOrderId: domain.String("BBBB"),
		})
		obj2 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String(string(collatedName2)),
			bundle.RelationKeyOrderId: domain.String("CCCC"),
		})

		om := &OrderMap{
			collator:       coll,
			collatorBuffer: collBuf,
			data: map[string]*domain.Details{
				"id1": obj1,
				"id2": obj2,
			},
		}

		details := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeyName:    domain.String("Updated Name1"),
				bundle.RelationKeyOrderId: domain.String("BBBB"),
			}),
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("id2"),
				bundle.RelationKeyName:    domain.String("Name2"),
				bundle.RelationKeyOrderId: domain.String("DDDD"),
			}),
		}

		updated := om.Update(details)
		assert.True(t, updated)
		assert.Equal(t, collatedName1, obj1.GetString(bundle.RelationKeyName))
		assert.Equal(t, "DDDD", obj2.GetString(bundle.RelationKeyOrderId))
	})
}

func TestOrderMap_SetOrders(t *testing.T) {
	coll := collate.New(language.Und, collate.IgnoreCase)
	collBuf := &collate.Buffer{}

	t.Run("nil store", func(t *testing.T) {
		om := &OrderMap{data: make(map[string]*domain.Details), store: &stubSpaceObjectStore{}}
		om.setOrders("id1", "id2")
		assert.Empty(t, om.data)
	})

	t.Run("nil data initialized", func(t *testing.T) {
		store := &stubSpaceObjectStore{
			queryRawResult: []Record{
				{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyId:      domain.String("id1"),
						bundle.RelationKeyName:    domain.String("Tag A"),
						bundle.RelationKeyOrderId: domain.String("BBBB"),
					}),
				},
			},
		}

		om := &OrderMap{store: store, collator: coll, collatorBuffer: collBuf}
		om.setOrders("id1")

		collatedName := string(coll.KeyFromString(collBuf, "Tag A"))
		collBuf.Reset()

		assert.Len(t, om.data, 1)
		assert.Contains(t, om.data, "id1")
		assert.Equal(t, collatedName, om.data["id1"].GetString(bundle.RelationKeyName))
		assert.Equal(t, "BBBB", om.data["id1"].GetString(bundle.RelationKeyOrderId))
	})

	t.Run("some ids already exist", func(t *testing.T) {
		existing := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("Existing"),
		})

		store := &stubSpaceObjectStore{
			queryRawResult: []Record{
				{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyId:      domain.String("id2"),
						bundle.RelationKeyName:    domain.String("New Tag"),
						bundle.RelationKeyOrderId: domain.String("CCCC"),
					}),
				},
			},
		}

		om := &OrderMap{
			store:          store,
			collator:       coll,
			collatorBuffer: collBuf,
			data: map[string]*domain.Details{
				"id1": existing,
			},
		}

		om.setOrders("id1", "id2") // id1 exists, id2 is new

		collatedName := string(coll.KeyFromString(collBuf, "New Tag"))
		collBuf.Reset()

		assert.Len(t, om.data, 2)
		assert.Equal(t, existing, om.data["id1"]) // Should be unchanged
		assert.Equal(t, collatedName, om.data["id2"].GetString(bundle.RelationKeyName))
	})

	t.Run("all ids already exist", func(t *testing.T) {
		store := &stubSpaceObjectStore{}
		om := &OrderMap{
			data: map[string]*domain.Details{
				"id1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyName: domain.String("Existing1"),
				}),
				"id2": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyName: domain.String("Existing2"),
				}),
			},
			store:          store,
			collator:       coll,
			collatorBuffer: collBuf,
		}
		// Should not call Query since all ids exist

		om.setOrders("id1", "id2")

		assert.Len(t, om.data, 2)
	})
}

func TestBuildOrderMap(t *testing.T) {
	t.Run("build order map with options", func(t *testing.T) {
		// given
		store := &stubSpaceObjectStore{
			options: []*model.RelationOption{
				{
					Id:      "opt1",
					Text:    "Option 1",
					OrderId: "BBBB",
				},
				{
					Id:      "opt2",
					Text:    "Option 2",
					OrderId: "CCCC",
				},
				{
					Id:   "opt3",
					Text: "Option 3",
					// No OrderId
				},
			},
		}

		// when
		om := BuildOrderMap(store, "status", model.RelationFormat_status, &collate.Buffer{})

		// then
		require.NotNil(t, om)
		assert.NotNil(t, om.store)
		assert.Equal(t, []domain.RelationKey{bundle.RelationKeyOrderId, bundle.RelationKeyName}, om.sortKeys)
		assert.Len(t, om.data, 3)
		assert.NotEmpty(t, om.data["opt1"].GetString(bundle.RelationKeyName))
		assert.Equal(t, "BBBB", om.data["opt1"].GetString(bundle.RelationKeyOrderId))
		assert.NotEmpty(t, om.data["opt2"].GetString(bundle.RelationKeyName))
		assert.Equal(t, "CCCC", om.data["opt2"].GetString(bundle.RelationKeyOrderId))
		assert.NotEmpty(t, om.data["opt3"].GetString(bundle.RelationKeyName))
		assert.Equal(t, "", om.data["opt3"].GetString(bundle.RelationKeyOrderId))
	})

	t.Run("build order map with objects", func(t *testing.T) {
		// given
		key := bundle.RelationKeyLinks
		objectHandlers := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				key: domain.StringList([]string{"1", "2"}),
			}),
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				key: domain.StringList([]string{"2", "3"}),
			}),
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				key: domain.StringList([]string{"1"}),
			}),
		}

		orderHandlers := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:   domain.String("1"),
				bundle.RelationKeyName: domain.String("1"),
			}),
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("2"),
				bundle.RelationKeyName:    domain.String(""),
				bundle.RelationKeySnippet: domain.String("<ERROR>"),
			}),
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:             domain.String("3"),
				bundle.RelationKeySnippet:        domain.String("3"),
				bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_note),
			}),
		}

		store := &stubSpaceObjectStore{
			iterate: func(q Query, proc func(record *domain.Details)) error {
				if q.Filters[0].RelationKey == bundle.RelationKeyId {
					for _, details := range orderHandlers {
						proc(details)
					}
					return nil
				}

				for _, details := range objectHandlers {
					proc(details)
				}

				return nil
			},
		}

		// when
		om := BuildOrderMap(store, key, model.RelationFormat_object, &collate.Buffer{})

		// then
		require.NotNil(t, om)
		assert.NotNil(t, om.store)
		assert.Equal(t, []domain.RelationKey{bundle.RelationKeyName}, om.sortKeys)
		assert.Len(t, om.data, 3)
		first := om.data["1"].GetString(bundle.RelationKeyName)
		second := om.data["2"].GetString(bundle.RelationKeyName)
		third := om.data["3"].GetString(bundle.RelationKeyName)
		assert.Less(t, second, first)
		assert.Less(t, first, third)
	})
}
