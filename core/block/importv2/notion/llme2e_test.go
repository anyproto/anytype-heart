package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"

	"github.com/anyproto/anytype-heart/core/ai/llmclient"
	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/llmplan"
	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// planArtifactPrefixes are the source-key prefixes of objects a plan brings
// into existence — its new types, the relations those types name, and those
// relations' options. Everything else is workspace content, keyed by notion id.
var planArtifactPrefixes = []string{"type:", "relation:", "option:"}

// TestLiveLLMPlan is the real-LLM end-to-end (docs/ImportV2LLM.md §8): a Notion
// workspace converts once with a real OpenAI-planned run and once with the
// naive planner, over byte-identical API traffic, so every difference is
// provably the plan's doing.
//
// The Notion side goes through a persistent cassette: the first run crawls the
// live workspace and tapes it, later runs replay that tape, so iterating costs
// one OpenAI call instead of a full re-crawl. The LLM call is always live.
//
//	NOTION_TOKEN=ntn_… OPENAI_API_KEY=sk-… \
//	  go test ./core/block/importv2/notion/ -run TestLiveLLMPlan -count=1 -v -timeout 30m
//
// IMPORTV2_LLM_MODEL overrides the model (default gpt-4o-mini).
// IMPORTV2_LLM_CASSETTE overrides the tape path (point it at
// testdata/cassettes/workspace to reuse the committed recording).
func TestLiveLLMPlan(t *testing.T) {
	// IMPORTV2_LLM_ENDPOINT points the harness at any OpenAI-compatible server
	// (ollama's /v1 shim, llama-server, a local proxy). Those need no real
	// credential, so the key is only required for the OpenAI default.
	endpoint := os.Getenv("IMPORTV2_LLM_ENDPOINT")
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
		if openaiKey == "" {
			t.Skip("set OPENAI_API_KEY (or IMPORTV2_LLM_ENDPOINT) to run the live LLM import e2e")
		}
	}
	if openaiKey == "" {
		openaiKey = "local"
	}
	cassettePath := os.Getenv("IMPORTV2_LLM_CASSETTE")
	if cassettePath == "" {
		cassettePath = filepath.Join(os.TempDir(), "importv2-llm-e2e-workspace")
	}
	notionToken := os.Getenv("NOTION_TOKEN")
	_, statErr := os.Stat(cassettePath + ".yaml")
	recordLive := statErr != nil
	if recordLive && notionToken == "" {
		t.Skipf("no cassette at %s.yaml and no NOTION_TOKEN to record one", cassettePath)
	}
	model := os.Getenv("IMPORTV2_LLM_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}

	// Observe the completions at the transport so the report can tell a model
	// that answered badly from one that was cut off: a truncated completion
	// comes back as finish_reason "length" but reaches llmplan as a plain
	// parse error, which is a very different diagnosis.
	observer := &observingTransport{}
	llmClient, err := llmclient.New(llmclient.Config{
		Endpoint: endpoint,
		Model:    model,
		Token:    openaiKey,
		// No client Timeout: production builds a bare http.Client and lets the
		// planner's ctx budget bound the call. Matching that keeps the test
		// honest about how the real thing behaves.
	}, llmclient.WithHTTPClient(&http.Client{Transport: observer}))
	require.NoError(t, err)

	// The planner the converter consults, wrapped so the test keeps the exact
	// evidence sent and the plan that came back.
	var (
		schemas  []schemaplan.ContainerSchema
		plan     schemaplan.Plan
		planErr  error
		planTook time.Duration
	)
	var planOpts []llmplan.Option
	if effort := os.Getenv("IMPORTV2_LLM_EFFORT"); effort != "" {
		planOpts = append(planOpts, llmplan.WithReasoningEffort(effort))
		t.Logf("reasoning effort set to %q", effort)
	}
	if os.Getenv("IMPORTV2_LLM_PERCONTAINER") != "" {
		planOpts = append(planOpts, llmplan.WithPerContainerCalls())
		t.Log("per-container (tier 3) planner engaged")
	}
	if raw := os.Getenv("IMPORTV2_LLM_BUDGET"); raw != "" {
		budget, err := time.ParseDuration(raw)
		require.NoError(t, err, "IMPORTV2_LLM_BUDGET must be a duration like 300s")
		planOpts = append(planOpts, llmplan.WithBudget(budget))
		t.Logf("plan budget overridden to %s (production default is 90s)", budget)
	}
	live := llmplan.New(llmClient, planOpts...)
	capturing := schemaplan.PlannerFunc(func(ctx context.Context, in []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
		schemas = in
		started := time.Now()
		plan, planErr = live.Plan(ctx, in)
		planTook = time.Since(started)
		return plan, planErr
	})

	mode := recorder.ModeReplayOnly
	token := "cassette-token"
	if recordLive {
		mode, token = recorder.ModeRecordOnly, notionToken
		t.Logf("no cassette at %s.yaml — crawling the live workspace and taping it", cassettePath)
	}
	llmSink := runNotionConvert(t, token, cassettePath, mode, WithPlanner(capturing), WithContentSamples())
	naiveSink := runNotionConvert(t, "cassette-token", cassettePath, recorder.ModeReplayOnly)

	require.NotEmpty(t, schemas, "workspace has no databases; the plan phase never ran")

	// Diagnostics first: a failed plan is exactly when the report matters most.
	reportLLMRun(t, schemas, plan, planTook, model, llmSink, naiveSink, observer.seen())

	// then — the model answered, and what it proposed reached the objects.
	assert.NoError(t, planErr, "live plan call failed")
	assert.Empty(t, issueMessages(llmSink, importv2.IssueLLMPlanFailed),
		"plan degraded to the naive rules")
	assert.NotEmpty(t, plan.Containers, "the model typed nothing at all")
	assert.NotEmpty(t, issueMessages(llmSink, importv2.IssueTypeSuggested),
		"nothing the model proposed survived sanitization")

	// Content objects — the workspace's own pages and databases — must come
	// through a planned run exactly as they do without one. Definition objects
	// (types, relations, their options) are the plan's to change: a remap
	// moves a property onto a different relation by design, so their source
	// keys legitimately differ between the runs.
	assert.Equal(t, contentKeys(naiveSink), contentKeys(llmSink),
		"the plan changed which content objects were imported")

	for _, run := range []struct {
		name string
		sink *recordingSink
	}{{"llm", llmSink}, {"naive", naiveSink}} {
		for _, issue := range run.sink.issues {
			assert.LessOrEqual(t, issue.Severity, importv2.SeverityWarning,
				"%s run issue: %s", run.name, issue.Error())
		}
	}
}

