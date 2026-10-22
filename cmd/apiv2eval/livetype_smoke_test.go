//go:build livesmoke

package main

// A live end-to-end check of create_type against a real heart: the wrapper
// creates the type, and the TASK'S OWN grader reads it back. It exists
// because the grader reads options from the type document while the tool
// writes them through POST /properties — two different endpoints, and only a
// live run can say whether the option names survive the round trip.
//
//	go test ./cmd/apiv2eval/ -tags livesmoke -run TestLiveCreateType -v -timeout 10m

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/apiv2eval/heartboot"
	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

func TestLiveCreateType(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()

	heart, err := heartboot.Start(ctx, heartboot.Options{AccountName: "livesmoke", AppName: "livesmoke"})
	require.NoError(t, err)
	defer heart.Stop()
	t.Logf("heart %s at %s", heart.AccountId, heart.APIURL)

	api := newAPIClient(heart.APIURL, heart.APIKey, nil)
	spaceId, err := api.createSpace(ctx, "livesmoke")
	require.NoError(t, err)
	deadline := time.Now().Add(spaceReadyTimeout)
	for {
		if _, err := api.call(ctx, http.MethodGet, "/v2/spaces/"+spaceId, nil, nil, nil); err == nil {
			break
		}
		require.False(t, time.Now().After(deadline), "space never became readable")
		time.Sleep(500 * time.Millisecond)
	}

	client := wrapper.NewClient(api.baseURL, api.apiKey)
	client.HTTP = &http.Client{Timeout: 60 * time.Second}
	ts, err := newMCPToolset(ctx, wrapper.NewRunner(client, wrapper.NewMemoryStore()), wrapper.TierLarge)
	require.NoError(t, err)

	const typeName = "Sourdough Recipe"
	out := ts.call(ctx, "create_type", map[string]any{
		"space":      spaceId,
		"name":       typeName,
		"properties": "Cook time: number, Source: url, Rating: select(Low, Medium, High)",
	})
	t.Logf("create_type (isError=%v) ->\n%s", out.IsError, out.Text)
	require.False(t, out.IsError, "create_type failed")

	// the grader the eval will actually score with
	row, err := api.findTypeByName(ctx, spaceId, typeName)
	require.NoError(t, err)
	require.NotNil(t, row, "the type the tool reported creating is not readable back")
	dump, _ := json.MarshalIndent(row, "", "  ")
	t.Logf("findTypeByName ->\n%s", dump)

	for _, p := range row.Properties {
		if p.Name == "Rating" {
			require.NotEmpty(t, p.Options, "GRADER GAP: the type document serves no options for Rating")
		}
	}

	// describe's option-listing mode, against a property with far more
	// options than the inline preview shows (describeOptionsLimit = 25)
	var many []string
	for i := 0; i < 60; i++ {
		many = append(many, fmt.Sprintf("Region %02d", i))
	}
	optDefs := make([]map[string]any, 0, len(many))
	for _, n := range many {
		optDefs = append(optDefs, map[string]any{"name": n})
	}
	_, err = api.call(ctx, http.MethodPost, "/v2/spaces/"+spaceId+"/properties", nil,
		map[string]any{"name": "Region", "format": "select", "options": optDefs}, nil)
	require.NoError(t, err)

	plain := ts.call(ctx, "describe", map[string]any{"space": spaceId, "type": typeName})
	t.Logf("describe (inline preview) ->\n%s", plain.Text)

	full := ts.call(ctx, "describe", map[string]any{"space": spaceId, "type": typeName, "options": "Region"})
	require.False(t, full.IsError, "describe options failed: %s", full.Text)
	t.Logf("describe options=Region ->\n%s", full.Text)
	require.Contains(t, full.Text, "Region 59", "the full listing must reach past the 25-option preview")

	narrowed := ts.call(ctx, "describe", map[string]any{
		"space": spaceId, "type": typeName, "options": "Region", "starting_with": "Region 4"})
	require.False(t, narrowed.IsError, "starting_with failed: %s", narrowed.Text)
	t.Logf("describe options=Region starting_with=\"Region 4\" ->\n%s", narrowed.Text)

	notSelect := ts.call(ctx, "describe", map[string]any{"space": spaceId, "type": typeName, "options": "Cook time"})
	t.Logf("describe options=Cook time (a number) -> isError=%v %s", notSelect.IsError, notSelect.Text)
	require.True(t, notSelect.IsError, "listing options of a number must be refused")

	// The bundled-optionless case: on a fresh account Status exists as a
	// select holding nothing, so this is what a model writes and what it
	// gets back. The refusal must lead with the repair that delivers the
	// options asked for.
	bundled := ts.call(ctx, "create_type", map[string]any{
		"space": spaceId, "name": "Field note",
		"properties": "Status: select(Todo, Doing, Done)"})
	t.Logf("create_type with a bundled optionless select -> isError=%v\n%s", bundled.IsError, bundled.Text)
	require.True(t, bundled.IsError, "Status holds its own options; declaring different ones must be refused")
	require.Contains(t, bundled.Text, "nothing was created")
	// "Todo" vs the bundled "To Do" differs by a space — the near-miss the
	// refusal must name, since it is the repair that costs one word
	require.Contains(t, bundled.Text, "Spell the option exactly as it exists",
		"a separator-only near-miss must be named, not reported as simply absent")

	// and the case-only mismatch names the spelling instead of a structural move
	caseOnly := ts.call(ctx, "create_type", map[string]any{
		"space": spaceId, "name": "Bake log", "properties": "Rating: select(low)"})
	t.Logf("create_type with a case-only option mismatch -> isError=%v\n%s", caseOnly.IsError, caseOnly.Text)
	require.True(t, caseOnly.IsError, "an option differing only in case must be refused")
	require.Contains(t, caseOnly.Text, "Spell the option exactly as it exists",
		"the cheapest repair must be named first")
}
