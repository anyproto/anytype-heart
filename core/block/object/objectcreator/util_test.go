package objectcreator

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// injectApiObjectKey mints the key every type, property and option created
// outside the api is addressed by. The fixtures are the shapes measured in a
// 38,123-object account, where 27 of 1,530 stored api keys sat outside the key
// grammar `^[a-zA-Z0-9_]+$` — all but four of them on options, whose key comes
// from the name and nothing else.
func TestInjectApiObjectKey(t *testing.T) {
	t.Run("derived from a name", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			want string
		}{
			{name: "Manual property", want: "manual_property"},
			// the three that used to store an unaddressable key
			{name: "Lists [in work]", want: "lists_in_work"},
			{name: "Manual export & import", want: "manual_export_import"},
			{name: "➡️ Medium", want: "medium"},
			// transliteration still carries the word across scripts
			{name: "Задача", want: "zadacha"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				// given
				object := domain.NewDetails()
				object.SetString(bundle.RelationKeyName, tc.name)

				// when
				injectApiObjectKey(object, "")

				// then
				assert.Equal(t, tc.want, object.GetString(bundle.RelationKeyApiObjectKey))
			})
		}
	})

	t.Run("derived from a name longer than a key may be", func(t *testing.T) {
		// given
		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyName, strings.Repeat("a", 300))

		// when
		injectApiObjectKey(object, "")

		// then
		assert.Len(t, object.GetString(bundle.RelationKeyApiObjectKey), bundle.MaxApiSlugLen)
	})

	t.Run("a name with nothing the grammar admits stores no key", func(t *testing.T) {
		// an object with no derivable slug is addressed by its internal key,
		// which is a real address; an empty apiObjectKey would not be
		for _, name := range []string{"➡️", "☕", "   "} {
			t.Run(name, func(t *testing.T) {
				// given
				object := domain.NewDetails()
				object.SetString(bundle.RelationKeyName, name)

				// when
				injectApiObjectKey(object, "")

				// then
				assert.False(t, object.Has(bundle.RelationKeyApiObjectKey))
			})
		}
	})

	t.Run("an internal key outranks the name and is not transliterated", func(t *testing.T) {
		// given
		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyName, "Some Name")

		// when
		injectApiObjectKey(object, "dueDate")

		// then
		assert.Equal(t, "due_date", object.GetString(bundle.RelationKeyApiObjectKey))
	})

	t.Run("an internal key still loses what the grammar refuses", func(t *testing.T) {
		// given
		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyName, "Some Name")

		// when
		injectApiObjectKey(object, "my key!")

		// then
		assert.Equal(t, "my_key", object.GetString(bundle.RelationKeyApiObjectKey))
	})

	t.Run("a key already on the object is never re-minted", func(t *testing.T) {
		// the user-provided apiObjectKey is the top of the priority list, and
		// re-minting one would silently respell an address already in use
		// somewhere
		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyName, "Some Name")
		object.SetString(bundle.RelationKeyApiObjectKey, "chosen_key")

		// when
		injectApiObjectKey(object, "dueDate")

		// then
		assert.Equal(t, "chosen_key", object.GetString(bundle.RelationKeyApiObjectKey))
	})
}
