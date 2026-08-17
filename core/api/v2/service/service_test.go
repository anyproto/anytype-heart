package v2service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeEtag(t *testing.T) {
	t.Run("deterministic and order-independent", func(t *testing.T) {
		// given
		a := ComputeEtag([]string{"headA", "headB"})
		b := ComputeEtag([]string{"headB", "headA"})

		// then
		assert.Equal(t, a, b)
		assert.Len(t, a, 8)
	})

	t.Run("different heads give different etags", func(t *testing.T) {
		assert.NotEqual(t, ComputeEtag([]string{"headA"}), ComputeEtag([]string{"headB"}))
	})

	t.Run("head concatenation is unambiguous", func(t *testing.T) {
		// "ab"+"c" must not hash like "a"+"bc"
		assert.NotEqual(t, ComputeEtag([]string{"ab", "c"}), ComputeEtag([]string{"a", "bc"}))
	})
}

func TestEtagMatches(t *testing.T) {
	heads := []string{"headA", "headB"}

	t.Run("absent If-Match is advisory pass", func(t *testing.T) {
		assert.True(t, EtagMatches("", heads))
	})

	t.Run("display form matches", func(t *testing.T) {
		assert.True(t, EtagMatches(ComputeEtag(heads), heads))
	})

	t.Run("full hash matches", func(t *testing.T) {
		assert.True(t, EtagMatches(headsHash(heads), heads))
	})

	t.Run("stale etag does not match", func(t *testing.T) {
		assert.False(t, EtagMatches(ComputeEtag([]string{"other"}), heads))
	})

	t.Run("quoted and weak header forms match (RFC 7232)", func(t *testing.T) {
		etag := ComputeEtag(heads)
		assert.Equal(t, `"`+etag+`"`, QuoteEtag(etag))
		assert.True(t, EtagMatches(QuoteEtag(etag), heads), "a client echoing the quoted ETag header must match")
		assert.True(t, EtagMatches(`W/"`+etag+`"`, heads), "a weak indicator is tolerated")
	})
}

func TestEncodeEnvelope(t *testing.T) {
	t.Run("canonical key order with unknown keys last", func(t *testing.T) {
		// given
		fields := map[string]json.RawMessage{
			"blocks":  json.RawMessage(`[]`),
			"etag":    json.RawMessage(`"abcd1234"`),
			"id":      json.RawMessage(`"obj1"`),
			"version": json.RawMessage(`1`),
			"zzz":     json.RawMessage(`true`),
		}
		want := `{"version":1,"etag":"abcd1234","id":"obj1","blocks":[],"zzz":true}`

		// when
		got, err := encodeEnvelope(fields)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, string(got))
	})

	t.Run("compacts the interior of embedded values (C3)", func(t *testing.T) {
		// given — the indented bytes anyblockjson.Marshal produces (SPEC §4
		// canonical form), re-embedded verbatim by parseEnvelope
		fields := map[string]json.RawMessage{
			"id":     json.RawMessage(`"obj1"`),
			"blocks": json.RawMessage("[\n  {\n    \"id\": \"b1\",\n    \"type\": \"paragraph\",\n    \"text\": \"a  b\"\n  }\n]"),
			"refs":   json.RawMessage("{\n  \"ab12c\": \"bafyreiab12c\"\n}"),
		}
		want := `{"id":"obj1","refs":{"ab12c":"bafyreiab12c"},"blocks":[{"id":"b1","type":"paragraph","text":"a  b"}]}`

		// when
		got, err := encodeEnvelope(fields)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, string(got), "C3 is about the whole body, not just the envelope")
	})

	t.Run("string contents survive compaction byte for byte", func(t *testing.T) {
		// given — the format's writer escapes without HTML escaping;
		// json.Compact must neither re-escape `<`/`>`/`&` nor rewrite a RAW
		// U+2028/U+2029. The fixture carries the six-character \u2028 escape
		// (which Compact could never touch) AND the raw U+2028 and U+2029
		// characters after "raw:" — only the raw ones actually pin the claim.
		fields := map[string]json.RawMessage{
			"id": json.RawMessage("{\n  \"a\": \"<b>&amp; \\u2028 raw:   café — ünïcode\"\n}"),
		}

		// when
		got, err := encodeEnvelope(fields)

		// then
		require.NoError(t, err)
		want := "{\"id\":{\"a\":\"<b>&amp; \\u2028 raw:   caf\u00e9 \u2014 \u00fcn\u00efcode\"}}"
		assert.Equal(t, want, string(got))
	})

	t.Run("round-trips through parseEnvelope", func(t *testing.T) {
		// given
		doc := []byte(`{"version":1,"id":"x","blocks":[{"type":"paragraph","text":"hi"}]}`)

		// when
		fields, err := parseEnvelope(doc)
		require.NoError(t, err)
		out, err := encodeEnvelope(fields)

		// then
		require.NoError(t, err)
		assert.JSONEq(t, string(doc), string(out))
	})
}

