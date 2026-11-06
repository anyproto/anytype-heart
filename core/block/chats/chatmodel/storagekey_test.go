package chatmodel

import (
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"github.com/anyproto/any-store/v2/anyenc"
	"github.com/anyproto/any-store/v2/query"
	"github.com/anyproto/lexid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cid returns a message id shaped like a change CID (59 characters), unique for i and differing
// from the other ones only near the end, so the tests see the tail that the key keeps.
func cid(i int) string {
	return fmt.Sprintf("bafyreih%039d%012d", 0, i)
}

func TestStorageKey(t *testing.T) {
	t.Run("order id, separator and the last 5 characters of the id", func(t *testing.T) {
		// given
		id := "bafyreihdb7g4mnw3ebhnm6wl2ddtwy4ejbryyhlrrwa4dxxkemhr3xzfqa"
		want := "!!$5\x01xzfqa"

		// when
		got := StorageKey("!!$5", id)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("an id shorter than the tail is kept whole", func(t *testing.T) {
		assert.Equal(t, "!!$5\x01abc", StorageKey("!!$5", "abc"))
	})

	t.Run("a stub without an order id has an empty order part", func(t *testing.T) {
		assert.Equal(t, "\x01xzfqa", StorageKey("", "bafyreihdb7g4mnw3ebhnm6wl2ddtwy4ejbryyhlrrwa4dxxkemhr3xzfqa"))
	})

	t.Run("the separators sort below every order-id character", func(t *testing.T) {
		// order ids are lexids over CharsAllNoEscape: any-sync's tree order and storestate.LexId
		for _, c := range []byte(lexid.CharsAllNoEscape) {
			assert.Less(t, storageKeySeparator, string(c))
			assert.Less(t, storageKeyAboveSeparator, string(c))
		}
		assert.Equal(t, storageKeySeparator[0]+1, storageKeyAboveSeparator[0])
	})
}

// keyedMessage is a message as the store keys it.
type keyedMessage struct {
	orderId, id string
}

func (m keyedMessage) key() string {
	return StorageKey(m.orderId, m.id)
}

// trickyMessages holds order ids where one is a prefix of another, duplicate order ids (changes
// made concurrently on one parent) and a legacy stub without an order id.
var trickyMessages = []keyedMessage{
	{orderId: "!!$5!", id: cid(1)},
	{orderId: "!!$5", id: cid(2)},
	{orderId: "!!$6", id: cid(4)},
	{orderId: "!!$5", id: cid(3)}, // the same order id as cid(2)
	{orderId: "!!$", id: cid(5)},  // a prefix of "!!$5"
	{orderId: "!!$5!!", id: cid(6)},
	{orderId: "", id: cid(7)}, // a legacy stub
	{orderId: "!!$6", id: cid(8)},
	{orderId: "~", id: cid(9)},
}

func TestStorageKeyOrder(t *testing.T) {
	// given
	keys := make([]string, 0, len(trickyMessages))
	for _, m := range trickyMessages {
		keys = append(keys, m.key())
	}
	want := slices.Clone(trickyMessages)
	slices.SortFunc(want, func(a, b keyedMessage) int {
		if c := strings.Compare(a.orderId, b.orderId); c != 0 {
			return c
		}
		return strings.Compare(a.id, b.id)
	})

	// when
	slices.Sort(keys)

	// then
	wantKeys := make([]string, 0, len(want))
	for _, m := range want {
		wantKeys = append(wantKeys, m.key())
	}
	assert.Equal(t, wantKeys, keys, "keys order as (order id, id tail), a prefix order id first")
	assert.Equal(t, StorageKey("", cid(7)), keys[0], "the stub sorts first")
	assert.Len(t, uniq(keys), len(keys), "duplicate order ids keep distinct keys")
}

func uniq(keys []string) map[string]struct{} {
	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		set[k] = struct{}{}
	}
	return set
}

// TestOrderIdFilters checks the four order-id comparisons expressed on the storage key against the
// comparisons on the order id itself, for every message and every bound: bounds equal to a stored
// order id, a prefix of one, one a stored order id is a prefix of, absent ones and the empty one.
func TestOrderIdFilters(t *testing.T) {
	bounds := []string{"", "!", "!!$", "!!$4", "!!$5", "!!$5!", "!!$5!!", "!!$5!!!", "!!$6", "!!$7", "~", "~~"}
	filters := []struct {
		name   string
		filter func(orderId string) query.Filter
		want   func(orderId, bound string) bool
	}{
		{"OrderIdAtLeast", OrderIdAtLeast, func(o, x string) bool { return o >= x }},
		{"OrderIdAfter", OrderIdAfter, func(o, x string) bool { return o > x }},
		{"OrderIdBefore", OrderIdBefore, func(o, x string) bool { return o < x }},
		{"OrderIdAtMost", OrderIdAtMost, func(o, x string) bool { return o <= x }},
	}
	arena := &anyenc.Arena{}
	for _, f := range filters {
		t.Run(f.name, func(t *testing.T) {
			for _, bound := range bounds {
				filter := f.filter(bound)
				for _, m := range trickyMessages {
					// given
					arena.Reset()
					doc := arena.NewObject()
					doc.Set(StorageKeyField, arena.NewString(m.key()))

					// when
					got := filter.Ok(doc, nil)

					// then
					assert.Equal(t, f.want(m.orderId, bound), got, "order id %q, bound %q", m.orderId, bound)
				}
			}
		})
	}
}

// TestStorageKeyInsertPattern counts, for a synthetic message history, the inserts that do not
// land at the right edge of the collection (a key below the largest one stored so far), keyed by
// the message id as before and by the storage key now. A btree insert at the right edge appends to
// the last leaf; any other one touches a leaf somewhere inside.
func TestStorageKeyInsertPattern(t *testing.T) {
	// given: 2000 creates in order-id order, as a full replay applies them; every 50th is made
	// concurrently with the previous one and shares its order id
	const n = 2000
	rnd := rand.New(rand.NewSource(1))
	orders := lexid.Must(lexid.CharsAllNoEscape, 4, 100)
	var (
		messages  = make([]keyedMessage, 0, n)
		lastOrder string
	)
	for i := range n {
		if i%50 != 49 {
			lastOrder = orders.Next(lastOrder)
		}
		// change ids are hashes: random with respect to the order
		messages = append(messages, keyedMessage{orderId: lastOrder, id: fmt.Sprintf("bafyrei%052x", rnd.Uint64())})
	}
	nonAppend := func(key func(m keyedMessage) string) int {
		var count int
		var maxKey string
		for _, m := range messages {
			k := key(m)
			if k < maxKey {
				count++
				continue
			}
			maxKey = k
		}
		return count
	}

	// when
	byId := nonAppend(func(m keyedMessage) string { return m.id })
	byStorageKey := nonAppend(keyedMessage.key)

	// then
	t.Logf("non-append inserts of %d: keyed by id %d, by storage key %d", n, byId, byStorageKey)
	assert.Greater(t, byId, n*9/10, "a random key lands inside the tree almost every time")
	// the only misses are a concurrent message whose id tail sorts below its sibling's
	require.LessOrEqual(t, byStorageKey, n/50)
}
