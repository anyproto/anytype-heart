package anyblockjson

// invalidutf8_test.go — a display name that is not valid UTF-8 is not a
// spelling.
//
// The writer maps every invalid byte to U+FFFD; the collision plan compares
// raw Go strings. So two names differing ONLY in their invalid bytes looked
// distinct to the plan, took no suffix, and then rendered as one member
// name — a JSON object holds a member once, so one value replaced the other
// and Validate saw nothing wrong, because by then the collision had already
// happened.
//
// Zero occurrences in the 77-space corpus; the retired normalization
// grammar dropped U+FFFD as a matter of course, so the exposure arrived
// with raw names. Hardening, pinned.

import (
	"encoding/json"
	"testing"
	"unicode/utf8"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestInvalidUTF8IsNotASpelling(t *testing.T) {
	// two names a reader would call different and the writer cannot: the
	// invalid byte is the only thing between them, and it is the byte that
	// does not survive
	const (
		keyA  = "6a7663db61fab21cd4b900a1"
		keyB  = "6a7663db61fab21cd4b900b2"
		nameA = "Region \xff"
		nameB = "Region \xfe"
	)
	require.False(t, utf8.ValidString(nameA))
	require.False(t, utf8.ValidString(nameB))

	t.Run("neither name is a writable key", func(t *testing.T) {
		assert.False(t, isWritablePropertyKey(nameA))
		assert.Contains(t, unwritableKeyReason("property key", nameA), "not valid UTF-8")
	})

	t.Run("both keys are written verbatim and both values survive", func(t *testing.T) {
		// given
		vocab := nameVocab{names: map[string]string{keyA: nameA, keyB: nameB}}
		snap := &model.SmartBlockSnapshotBase{Details: &types.Struct{Fields: map[string]*types.Value{
			"id": pbtypes.String("o1"),
			keyA: pbtypes.String("value of A"),
			keyB: pbtypes.String("value of B"),
		}}}
		opts := Options{Keys: vocab}

		// when
		data, err := Marshal(model.SmartBlockType_Page, snap, opts)
		require.NoError(t, err)

		// then — the stored key is always its own address, so each holder
		// keeps its own member and neither value is lost
		require.NoError(t, Validate(data), "I1:\n%s", data)
		var doc struct {
			Properties map[string]string `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(data, &doc))
		assert.Equal(t, "value of A", doc.Properties[keyA])
		assert.Equal(t, "value of B", doc.Properties[keyB])
		assert.Len(t, doc.Properties, 2, "no member was written twice and silently collapsed")

		// I2's substance, and the fixpoint
		_, back, err := Unmarshal(data, opts)
		require.NoError(t, err)
		assert.Equal(t, "value of A", back.Details.Fields[keyA].GetStringValue())
		assert.Equal(t, "value of B", back.Details.Fields[keyB].GetStringValue())
		again, err := Marshal(model.SmartBlockType_Page, back, opts)
		require.NoError(t, err)
		assert.Equal(t, string(data), string(again))
	})

	t.Run("a legend entry is refused rather than written unreadable", func(t *testing.T) {
		reason, refused := legendEntryRefusal(nameA, keyA, true)
		assert.True(t, refused)
		assert.Contains(t, reason, "not valid UTF-8")
	})
}