// contentKeys are the source keys of the workspace's own objects — everything
// but the definition objects a plan mints (types, relations, options).
func contentKeys(sink *recordingSink) []string {
	var out []string
	for _, object := range sink.objects {
		if isPlanArtifact(object.SourceKey) {
			continue
		}
		out = append(out, object.SourceKey)
	}
	sort.Strings(out)
	return out
}

// runNotionConvert runs one full conversion against the Notion API through a
// recorder — recording live traffic, or replaying what a previous run taped.
func runNotionConvert(t *testing.T, token, cassettePath string, mode recorder.Mode, opts ...Option) *recordingSink {
	t.Helper()

	scrub := func(interaction *cassette.Interaction) error {
		delete(interaction.Request.Headers, "Authorization")
		return nil
	}
	rec, err := recorder.New(cassettePath,
		recorder.WithMode(mode),
		recorder.WithHook(scrub, recorder.AfterCaptureHook),
		recorder.WithSkipRequestLatency(true),
		recorder.WithMatcher(cassette.NewDefaultMatcher(cassette.WithIgnoreAuthorization())),
	)
	require.NoError(t, err)
	// Stop flushes the tape; the replay run reads what this one wrote.
	defer func() { require.NoError(t, rec.Stop()) }()

	apiClient := client.NewClient(token,
		client.WithTransport(&http.Client{Transport: rec, Timeout: time.Minute}),
		client.WithRateLimit(1000),
		// Same reasoning as TestCassetteWorkspace: an unmatched replay looks
		// like a retryable transport error, so keep the backoff cheap.
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond, TotalBudget: 30 * time.Second}),
	)
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir(), opts...)

	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil }))
	sink := &recordingSink{}
	_, err = converter.Convert(context.Background(), sink)
	require.NoError(t, err)
	return sink
}

func isPlanArtifact(sourceKey string) bool {
	for _, prefix := range planArtifactPrefixes {
		if strings.HasPrefix(sourceKey, prefix) {
			return true
		}
	}
	return false
}

// completionInfo is what one chat completion reveals about how it ended.
type completionInfo struct {
	finishReason     string
	promptTokens     int
	completionTokens int
	content          string
}

// observingTransport tees every completion response so the test can read the
// provider's own account of the call. It never alters the exchange.
type observingTransport struct {
	mu          sync.Mutex
	completions []completionInfo
}

func (t *observingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil || resp.Body == nil {
		return resp, err
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))

	var parsed struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &parsed) == nil && len(parsed.Choices) > 0 {
		t.mu.Lock()
		t.completions = append(t.completions, completionInfo{
			finishReason:     parsed.Choices[0].FinishReason,
			promptTokens:     parsed.Usage.PromptTokens,
			completionTokens: parsed.Usage.CompletionTokens,
			content:          parsed.Choices[0].Message.Content,
		})
		t.mu.Unlock()
	}
	return resp, nil
}

