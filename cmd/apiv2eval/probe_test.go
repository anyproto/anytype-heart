package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeToolSpecsComeFromThePublishedSchemas(t *testing.T) {
	// when
	specs, err := probeToolSpecs(exampleAsPublished, false)

	// then
	require.NoError(t, err)
	require.Len(t, specs, 2)

	byName := map[string]toolSpec{}
	for _, s := range specs {
		byName[s.Name] = s
	}
	// the matched pair the probe exists for: one schema publishes the payload
	// id slot, the other does not — if this ever stops holding, the probe is
	// measuring nothing
	insert := string(byName["insert_blocks"].Parameters)
	replace := string(byName["replace_subtree"].Parameters)
	assert.NotContains(t, insert, `"id"`, "insert_blocks must publish no id slot anywhere (§8.30/§8.31)")
	assert.Contains(t, replace, `"id"`, "replace_subtree still publishes one — it is the control")
}

func TestStaticRefusalRisks(t *testing.T) {
	tests := []struct {
		name string
		op   string
		args string
		want []string
	}{
		{
			// the shape gemma4:e2b produced 20/20 times: it is no longer a
			// refusal, so it is no longer a risk (§8.32)
			name: "position with no targeting field targets the document",
			op:   "insert_blocks",
			args: `{"op":"insert_blocks","blocks":[{"type":"paragraph"}],"position":"last"}`,
		},
		{
			name: "the same shape asking for the start of the document",
			op:   "insert_blocks",
			args: `{"op":"insert_blocks","blocks":[{"type":"paragraph"}],"position":"first"}`,
		},
		{
			name: "position alongside after is refused too",
			op:   "insert_blocks",
			args: `{"op":"insert_blocks","after":"b3","blocks":[{"type":"paragraph"}],"position":"last"}`,
			want: []string{riskPositionNotIside},
		},
		{
			name: "position with inside is the one legal use",
			op:   "insert_blocks",
			args: `{"op":"insert_blocks","inside":"b3","blocks":[{"type":"paragraph"}],"position":"first"}`,
		},
		{
			name: "no position, no risk",
			op:   "insert_blocks",
			args: `{"op":"insert_blocks","markdown":"## Risks"}`,
		},
		{
			name: "ops without targeting are out of scope",
			op:   "replace_subtree",
			args: `{"op":"replace_subtree","id":"b7","blocks":[{"type":"paragraph"}],"position":"last"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			var args map[string]any
			require.NoError(t, json.Unmarshal([]byte(tt.args), &args))

			// when
			got := staticRefusalRisks(tt.op, args)

			// then
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestServedOpExampleIsAnInstanceOfItsOwnSchema(t *testing.T) {
	// This test was written to DOCUMENT the defect: GET /v2/schemas/ops/{op}
	// answered with a `schema` describing ONE op (additionalProperties:false,
	// `op` required with a const) beside an `example` that was a whole PATCH
	// request body, {"ops":[{…}]} — so the example was rejected by the schema
	// served with it. Unwrapping it took gemma4:e4b's missing-`op` rate from
	// 9/60 to 0/60, and the route now serves the op level. The assertion is
	// inverted rather than deleted, so the harness's two example shapes stay
	// meaningful and a regression to the wrapped body is caught here as well
	// as at the source (TestServedOpExampleValidatesAgainstItsOwnSchema).
	specs, err := probeToolSpecs(exampleAsPublished, false)
	require.NoError(t, err)
	for _, spec := range specs {
		var schema struct {
			Properties           map[string]any `json:"properties"`
			AdditionalProperties *bool          `json:"additionalProperties"`
		}
		require.NoError(t, json.Unmarshal(spec.Parameters, &schema))
		require.NotNil(t, schema.AdditionalProperties)
		assert.False(t, *schema.AdditionalProperties, "%s: the op schema is C13-strict", spec.Name)
		assert.NotContains(t, schema.Properties, "ops", "%s: the schema describes one op, not a request body", spec.Name)
		assert.NotContains(t, spec.Description, `{"ops":[`, "%s: the served example is not a request body", spec.Name)
		assert.Contains(t, spec.Description, `"op":"`+spec.Name+`"`, "%s: it is an instance of the schema", spec.Name)
	}

	// the two shapes the probe can serve are now the same bytes: -probe-example
	// stays a knob, but it no longer separates anything
	unwrapped, err := probeToolSpecs(exampleAtOpLevel, false)
	require.NoError(t, err)
	require.Len(t, unwrapped, len(specs))
	for i, spec := range unwrapped {
		assert.Equal(t, specs[i].Description, spec.Description)
	}
}

func TestRewriteConstAsEnumTouchesOnlyTheDiscriminator(t *testing.T) {
	// given — the harness's one deliberate deviation from a served schema
	served, err := probeToolSpecs(exampleAtOpLevel, false)
	require.NoError(t, err)
	swapped, err := probeToolSpecs(exampleAtOpLevel, true)
	require.NoError(t, err)
	require.Len(t, swapped, len(served))

	for i, spec := range swapped {
		// then
		assert.Contains(t, string(served[i].Parameters), `"op":{"const":"`+spec.Name+`"}`)
		assert.Contains(t, string(spec.Parameters), `"op":{"enum":["`+spec.Name+`"]}`)
		assert.NotContains(t, string(spec.Parameters), `"const"`, "no other const may be rewritten")
		// everything else is byte-identical
		restored := strings.Replace(string(spec.Parameters),
			`"op":{"enum":["`+spec.Name+`"]}`, `"op":{"const":"`+spec.Name+`"}`, 1)
		assert.Equal(t, string(served[i].Parameters), restored)
	}
}

func TestProbeRecordsWhatWasWrittenIntoTheDiscriminator(t *testing.T) {
	tests := []struct {
		name        string
		args        string
		wantMissing bool
		wantValue   string
	}{
		{name: "the const, correctly", args: `{"op":"insert_blocks","markdown":"x"}`},
		{name: "absent", args: `{"markdown":"x"}`, wantMissing: true},
		{name: "a positional word", args: `{"op":"append","markdown":"x"}`, wantValue: `"append"`},
		{name: "an empty object", args: `{"op":{},"markdown":"x"}`, wantValue: `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			model, _ := newScriptedModel(toolCallTurn("insert_blocks", tt.args))
			defer model.Close()

			// when
			got := probeOnce(context.Background(), newChatClient(model.URL, "", 30*time.Second),
				"stub", probeCase{id: "c", wantOp: "insert_blocks"}, nil, "sys", options{}, "run1", 1)

			// then
			assert.Equal(t, tt.wantMissing, got.MissingOpConst)
			assert.Equal(t, tt.wantValue, got.OpConstValue)
		})
	}
}
