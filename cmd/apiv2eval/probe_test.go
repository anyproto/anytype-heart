package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeToolSpecsComeFromThePublishedSchemas(t *testing.T) {
	// when
	specs, err := probeToolSpecs()

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