func TestV2ObjectQueryValidate(t *testing.T) {
	tests := []struct {
		name     string
		query    ObjectQuery
		wantErr  string // expected error code, "" = valid
		wantPlan func(t *testing.T, plan objectReadPlan)
	}{
		{
			// the edit shape: short labels for minted block ids; object refs
			// are full inline on every shape (the legend is a measured loss —
			// TOKENS §1.2, §8.26)
			name:  "defaults: both sections, short block labels, anyblock",
			query: ObjectQuery{},
			wantPlan: func(t *testing.T, plan objectReadPlan) {
				assert.True(t, plan.wantProperties)
				assert.True(t, plan.wantBlocks)
				assert.True(t, plan.compactBlockLabels)
				assert.False(t, plan.markdown)
			},
		},
		{
			name:  "include=properties suppresses blocks",
			query: ObjectQuery{Include: "properties"},
			wantPlan: func(t *testing.T, plan objectReadPlan) {
				assert.True(t, plan.wantProperties)
				assert.False(t, plan.wantBlocks)
			},
		},
		{
			// the export shape: full block ids so a GET body PUTs back as a
			// minimal diff; no legend here either
			name:  "ids=full is the export shape: full block ids",
			query: ObjectQuery{Ids: "full"},
			wantPlan: func(t *testing.T, plan objectReadPlan) {
				assert.False(t, plan.compactBlockLabels)
			},
		},
		{
			name:  "ids=compact is the explicit spelling of the default",
			query: ObjectQuery{Ids: "compact"},
			wantPlan: func(t *testing.T, plan objectReadPlan) {
				assert.True(t, plan.compactBlockLabels)
			},
		},
		{
			// T7: the outline fixes the axis and ignores ?ids=
			name:  "outline fixes the axis and ignores ids=full",
			query: ObjectQuery{Outline: true, Ids: "full"},
			wantPlan: func(t *testing.T, plan objectReadPlan) {
				assert.True(t, plan.compactBlockLabels)
			},
		},
		{name: "outline and block conflict", query: ObjectQuery{Outline: true, Block: "b1"}, wantErr: "ambiguous_input"},
		{name: "outline and md conflict", query: ObjectQuery{Outline: true, Format: "md"}, wantErr: "ambiguous_input"},
		{name: "block and md conflict", query: ObjectQuery{Block: "b1", Format: "md"}, wantErr: "ambiguous_input"},
		{name: "unknown ids value", query: ObjectQuery{Ids: "short"}, wantErr: "validation_failed"},
		{name: "unknown format value", query: ObjectQuery{Format: "html"}, wantErr: "validation_failed"},
		{name: "unknown include value", query: ObjectQuery{Include: "blocks,everything"}, wantErr: "validation_failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := tt.query.validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				tt.wantPlan(t, plan)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
