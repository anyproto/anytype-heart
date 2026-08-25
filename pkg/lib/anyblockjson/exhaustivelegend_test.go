package anyblockjson

// exhaustivelegend_test.go — the legend names every spelling the bundled
// table cannot speak for, including the ones written verbatim.
//
// The old rule asked the bundled table to INVERT the term. A table that does
// not know a term answers the term itself (chain step 4), so every custom key
// spelled verbatim "inverted" trivially and owed nothing — and the document
// said nothing at all about the one population no reader can resolve without
// it. The key is unambiguous the day it is written; the loss happens later,
// when the relation is deleted and the freed spelling becomes some other
// relation's api key. Every document already written then re-points, offline,
// with nothing in it to say otherwise. That is the corpse-AFTER-export hole,
// and only the writer's own document can close it, because the writer had
// nothing to warn about at the time.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The hole, end to end. The writer has NO vocabulary that binds `initiative`
// — nothing to warn about, nothing to record under the old rule — and the
// reader is a space where the relation has since been deleted and its
// spelling reassigned.
func TestExport_AVerbatimCustomKeyNamesItself(t *testing.T) {
	// given — an object holding a space-local relation keyed `initiative`,
	// exported by a package-only writer
	snap := customKeySnapshot(map[string]*types.Value{
		"initiative": str("value of the relation that was deleted later"),
		"dueDate":    str("2026-07-06T08:44:05Z"),
	})

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{})

	// then — the entry is there, and only for the term the bundled table
	// cannot speak for: `dueDate` is spelled `due_date`, which every reader's
	// table binds, so it owes nothing
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	doc := decodeDoc(t, data)
	assert.Equal(t, map[string]string{"initiative": "initiative"}, doc.PropertyKeys)

	// and the reader that binds the spelling elsewhere is overruled by the
	// document. corpseVocabulary is fully conforming — the writer could not
	// have been warned, and was not running it
	_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g"), Keys: corpseVocabulary{}})
	require.NoError(t, err)
	assert.Equal(t, "value of the relation that was deleted later",
		back.Details.Fields["initiative"].GetStringValue(),
		"without the entry this value lands on "+corpsePropKey+", silently")
	assert.NotContains(t, back.Details.Fields, corpsePropKey)
	assert.Contains(t, back.Details.Fields, "dueDate",
		"a bundled key still needs no entry: `due_date` is bound by every reader's table")
}

