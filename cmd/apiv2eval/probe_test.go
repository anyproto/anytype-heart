package main

import (
	"encoding/json"
	"strings"
	"testing"

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
	insert := string(byName["insertBlocks"].Parameters)
	replace := string(byName["replaceSubtree"].Parameters)
	assert.NotContains(t, insert, `"id"`, "insertBlocks must publish no id slot anywhere (§8.30/§8.31)")
	assert.Contains(t, replace, `"id"`, "replaceSubtree still publishes one — it is the control")
}

func TestStaticRefusalRisks(t *testing.T) {
	tests := []struct {
		name string
		op   string
		args string
		want []string
	}{
		{
			name: "position with no targeting field is refused at .position",
			op:   "insertBlocks",
			args: `{"op":"insertBlocks","blocks":[{"type":"paragraph"}],"position":"last"}`,
			want: []string{riskPositionNoTarget},
		},
		{
			name: "position alongside after is refused too",
			op:   "insertBlocks",
			args: `{"op":"insertBlocks","after":"b3","blocks":[{"type":"paragraph"}],"position":"last"}`,
			want: []string{riskPositionNotIside},
		},
		{
			name: "position with inside is the one legal use",
			op:   "insertBlocks",
			args: `{"op":"insertBlocks","inside":"b3","blocks":[{"type":"paragraph"}],"position":"first"}`,
		},
		{
			name: "no position, no risk",
			op:   "insertBlocks",
			args: `{"op":"insertBlocks","markdown":"## Risks"}`,
		},
		{
			name: "ops without targeting are out of scope",
			op:   "replaceSubtree",
			args: `{"op":"replaceSubtree","id":"b7","blocks":[{"type":"paragraph"}],"position":"last"}`,
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

func TestServedOpExampleIsNotAnInstanceOfItsOwnSchema(t *testing.T) {
	// GET /v2/schemas/ops/{op} answers with a `schema` describing ONE op
	// (additionalProperties:false, `op` required with a const) and an
	// `example` that is a whole PATCH request body, {"ops":[{…}]}. The
	// example would therefore be REJECTED by the schema served beside it.
	// This pins the state of the discovery response as it is today, so the
	// harness's two example shapes stay meaningful; it is a finding, not an
	// endorsement.
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
		assert.Contains(t, spec.Description, `{"ops":[`, "%s: the served example is a whole request body", spec.Name)
	}

	// unwrapped, the same example IS an instance of the schema
	unwrapped, err := probeToolSpecs(exampleAtOpLevel, false)
	require.NoError(t, err)
	for _, spec := range unwrapped {
		assert.NotContains(t, spec.Description, `{"ops":[`)
		assert.Contains(t, spec.Description, `"op":"`+spec.Name+`"`)
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
