package anyblockjson

// nfcspelling_test.go — §3 says a key's spelling is "NFC-normalized,
// otherwise verbatim", and the WRITE half has honoured it all along
// (PropertyLabel/TypeLabel mint NFC labels). This pins the READ half: a
// spelling resolves under its canonical NFC form, so the precomposed and the
// decomposed bytes of one name land on ONE key instead of minting two
// visually indistinguishable properties — the twin a hostile or hand-edited
// document could plant beside a real one, splitting values between them with
// no diagnostic.

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	// one name, two byte forms: U+00E9 vs "e" + U+0301
	cafeNFC = "Caf\u00e9"
	cafeNFD = "Cafe\u0301"
)

func TestNFCSpelling_ReadPath(t *testing.T) {
	t.Run("an NFD spelling resolves to the NFC key", func(t *testing.T) {
		// given — no legend, so the term resolves verbatim; verbatim under
		// §3 means the canonical NFC form, not the accidental byte form
		doc := `{"version":2,"id":"o1","properties":{"` + cafeNFD + `":"x"}}`

		// when
		_, snap, err := Unmarshal([]byte(doc), Options{})

		// then
		require.NoError(t, err)
		require.NotNil(t, snap.Details)
		assert.Contains(t, snap.Details.Fields, cafeNFC)
		assert.NotContains(t, snap.Details.Fields, cafeNFD)
	})

	t.Run("a pure-ASCII key is unaffected", func(t *testing.T) {
		// given
		doc := `{"version":2,"id":"o1","properties":{"priority":"high"}}`

		// when
		_, snap, err := Unmarshal([]byte(doc), Options{})

		// then
		require.NoError(t, err)
		assert.Contains(t, snap.Details.Fields, "priority")
	})

	t.Run("both forms in one document refuse with both spellings named", func(t *testing.T) {
		// given — the two byte forms of one name, resolving onto one key
		doc := `{"version":2,"id":"o1","properties":{"` + cafeNFC + `":"a","` + cafeNFD + `":"b"}}`

		// when
		_, _, err := Unmarshal([]byte(doc), Options{})

		// then — refused, not a coin flip over which value survives
		require.Error(t, err)
		var ve *ValidationError
		require.True(t, errors.As(err, &ve))
		assert.Contains(t, err.Error(), "normal forms",
			"the refusal should say the two spellings are one name in two Unicode normal forms: %v", err)
	})

	t.Run("both forms as member names warn through the C11 channel", func(t *testing.T) {
		// given — Validate has no vocabulary and must stay warning-grade
		// here: export legitimately writes both forms when the space holds
		// both byte forms as stored keys, each its own address
		doc := `{"version":2,"id":"o1","property_internal_keys":{"` +
			cafeNFC + `":"keyA","` + cafeNFD + `":"keyB"},"properties":{"` +
			cafeNFC + `":"a","` + cafeNFD + `":"b"}}`
		var warnings []Issue

		// when
		err := ValidateWarn([]byte(doc), func(i Issue) { warnings = append(warnings, i) })

		// then
		require.NoError(t, err)
		var twinWarnings int
		for _, w := range warnings {
			if strings.Contains(w.Message, "normal forms") {
				twinWarnings++
			}
		}
		assert.GreaterOrEqual(t, twinWarnings, 2,
			"one twin warning per map carrying both forms: %v", warnings)
	})

	t.Run("a legend maps in both directions across normal forms", func(t *testing.T) {
		t.Run("NFC legend entry answers an NFD slot", func(t *testing.T) {
			// given
			doc := `{"version":2,"id":"o1","property_internal_keys":{"` +
				cafeNFC + `":"customKey1"},"properties":{"` + cafeNFD + `":"x"}}`

			// when
			_, snap, err := Unmarshal([]byte(doc), Options{})

			// then
			require.NoError(t, err)
			assert.Contains(t, snap.Details.Fields, "customKey1")
		})

		t.Run("NFD legend entry answers an NFC slot", func(t *testing.T) {
			// given
			doc := `{"version":2,"id":"o1","property_internal_keys":{"` +
				cafeNFD + `":"customKey2"},"properties":{"` + cafeNFC + `":"x"}}`

			// when
			_, snap, err := Unmarshal([]byte(doc), Options{})

			// then
			require.NoError(t, err)
			assert.Contains(t, snap.Details.Fields, "customKey2")
		})
	})

	t.Run("an NFD stored key still round-trips through its identity legend entry", func(t *testing.T) {
		// given — a stored key whose own bytes are decomposed: export spells
		// it verbatim and writes the identity legend entry, which binds the
		// exact bytes and outranks normalization (legend VALUES are stored
		// keys, byte-verbatim always)
		snap := &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				"id":    {Kind: &types.Value_StringValue{StringValue: "o1"}},
				cafeNFD: {Kind: &types.Value_StringValue{StringValue: "x"}},
			}},
		}
		out, err := Marshal(model.SmartBlockType_Page, snap, Options{})
		require.NoError(t, err)

		// when
		_, got, err := Unmarshal(out, Options{})

		// then — the stored key keeps its own bytes
		require.NoError(t, err)
		assert.Contains(t, got.Details.Fields, cafeNFD)
		assert.NotContains(t, got.Details.Fields, cafeNFC)
	})

	t.Run("a block key slot resolves an NFD spelling through an NFC legend entry", func(t *testing.T) {
		// given — the same choke point serves every slot, not just /properties
		doc := `{"version":2,"id":"o1","property_internal_keys":{"` + cafeNFC + `":"customKey3"},
			"blocks":[{"id":"b1","type":"dataview","views":[{"id":"v1","type":"table",
				"filters":[{"property":"` + cafeNFD + `","condition":"equal","value":"x"}],
				"sorts":[{"property":"` + cafeNFD + `"}]}]}]}`

		// when
		_, snap, err := Unmarshal([]byte(doc), Options{})

		// then
		require.NoError(t, err)
		var dv *model.BlockContentDataview
		for _, b := range snap.Blocks {
			if c, ok := b.Content.(*model.BlockContentOfDataview); ok {
				dv = c.Dataview
			}
		}
		require.NotNil(t, dv)
		require.Len(t, dv.Views, 1)
		require.Len(t, dv.Views[0].Filters, 1)
		assert.Equal(t, "customKey3", dv.Views[0].Filters[0].RelationKey)
		require.Len(t, dv.Views[0].Sorts, 1)
		assert.Equal(t, "customKey3", dv.Views[0].Sorts[0].RelationKey)
	})

	t.Run("the type namespace normalizes the same way", func(t *testing.T) {
		// given
		doc := `{"version":2,"id":"o1","type":"` + cafeNFD + `","type_internal_keys":{"` +
			cafeNFC + `":"customType1"}}`

		// when
		_, snap, err := Unmarshal([]byte(doc), Options{})

		// then
		require.NoError(t, err)
		assert.Contains(t, snap.ObjectTypes, "ot-customType1")
	})

	t.Run("the fragment door's legend normalizes the same way", func(t *testing.T) {
		// given
		raw := []byte(`[{"property":"` + cafeNFD + `","condition":"equal","value":"x"}]`)
		opts := fragFilterOpts()
		opts.Legend = Legend{PropertyKeys: map[string]string{cafeNFC: "customKey4"}}

		// when
		got, err := UnmarshalFilters(raw, opts)

		// then
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "customKey4", got[0].RelationKey)
	})
}
