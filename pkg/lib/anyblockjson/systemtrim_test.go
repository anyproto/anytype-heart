package anyblockjson

// systemtrim_test.go — the seven system-stamped keys whose empty value is not
// written (§15 #12), and the keys deliberately kept out of that list.

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func boolValue(b bool) *types.Value {
	return &types.Value{Kind: &types.Value_BoolValue{BoolValue: b}}
}

func trimSnapshot(details map[string]*types.Value) *model.SmartBlockSnapshotBase {
	details["id"] = str("bafyreitrimroot")
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{
			Id:      "bafyreitrimroot",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}},
		Details: fields(details),
	}
}

// Each admitted key, empty, is absent from the document — and the SAME key
// with a value is written, so the rule is about the value and never about
// the key.
//
// How this can fail: drop the buildProperties skip and the empty spellings
// appear; widen isEmptySystemValue and the non-empty ones vanish.
func TestSystemTrim_AnEmptyAdmittedKeyIsNotWritten(t *testing.T) {
	for key, filled := range map[string]*types.Value{
		"relationDefaultValue":  str("unstarted"),
		"relationReadonlyValue": boolValue(true),
		"revision":              num(3),
		"isHidden":              boolValue(true),
		"isHiddenDiscovery":     boolValue(true),
		"isArchived":            boolValue(true),
		"relationMaxCount":      num(1),
	} {
		t.Run(key, func(t *testing.T) {
			// given the same key, once empty and once carrying a value
			empty := map[string]*types.Value{key: emptyLike(filled)}
			set := map[string]*types.Value{key: filled}

			// when
			emptyDoc, err := Marshal(model.SmartBlockType_Page, trimSnapshot(empty), testOptions())
			require.NoError(t, err)
			require.NoError(t, Validate(emptyDoc), "§11 I1")
			setDoc, err := Marshal(model.SmartBlockType_Page, trimSnapshot(set), testOptions())
			require.NoError(t, err)

			// then
			slug := BundledKeyVocabulary{}.PropertySlug(key)
			assert.NotContains(t, string(emptyDoc), `"`+slug+`"`,
				"an empty %s says nothing a reader could act on (§15 #12)", key)
			assert.Contains(t, string(setDoc), `"`+slug+`"`,
				"the rule is about the VALUE, never the key")
		})
	}
}

// emptyLike returns the empty value of the same kind, so each case tests the
// emptiness that key actually carries in the store rather than a uniform nil.
func emptyLike(v *types.Value) *types.Value {
	switch v.GetKind().(type) {
	case *types.Value_BoolValue:
		return boolValue(false)
	case *types.Value_NumberValue:
		return num(0)
	}
	return str("")
}

// The keys that FAILED the admission test keep their empty value. Each is one
// line away from being trimmed, and each would be wrong:
// `relationFormat`'s zero is `longtext`, a real format; the list-valued
// user-intent keys express a CLEARED set by being empty, which GO-7451
// settled for a type's recommended lists. `relationFormat` and
// `relationFormatObjectTypes` have since moved to a relation document's
// envelope (§2d), where the SAME admission verdict holds: the envelope
// fields mirror stored presence, empty values included.
//
// How this can fail: add featuredRelations to trimmedWhenEmpty and the page
// assertion finds the key gone; make buildPropertySettings treat format 0 as
// unset, or omit an empty object_types list, and the relation assertions
// find the fields missing.
func TestSystemTrim_TheExcludedKeysKeepTheirEmptyValue(t *testing.T) {
	// given
	pageSnap := trimSnapshot(map[string]*types.Value{
		"featuredRelations": strList(),
	})
	relSnap := trimSnapshot(map[string]*types.Value{
		"relationFormat":            num(0), // 0 is longtext, not "unset"
		"relationFormatObjectTypes": strList(),
	})

	// when
	pageDoc, err := Marshal(model.SmartBlockType_Page, pageSnap, testOptions())
	require.NoError(t, err)
	relDoc, err := Marshal(model.SmartBlockType_STRelation, relSnap, testOptions())
	require.NoError(t, err)

	// then
	// featuredRelations used to be this test's example of a key deliberately
	// OUTSIDE the whitelist, on the reasoning that an empty list is a cleared
	// set. That reasoning was wrong — no UI sets a per-object featured list,
	// and an empty one is the layout syncer's output — so the key is now
	// deprecated outright and never reaches this rule at all.
	assert.NotContains(t, string(pageDoc), `"featured_properties"`,
		"deprecated: the type owns an object's featured list")
	assert.Contains(t, string(relDoc), `"format": "text"`,
		"relationFormat 0 is longtext, a real format, and §2d requires the field")
	assert.Contains(t, string(relDoc), `"object_types": []`,
		"an empty target set is a CLEARED set; the §2d field mirrors stored presence")
}

// The whitelist is a list, not a category: a system relation NOT on it keeps
// its empty value, which is the whole difference between this and the
// blanket rule over bundle.SystemRelations that was declined.
//
// How this can fail: replace trimmedWhenEmpty with a SystemRelations
// membership test and this key disappears.
func TestSystemTrim_AnUnlistedSystemRelationIsUntouched(t *testing.T) {
	// given `origin` is in bundle.SystemRelations and not on the whitelist
	snap := trimSnapshot(map[string]*types.Value{"origin": num(0)})

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, testOptions())
	require.NoError(t, err)

	// then
	assert.Contains(t, string(data), `"origin"`,
		"admission is by explicit entry, never by category")
}