func (t *observingTransport) seen() []completionInfo {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]completionInfo(nil), t.completions...)
}

// nameLooksWrong flags a plan-supplied display name that should never reach
// RelationKeyName: prose the model wrote into the wrong field, or characters
// that break a name field.
func nameLooksWrong(name string) (bool, string) {
	switch {
	case name == "":
		return false, ""
	case len(([]rune)(name)) > 64:
		return true, "longer than 64 runes"
	case strings.ContainsAny(name, "\n\r\t"):
		return true, "contains control characters"
	case strings.Contains(name, " remapped to "):
		return true, "model wrote prose into the name field"
	case strings.TrimSpace(name) != name:
		return true, "leading or trailing whitespace"
	}
	return false, ""
}

func sortedKeys(m map[string]schemaplan.PropertyPlan) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// objectTypes indexes the emitted objects' type keys by source key — the
// observable end of the plan's typing decisions.
func objectTypes(sink *recordingSink) map[string]string {
	out := map[string]string{}
	for _, object := range sink.objects {
		if object.Payload == nil || len(object.Payload.ObjectTypes) == 0 {
			continue
		}
		out[object.SourceKey] = object.Payload.ObjectTypes[0]
	}
	return out
}

// reportLLMRun prints what the model saw, what it decided, and what changed in
// the emitted objects — the whole point of a live run is reading this.
func reportLLMRun(t *testing.T, schemas []schemaplan.ContainerSchema, plan schemaplan.Plan,
	took time.Duration, model string, llmSink, naiveSink *recordingSink, completions []completionInfo) {
	t.Helper()

	t.Logf("model %s planned %d containers in %s", model, len(schemas), took.Round(time.Millisecond))
	for i, completion := range completions {
		note := ""
		if completion.finishReason == "length" {
			note = "  <-- TRUNCATED at the token cap; llmplan sees this as a parse error"
		}
		t.Logf("  completion %d: finish_reason=%q prompt=%d completion=%d tokens%s",
			i+1, completion.finishReason, completion.promptTokens, completion.completionTokens, note)
		// A suspiciously small answer is the interesting case: print it whole
		// so a model that declined the task can be told from one that failed.
		if completion.completionTokens < 1500 && completion.content != "" {
			t.Logf("    raw response: %s", completion.content)
		}
	}
	t.Logf("objects: %d emitted with the plan, %d without (+%d)",
		len(llmSink.objects), len(naiveSink.objects), len(llmSink.objects)-len(naiveSink.objects))
	t.Logf("plan: %d containers typed, %d new types defined", len(plan.Containers), len(plan.NewTypes))

	namesById := map[string]string{}
	for _, schema := range schemas {
		namesById[schema.Id] = schema.Name
	}

	t.Log("--- evidence sent ---")
	for _, schema := range schemas {
		names := make([]string, 0, len(schema.Properties))
		for _, property := range schema.Properties {
			names = append(names, fmt.Sprintf("%s(%s)", property.Name, property.Format))
		}
		samples := ""
		if schema.Samples != nil {
			samples = " | titles: " + strings.Join(schema.Samples.Titles, ", ")
		}
		t.Logf("  %q [%s]: %s%s", schema.Name, schema.Id, strings.Join(names, ", "), samples)
	}

	t.Log("--- new types the model defined ---")
	for _, def := range plan.NewTypes {
		properties := make([]string, 0, len(def.Properties))
		for _, property := range def.Properties {
			properties = append(properties, string(property.Key))
		}
		t.Logf("  %q (key %s, layout %s): %s",
			def.Name, def.Key, def.Layout, strings.Join(properties, ", "))
	}

	t.Log("--- container verdicts ---")
	ids := make([]string, 0, len(plan.Containers))
	for id := range plan.Containers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	bundledHits, customHits := 0, 0
	for _, id := range ids {
		containerPlan := plan.Containers[id]
		if bundle.HasObjectTypeByKey(containerPlan.TypeKey) {
			bundledHits++
		} else if containerPlan.TypeKey != "" {
			customHits++
		}
		t.Logf("  %q [%s] → type %q (%d properties remapped)",
			namesById[id], id, containerPlan.TypeKey, len(containerPlan.Properties))
		keys := make([]string, 0, len(containerPlan.Properties))
		for key := range containerPlan.Properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			property := containerPlan.Properties[key]
			bundled := ""
			if bundle.HasRelation(property.Key) {
				bundled = " [bundled]"
			}
			t.Logf("      %s → %s%s (name %q, format %s)",
				key, property.Key, bundled, property.Name, property.Format)
		}
	}
	t.Logf("typing: %d containers onto bundled types, %d onto model-defined types", bundledHits, customHits)

	// Rejected entries are the sanitizer earning its keep, and the sharpest
	// quality signal on the model: what it proposed that was not trustworthy.
	dropped := issueMessages(llmSink, importv2.IssueLLMPlanEntryDropped)
	t.Logf("--- %d plan entries rejected by the sanitizer ---", len(dropped))
	byReason := map[string]int{}
	for _, message := range dropped {
		byReason[message]++
	}
	reasons := make([]string, 0, len(byReason))
	for reason := range byReason {
		reasons = append(reasons, reason)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if byReason[reasons[i]] != byReason[reasons[j]] {
			return byReason[reasons[i]] > byReason[reasons[j]]
		}
		return reasons[i] < reasons[j]
	})
	for _, reason := range reasons {
		t.Logf("  %2dx %s", byReason[reason], reason)
	}

	// Names the model supplies land verbatim in RelationKeyName, so a sloppy
	// or hostile plan shows up here as prose or control characters.
	t.Log("--- plan-supplied name hygiene ---")
	suspicious := 0
	for _, def := range plan.NewTypes {
		if bad, why := nameLooksWrong(def.Name); bad {
			suspicious++
			t.Logf("  type %s: %s — %q", def.Key, why, def.Name)
		}
	}
	for _, id := range ids {
		for _, key := range sortedKeys(plan.Containers[id].Properties) {
			name := plan.Containers[id].Properties[key].Name
			if bad, why := nameLooksWrong(name); bad {
				suspicious++
				t.Logf("  %q property %s: %s — %q", namesById[id], key, why, name)
			}
		}
	}
	t.Logf("%d plan-supplied names look wrong", suspicious)

	t.Log("--- adopted decisions (report-page issues) ---")
	for _, code := range []importv2.IssueCode{importv2.IssueTypeSuggested, importv2.IssuePropertyMapped} {
		for _, message := range issueMessages(llmSink, code) {
			t.Logf("  [%s] %s", code, message)
		}
	}

	t.Log("--- content objects retyped vs naive ---")
	llmTypes, naiveTypes := objectTypes(llmSink), objectTypes(naiveSink)
	keys := make([]string, 0, len(llmTypes))
	for key := range llmTypes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	retyped, added := 0, 0
	for _, key := range keys {
		naiveType, existed := naiveTypes[key]
		switch {
		case !existed:
			added++
		case naiveType != llmTypes[key]:
			retyped++
			t.Logf("  %s: %s → %s", key, naiveType, llmTypes[key])
		}
	}
	t.Logf("%d content objects retyped, %d plan objects added, %d unchanged",
		retyped, added, len(llmTypes)-retyped-added)

	if retyped == 0 {
		t.Log("NOTE: no emitted object changed type; the model agreed with the built-in rules")
	}
}