// The type namespace, same hole, with the loss that costs more: an object's
// TYPE. Nothing in a package-only export knows `initiative` will be someone
// else's slug tomorrow.
func TestExport_AVerbatimCustomTypeKeyNamesItself(t *testing.T) {
	// given
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{Id: "o1",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		Details:     fields(map[string]*types.Value{"id": str("o1")}),
		ObjectTypes: []string{"ot-initiative"},
	}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{})

	// then
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	var doc struct {
		Type     string            `json:"type"`
		TypeKeys map[string]string `json:"type_internal_keys"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "initiative", doc.Type)
	assert.Equal(t, map[string]string{"initiative": "initiative"}, doc.TypeKeys)

	_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g"), Keys: corpseVocabulary{}})
	require.NoError(t, err)
	assert.Equal(t, []string{"ot-initiative"}, back.ObjectTypes,
		"without the entry the object comes back typed as ot-"+customTypeKey)
}

// The exact boundary of "exhaustive": a BUNDLED key spelled as its bundled
// slug still owes nothing, because the table that binds it ships with every
// reader. Without this the rule would be "emit everything", the legend would
// double the size of an ordinary document, and nothing would be bought.
func TestExport_ABundledSpellingStillOwesNothing(t *testing.T) {
	snap := customKeySnapshot(map[string]*types.Value{
		"dueDate":    str("2026-07-06T08:44:05Z"),
		"pluralName": str("A"),
	})

	data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
	require.NoError(t, err)

	doc := decodeDoc(t, data)
	assert.Contains(t, doc.Properties, "due_date")
	assert.Contains(t, doc.Properties, "plural_name")
	assert.Empty(t, doc.PropertyKeys,
		"every spelling here is one the bundled table binds to the key it came from")
}

// decoyVocabulary is the corpse policy taken to its limit: it binds EVERY
// spelling the bundled table does not know to a decoy stored key, in both
// namespaces. It conforms — it spells nothing, it never touches a spelling
// the bundled table binds, and it refuses spellings the format could not
// write anyway — so §11.1's preconditions do not exclude it, and a document
// that survives it survives any space that reassigned any of its spellings.
type decoyVocabulary struct{}

const decoyPrefix = "decoy-"

func (decoyVocabulary) PropertySlug(key string) string { return key }
func (decoyVocabulary) TypeSlug(key string) string     { return key }

func (decoyVocabulary) PropertyKey(slug string) (string, bool) {
	if key, ok := (BundledKeyVocabulary{}).PropertyKey(slug); ok {
		return key, true
	}
	if !isWritablePropertyKey(slug) {
		return slug, false
	}
	return decoyPrefix + slug, true
}

func (decoyVocabulary) TypeKey(slug string) (string, bool) {
	if key, ok := (BundledKeyVocabulary{}).TypeKey(slug); ok {
		return key, true
	}
	if !isWritablePropertyKey(slug) {
		return slug, false
	}
	return decoyPrefix + slug, true
}

// The corpus form of the rule, and the one that keeps it exhaustive as the
// format grows: over the whole hostile corpus, a reader that has reassigned
// every non-bundled spelling in the document resolves the SAME stored keys as
// a reader with no vocabulary at all. It can only do that by reading the
// legend, because that is the one thing it is told to consult first.
//
// The comparison is against the package-only reader rather than against the
// snapshot, deliberately: export legitimately drops keys (unwritable ones,
// stripped ones, blocks it does not model), and this invariant is about the
// legend's completeness, not about what export chooses to carry.
func TestInvariant_TheLegendSurvivesAReaderThatReassignedEverySpelling(t *testing.T) {
	reassigned := 0
	for n := 0; n < 300; n++ {
		sbType, snap := hostileSnapshot(n)
		o := Options{ResolveOptions: hostileOptions}
		if sbType == model.SmartBlockType_STType {
			o.ResolveProperties = hostileTypePropResolver{}
		}
		data, err := Marshal(sbType, snap, o)
		if err != nil {
			continue
		}
		_, plain, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
		require.NoError(t, err, "seed %d produced:\n%s", n, data)
		_, decoyed, err := Unmarshal(data,
			Options{GenerateId: seqIds("g"), Keys: decoyVocabulary{}})
		require.NoError(t, err, "seed %d produced:\n%s", n, data)

		require.Equal(t, keyCensus(plain), keyCensus(decoyed),
			"seed %d: a reader that reassigned every non-bundled spelling read "+
				"different stored keys out of:\n%s", n, data)
		for _, k := range keyCensus(plain) {
			if strings.HasPrefix(k, decoyPrefix) {
				t.Fatalf("seed %d: the decoy leaked into the package-only read: %s", n, k)
			}
		}
		reassigned += len(keyCensus(plain))
	}
	// the corpus has to REACH the rule, or a green sweep says nothing. Every
	// custom key in every document is one this vocabulary would reassign.
	require.Greater(t, reassigned, 500,
		"the corpus stopped producing keys for this invariant to be about")
}

// keyCensus lists every stored key an imported snapshot names, at every slot
// the legend covers, sorted. It is the observable the invariant compares: two
// reads of one document must name the same relations and the same types.
func keyCensus(snap *model.SmartBlockSnapshotBase) []string {
	seen := map[string]struct{}{}
	add := func(k string) {
		if !strings.HasSuffix(k, ":") {
			seen[k] = struct{}{}
		}
	}
	if snap.Details != nil {
		for k := range snap.Details.Fields {
			add("detail:" + k)
		}
	}
	for _, t := range snap.ObjectTypes {
		add("type:" + t)
	}
	for _, b := range snap.Blocks {
		if b == nil {
			continue
		}
		switch c := b.Content.(type) {
		case *model.BlockContentOfRelation:
			add("block:" + orEmpty(c.Relation).Key)
		case *model.BlockContentOfLink:
			for _, k := range orEmpty(c.Link).Relations {
				add("link:" + k)
			}
		case *model.BlockContentOfDataview:
			dv := orEmpty(c.Dataview)
			for _, rl := range dv.RelationLinks {
				if rl != nil {
					add("dv:" + rl.Key)
				}
			}
			for _, v := range dv.Views {
				if v == nil {
					continue
				}
				add("group:" + v.GroupRelationKey)
				add("cover:" + v.CoverRelationKey)
				add("end:" + v.EndRelationKey)
				for _, r := range v.Relations {
					if r != nil {
						add("col:" + r.Key)
					}
				}
				for _, s := range v.Sorts {
					if s != nil {
						add("sort:" + s.RelationKey)
					}
				}
				for _, f := range flattenFilters(v.Filters) {
					add("filter:" + f.RelationKey)
				}
			}
		}
	}
	return sortedKeys(seen)
}
