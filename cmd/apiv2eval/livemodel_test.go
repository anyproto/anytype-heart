package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

// A real model against a stub API. This proves the one thing the scripted
// tests cannot — that a ~2B model can read the tier's published schemas and
// drive the loop at all — without needing the app running, and it is how the
// harness gets smoke-tested before an hours-long run. It is NOT a result:
// the API is a stub, so nothing here says whether an edit lands.
//
//	APIV2EVAL_LIVE_MODEL=gemma4:e2b OLLAMA_BASE_URL=http://host:11434/v1 \
//	  go test ./cmd/apiv2eval -run TestLiveModelDrivesTheTierSmallToolSet -v
func TestLiveModelDrivesTheTierSmallToolSet(t *testing.T) {
	model := os.Getenv("APIV2EVAL_LIVE_MODEL")
	if model == "" {
		t.Skip("set APIV2EVAL_LIVE_MODEL (and OLLAMA_BASE_URL) to run the live smoke test")
	}
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434/v1"
	}

	// given
	api := newStubAPI(servedDoc)
	defer api.Close()
	rec := &recorder{}
	client := wrapper.NewClient(api.URL, "key")
	client.HTTP = &http.Client{Transport: &recordingTransport{base: http.DefaultTransport, rec: rec}}
	runner := wrapper.NewRunner(client, wrapper.NewMemoryStore())
	ts, err := newMCPToolset(context.Background(), runner, wrapper.TierSmall)
	require.NoError(t, err)
	defer ts.close()

	// when
	tr, err := runAgent(context.Background(), agentConfig{
		chat:     newChatClient(baseURL, "ollama", 5*time.Minute),
		model:    model,
		maxTurns: 6,
		rec:      rec,
	}, ts, ts.instructions()+"\n\nWork in the Anytype space space1. When the work is done, reply with one short sentence and no tool call.",
		`In the note titled "Quarterly plan ab12", change Q3 to Q4. Change nothing else.`)

	// then
	require.NoError(t, err)
	t.Logf("stopped by %s after %d turns, %d prompt + %d completion tokens\n%s",
		tr.StoppedBy, len(tr.Turns), tr.PromptTokens, tr.CompletionTokens, summarizeCalls(tr.Calls))
	assert.NotEmpty(t, tr.Calls, "the model made no tool call at all — the tier's schemas did not reach it")

	sig := analyze(tr.Calls)
	t.Logf("refs: %+v", sig.Refs)
	t.Logf("repairs: %+v", sig.Repairs)
}

// The same smoke test for the ops arm, whose schemas are an order of
// magnitude larger than the wrapper's — the size at which a small model
// stops answering is worth knowing before, not during, a long run.
func TestLiveModelDrivesTheOpsArm(t *testing.T) {
	model := os.Getenv("APIV2EVAL_LIVE_MODEL")
	if model == "" {
		t.Skip("set APIV2EVAL_LIVE_MODEL (and OLLAMA_BASE_URL) to run the live smoke test")
	}
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434/v1"
	}

	// given — the ops arm needs the published op schemas, which only the
	// server serves; against the stub they are placeholders, so this checks
	// the loop and the tool count, not the schemas themselves
	stub := httptest.NewServer(&stubAPI{doc: servedDoc})
	defer stub.Close()
	client := newAPIClient(stub.URL, "key", &recordingTransport{base: http.DefaultTransport, rec: &recorder{}})
	ts, err := newOpsToolset(context.Background(), client, "space1", "obj1")
	require.NoError(t, err)

	// when
	tr, err := runAgent(context.Background(), agentConfig{
		chat:     newChatClient(baseURL, "ollama", 5*time.Minute),
		model:    model,
		maxTurns: 4,
	}, ts, ts.instructions(), "Change Q3 to Q4 in this document. Read it first.")

	// then
	require.NoError(t, err)
	t.Logf("stopped by %s after %d turns\n%s", tr.StoppedBy, len(tr.Turns), summarizeCalls(tr.Calls))
	assert.NotEmpty(t, tr.Calls)
}