// TestDumpRealSchemas writes the workspace's container schemas — the exact
// evidence the planner sees — to a file, replaying the committed cassette and
// calling no LLM. It exists so the naming-quality harness
// (llmplan.TestLiveNamingQuality) can be pointed at real data reproducibly;
// that data used to live in a scratchpad and was lost to a reboot.
//
//	IMPORTV2_DUMP_SCHEMAS=/tmp/schemas.json \
//	  go test ./core/block/importv2/notion/ -run TestDumpRealSchemas -count=1
func TestDumpRealSchemas(t *testing.T) {
	out := os.Getenv("IMPORTV2_DUMP_SCHEMAS")
	if out == "" {
		t.Skip("set IMPORTV2_DUMP_SCHEMAS to write the container schemas")
	}
	cassettePath := os.Getenv("IMPORTV2_LLM_CASSETTE")
	if cassettePath == "" {
		cassettePath = filepath.Join("testdata", "cassettes", "workspace")
	}
	if _, err := os.Stat(cassettePath + ".yaml"); err != nil {
		t.Skipf("no cassette at %s.yaml", cassettePath)
	}

	var schemas []schemaplan.ContainerSchema
	capturing := schemaplan.PlannerFunc(func(_ context.Context, in []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
		schemas = in
		// No plan: the converter degrades to its naive path, which is all this
		// test needs — it only wants the evidence.
		return schemaplan.Plan{}, fmt.Errorf("dump only")
	})
	runNotionConvert(t, "cassette-token", cassettePath, recorder.ModeReplayOnly,
		WithPlanner(capturing), WithContentSamples())

	require.NotEmpty(t, schemas, "no container schemas captured")
	raw, err := json.MarshalIndent(schemas, "", " ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(out, raw, 0o644))
	t.Logf("wrote %d container schemas to %s (%d bytes)", len(schemas), out, len(raw))
}
