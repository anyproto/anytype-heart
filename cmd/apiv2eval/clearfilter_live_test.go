//go:build livesmoke

package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/apiv2eval/heartboot"
	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

// TestLiveClearFilter establishes what actually clears a view filter, so the
// wrapper's clearing path is built on a measurement and not on the op
// comment's word "null".
func TestLiveClearFilter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()
	heart, err := heartboot.Start(ctx, heartboot.Options{AccountName: "clearfilter", AppName: "clearfilter"})
	require.NoError(t, err)
	defer heart.Stop()

	api := newAPIClient(heart.APIURL, heart.APIKey, nil)
	spaceId, err := api.createSpace(ctx, "clearfilter")
	require.NoError(t, err)
	deadline := time.Now().Add(spaceReadyTimeout)
	for {
		if _, err := api.call(ctx, http.MethodGet, "/v2/spaces/"+spaceId, nil, nil, nil); err == nil {
			break
		}
		require.False(t, time.Now().After(deadline))
		time.Sleep(500 * time.Millisecond)
	}

	var created struct {
		Id string `json:"id"`
	}
	_, err = api.call(ctx, http.MethodPost, "/v2/spaces/"+spaceId+"/queries", nil,
		map[string]any{"name": "Open tasks", "type": "Page"}, &created)
	require.NoError(t, err)

	client := wrapper.NewClient(api.baseURL, api.apiKey)
	client.HTTP = &http.Client{Timeout: 60 * time.Second}
	ts, err := newMCPToolset(ctx, wrapper.NewRunner(client, wrapper.NewMemoryStore()), wrapper.TierLarge)
	require.NoError(t, err)

	set := ts.call(ctx, "update_view", map[string]any{
		"space": spaceId, "object": created.Id, "filter": `Name != ""`})
	t.Logf("set filter -> isError=%v %s", set.IsError, set.Text)
	require.False(t, set.IsError)
	read := ts.call(ctx, "read", map[string]any{"space": spaceId, "object": created.Id})
	t.Logf("after set:\n%s", read.Text)

	require.Contains(t, read.Text, `filter: Name != ""`)

	// the tool's own clearing path, end to end
	cleared := ts.call(ctx, "update_view", map[string]any{
		"space": spaceId, "object": created.Id, "filter": "none"})
	t.Logf("clear filter -> isError=%v %s", cleared.IsError, cleared.Text)
	require.False(t, cleared.IsError, "clearing must not be refused")

	after := ts.call(ctx, "read", map[string]any{"space": spaceId, "object": created.Id})
	t.Logf("after clear:\n%s", after.Text)
	require.Contains(t, after.Text, "filter: (none", "the filter must actually be gone")
}
