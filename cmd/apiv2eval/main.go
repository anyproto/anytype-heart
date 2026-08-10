// Command apiv2eval is a development harness, not a shipped tool: it runs
// small local models through real read→edit loops against a real local
// Anytype API and records what happened. It is the agent-loop runner
// core/api/eval defers ("the agent-loop runner that drives models against a
// scratch space … is intentionally absent here"); that package keeps the
// Phase-0 scoring primitives, and the task ids here follow its names where
// the tasks correspond. Token counts come from the model host's own usage
// numbers rather than eval.CountTokens' 4-bytes-per-token approximation.
//
// The question it exists to answer is the one no review round can: does a
// small model complete the loop, or walk into a 400 it cannot get out of.
// Every cell of (model × surface × task) runs more than once — small models
// are high-variance and one sample is not a rate — and every attempt is
// judged by asking the API what the document says afterwards, never by the
// model's own account of what it did.
//
//	apiv2eval -n 3                          # the whole matrix
//	apiv2eval -models gemma4:e2b -arms ops  # one cell
//	apiv2eval -list                         # print the matrix and exit
//
// Configuration comes from the repo's .env: ANYTYPE_API_URL, ANYTYPE_API_KEY
// and OLLAMA_BASE_URL (the OpenAI-compatible model endpoint). The API must
// be running; the harness refuses to start otherwise, naming which of the
// two failures it hit — server down, or key rejected.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// options are the harness flags.
type options struct {
	envFile     string
	models      string
	arms        string
	taskFilter  string
	n           int
	maxTurns    int
	temperature float64
	modelTimout time.Duration
	spaceId     string
	spaceName   string
	outDir      string
	list        bool
	probe       bool
	// exampleShape selects which example the probe pairs with each schema
	// (probe.go): "published" — byte-for-byte as GET /v2/schemas/ops/{op}
	// serves it — or "op", that example unwrapped to the single op inside.
	exampleShape string
	// constAsEnum is the probe's one diagnostic deviation from a served
	// schema — see rewriteConstAsEnum.
	constAsEnum bool
}

func run() error {
	var opt options
	flag.StringVar(&opt.envFile, "env", ".env", "file holding ANYTYPE_API_URL / ANYTYPE_API_KEY / OLLAMA_BASE_URL")
	flag.StringVar(&opt.models, "models", "gemma4:e2b,gemma4:e4b", "comma-separated model ids to evaluate")
	flag.StringVar(&opt.arms, "arms", "wrapper/small,wrapper/large,ops", "comma-separated surfaces: wrapper/small, wrapper/large, ops")
	flag.StringVar(&opt.taskFilter, "tasks", "", "comma-separated task ids (default: all)")
	flag.IntVar(&opt.n, "n", 3, "attempts per cell — report this number, never present one sample as a rate")
	flag.IntVar(&opt.maxTurns, "max-turns", 8, "turn budget per attempt")
	flag.Float64Var(&opt.temperature, "temperature", 0, "sampling temperature")
	flag.DurationVar(&opt.modelTimout, "model-timeout", 5*time.Minute, "per-completion timeout")
	flag.StringVar(&opt.spaceId, "space", "", "space to work in (default: reuse or create the eval space)")
	flag.StringVar(&opt.spaceName, "space-name", "APIv2 eval", "name of the eval space when one must be created")
	flag.StringVar(&opt.outDir, "out", "eval-out", "output directory for attempts.jsonl and summary.txt")
	flag.BoolVar(&opt.list, "list", false, "print the run matrix and exit")
	flag.BoolVar(&opt.probe, "probe", false, "run the one-turn schema-emission probe instead of the loop (needs no live API)")
	flag.StringVar(&opt.exampleShape, "probe-example", exampleAsPublished,
		"probe only: which example to pair with each op schema — published (a whole PATCH body, as served) or op (unwrapped to one op)")
	flag.BoolVar(&opt.constAsEnum, "probe-const-as-enum", false,
		"probe only, diagnostic: spell the op discriminator as a single-value enum instead of const (the default is the schema as served)")
	flag.Parse()

	env, err := loadEnv(opt.envFile)
	if err != nil {
		return err
	}
	apiURL := firstNonEmpty(env["ANYTYPE_API_URL"], os.Getenv("ANYTYPE_API_URL"), wrapper.DefaultBaseURL)
	apiKey := firstNonEmpty(env["ANYTYPE_API_KEY"], os.Getenv("ANYTYPE_API_KEY"))
	modelURL := firstNonEmpty(env["OLLAMA_BASE_URL"], os.Getenv("OLLAMA_BASE_URL"), "http://127.0.0.1:11434/v1")
	modelKey := firstNonEmpty(env["OPENAI_API_KEY"], os.Getenv("OPENAI_API_KEY"), "ollama")

	arms, err := parseArms(opt.arms)
	if err != nil {
		return err
	}
	selected, err := selectTasks(opt.taskFilter)
	if err != nil {
		return err
	}
	models := splitList(opt.models)
	if len(models) == 0 {
		return fmt.Errorf("no models selected")
	}

	if opt.list {
		printMatrix(os.Stdout, models, arms, selected, opt.n)
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if opt.probe {
		if opt.exampleShape != exampleAsPublished && opt.exampleShape != exampleAtOpLevel {
			return fmt.Errorf("unknown -probe-example %q — shapes: %s, %s", opt.exampleShape, exampleAsPublished, exampleAtOpLevel)
		}
		// the probe asks only what the model WRITES given a published schema,
		// so it skips the API preflight entirely
		chat := newChatClient(modelURL, modelKey, opt.modelTimout)
		if err := checkModels(ctx, chat, modelURL, models); err != nil {
			return err
		}
		return runProbe(ctx, chat, models, opt)
	}

	rec := &recorder{}
	transport := &recordingTransport{base: http.DefaultTransport, rec: rec}
	api := newAPIClient(apiURL, apiKey, transport)
	chat := newChatClient(modelURL, modelKey, opt.modelTimout)

	// preflight — both halves fail fast, and they fail differently
	if apiKey == "" {
		return fmt.Errorf("no ANYTYPE_API_KEY in %s or the environment — the API refuses every call without one", opt.envFile)
	}
	if err := api.whoami(ctx); err != nil {
		var ae *apiError
		if errors.As(err, &ae) && ae.Status == http.StatusUnauthorized {
			return fmt.Errorf("the Anytype API at %s rejected the key — create a fresh one in the app (Settings → API keys) and update %s: %w", apiURL, opt.envFile, err)
		}
		return fmt.Errorf("the Anytype API at %s is not answering — start the app (or the build under test) and retry; nothing was run: %w", apiURL, err)
	}
	if err := checkModels(ctx, chat, modelURL, models); err != nil {
		return err
	}
	spaceId, err := resolveSpace(ctx, api, opt)
	if err != nil {
		return err
	}
	rec.take() // preflight exchanges belong to no attempt

	if err := os.MkdirAll(opt.outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	jsonlPath := filepath.Join(opt.outDir, "attempts.jsonl")
	file, err := os.OpenFile(jsonlPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open attempts file: %w", err)
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	defer writer.Flush()

	runId := time.Now().UTC().Format("20060102-150405")
	fmt.Printf("run %s — space %s, %d models × %d arms × %d tasks × n=%d\n",
		runId, spaceId, len(models), len(arms), len(selected), opt.n)

	var attempts []attemptRecord
	for _, model := range models {
		// one model at a time: a shared host reloads weights on every switch
		for seq := 1; seq <= opt.n; seq++ {
			for _, arm := range arms {
				for _, t := range selected {
					if !t.runsOnArm(arm.surface) || (arm.surface == surfaceWrapper && !t.runsOnTier(arm.tier)) {
						continue
					}
					if ctx.Err() != nil {
						fmt.Println("interrupted — writing what ran")
						return finish(writer, attempts, opt, runId)
					}
					rec.take()
					att := runAttempt(ctx, attemptDeps{
						api: api, chat: chat, rec: rec, opt: opt,
						runId: runId, spaceId: spaceId,
					}, model, arm, t, seq)
					attempts = append(attempts, att)
					line, err := json.Marshal(att)
					if err != nil {
						return fmt.Errorf("encode attempt record: %w", err)
					}
					if _, err := writer.Write(append(line, '\n')); err != nil {
						return fmt.Errorf("write attempt record: %w", err)
					}
					writer.Flush()
					fmt.Printf("  %-12s %-15s %-15s #%d → %-11s %2d turns %6d tok  %s\n",
						model, arm.name, t.Id, seq, att.Outcome, att.Turns,
						att.PromptTokens+att.CompletionTokens, firstLine(att.CheckDetail))
				}
			}
		}
	}
	return finish(writer, attempts, opt, runId)
}

func finish(writer *bufio.Writer, attempts []attemptRecord, opt options, runId string) error {
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush attempts file: %w", err)
	}
	summary := buildSummary(runId, attempts, opt)
	path := filepath.Join(opt.outDir, "summary.txt")
	if err := os.WriteFile(path, []byte(summary), 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	fmt.Println()
	fmt.Print(summary)
	fmt.Printf("\nrecords: %s\n", filepath.Join(opt.outDir, "attempts.jsonl"))
	return nil
}

//
// ---- one attempt ----
//

const (
	surfaceWrapper = "wrapper"
	surfaceOps     = "ops"
)

// fixtureIndexTimeout bounds the wait for a fresh fixture to become
// searchable; spaceReadyTimeout bounds the wait for a freshly created eval
// space to become readable.
const (
	fixtureIndexTimeout = 30 * time.Second
	spaceReadyTimeout   = 60 * time.Second
)

// armSpec is one surface under test.
type armSpec struct {
	name    string
	surface string
	tier    wrapper.Tier
}

// attemptRecord is one (model, arm, task, seq) run, as written to JSONL.
type attemptRecord struct {
	Run        string    `json:"run"`
	StartedAt  time.Time `json:"started_at"`
	DurationMs int64     `json:"duration_ms"`
	Model      string    `json:"model"`
	Arm        string    `json:"arm"`
	Surface    string    `json:"surface"`
	Tier       string    `json:"tier,omitempty"`
	Task       string    `json:"task"`
	Seq        int       `json:"seq"`
	SpaceId    string    `json:"space_id"`
	ObjectId   string    `json:"object_id,omitempty"`
	Title      string    `json:"title,omitempty"`
	System     string    `json:"system_prompt"`
	Prompt     string    `json:"prompt"`
	// Outcome is success | failure | environment. An environment outcome is
	// the harness's or the host's fault and never enters a success rate.
	Outcome     string `json:"outcome"`
	CheckDetail string `json:"check_detail,omitempty"`
	EnvError    string `json:"env_error,omitempty"`
	Turns       int    `json:"turns"`
	StoppedBy   string `json:"stopped_by,omitempty"`
	ToolCalls   int    `json:"tool_calls"`
	FailedCalls int    `json:"failed_calls"`
	// FixtureIndexMs is how long the fixture took to become searchable.
	FixtureIndexMs int64 `json:"fixture_index_ms,omitempty"`
	// WrongTargetWrites counts mutations that landed on some OTHER object.
	// Fixtures share a space and a title stem, so a find that returns
	// several and a handle picked from the wrong row writes to a real note
	// that is not the one under test — a failure whose cause is the
	// reference channel, not the edit.
	WrongTargetWrites int         `json:"wrong_target_writes,omitempty"`
	PromptTokens      int         `json:"prompt_tokens"`
	CompletionTokens  int         `json:"completion_tokens"`
	Signals           signals     `json:"signals"`
	Transcript        *transcript `json:"transcript,omitempty"`
}

const (
	outcomeSuccess = "success"
	outcomeFailure = "failure"
	outcomeEnv     = "environment"
)

type attemptDeps struct {
	api   *apiClient
	chat  *chatClient
	rec   *recorder
	opt   options
	runId string

	spaceId string
}

func runAttempt(ctx context.Context, deps attemptDeps, model string, arm armSpec, t task, seq int) attemptRecord {
	started := time.Now()
	att := attemptRecord{
		Run: deps.runId, StartedAt: started, Model: model, Arm: arm.name,
		Surface: arm.surface, Tier: string(arm.tier), Task: t.Id, Seq: seq,
		SpaceId: deps.spaceId,
	}
	finishRecord := func() attemptRecord {
		att.DurationMs = time.Since(started).Milliseconds()
		return att
	}

	fx, err := setupFixture(ctx, deps.api, deps.spaceId, t)
	if err != nil {
		att.Outcome, att.EnvError = outcomeEnv, err.Error()
		return finishRecord()
	}
	att.ObjectId, att.Title = fx.ObjectId, fx.Title

	// the wrapper arm reaches the object through find, which searches; the
	// index is asynchronous, so an attempt started too early fails for a
	// reason that is neither the model's nor the API's
	if arm.surface == surfaceWrapper {
		ok, took, err := deps.api.waitSearchable(ctx, deps.spaceId, fx.Title, fx.ObjectId, fixtureIndexTimeout)
		att.FixtureIndexMs = took.Milliseconds()
		switch {
		case err != nil:
			att.Outcome, att.EnvError = outcomeEnv, fmt.Errorf("wait for the fixture to be searchable: %w", err).Error()
			return finishRecord()
		case !ok:
			att.Outcome = outcomeEnv
			att.EnvError = fmt.Sprintf("the fixture %q was still not searchable after %s — the full-text index had not caught up", fx.Title, fixtureIndexTimeout)
			return finishRecord()
		}
	}

	ts, err := buildToolset(ctx, deps, arm, fx)
	if err != nil {
		att.Outcome, att.EnvError = outcomeEnv, err.Error()
		return finishRecord()
	}
	defer ts.close()

	att.System = ts.instructions() + "\n\n" + armPreamble(arm, deps.spaceId)
	att.Prompt = t.Prompt(fx)

	tr, err := runAgent(ctx, agentConfig{
		chat: deps.chat, model: model, temperature: deps.opt.temperature,
		maxTurns: deps.opt.maxTurns, rec: deps.rec,
	}, ts, att.System, att.Prompt)
	if tr != nil {
		att.Transcript = tr
		att.Turns = len(tr.Turns)
		att.StoppedBy = tr.StoppedBy
		att.ToolCalls = len(tr.Calls)
		for _, c := range tr.Calls {
			if c.IsError {
				att.FailedCalls++
			}
		}
		att.PromptTokens, att.CompletionTokens = tr.PromptTokens, tr.CompletionTokens
		att.Signals = analyze(tr.Calls)
		att.WrongTargetWrites = countWrongTargetWrites(tr.Calls, fx.ObjectId)
	}
	if err != nil {
		// a model timeout or a host that went away is not a task failure
		att.Outcome, att.EnvError = outcomeEnv, err.Error()
		return finishRecord()
	}

	doc, _, err := deps.api.getDocument(ctx, deps.spaceId, fx.ObjectId)
	if err != nil {
		att.Outcome, att.EnvError = outcomeEnv, fmt.Errorf("check read: %w", err).Error()
		return finishRecord()
	}
	verdict := t.Check(doc, fx)
	if verdict.OK {
		att.Outcome = outcomeSuccess
	} else {
		att.Outcome = outcomeFailure
		att.CheckDetail = verdict.Detail
	}
	return finishRecord()
}

// countWrongTargetWrites counts successful mutations addressed at an object
// other than the fixture.
func countWrongTargetWrites(calls []callRecord, objectId string) int {
	n := 0
	for _, c := range calls {
		for _, ex := range c.Exchanges {
			if ex.Method == http.MethodGet || ex.Status < 200 || ex.Status > 299 {
				continue
			}
			if !strings.Contains(ex.Path, "/objects/") || strings.HasSuffix(ex.Path, "/objects") {
				continue
			}
			if !strings.HasSuffix(ex.Path, "/objects/"+objectId) {
				n++
			}
		}
	}
	return n
}

// buildToolset constructs the arm's surface for one attempt. Both are built
// fresh per attempt: the wrapper's session (handles, the idempotency reuse
// record) must not leak between attempts.
func buildToolset(ctx context.Context, deps attemptDeps, arm armSpec, fx *fixture) (toolset, error) {
	switch arm.surface {
	case surfaceWrapper:
		client := wrapper.NewClient(deps.api.baseURL, deps.api.apiKey)
		client.HTTP = &http.Client{Timeout: 60 * time.Second, Transport: deps.api.http.Transport}
		runner := wrapper.NewRunner(client, wrapper.NewMemoryStore())
		ts, err := newMCPToolset(ctx, runner, arm.tier)
		if err != nil {
			return nil, fmt.Errorf("build wrapper toolset: %w", err)
		}
		return ts, nil
	case surfaceOps:
		ts, err := newOpsToolset(ctx, deps.api, deps.spaceId, fx.ObjectId)
		if err != nil {
			return nil, fmt.Errorf("build ops toolset: %w", err)
		}
		return ts, nil
	default:
		return nil, fmt.Errorf("unknown surface %q", arm.surface)
	}
}

// armPreamble is the small amount of context a host would supply: which
// space the work happens in (wrapper arm), or that the object is already
// selected (ops arm). Everything else the model is told comes from the
// product's own instructions.
func armPreamble(arm armSpec, spaceId string) string {
	if arm.surface == surfaceOps {
		return "The object named in the request is the one your tools already act on. " +
			"When the work is done, reply with one short sentence and no tool call."
	}
	return "Work in the Anytype space " + spaceId + ". " +
		"When the work is done, reply with one short sentence and no tool call."
}

//
// ---- selection, preflight, small helpers ----
//

// checkModels fails fast when the model endpoint is down or does not serve
// a requested model — a missing model would otherwise fail every attempt
// one at a time, an hour into a run.
func checkModels(ctx context.Context, chat *chatClient, modelURL string, models []string) error {
	served, err := chat.listModels(ctx)
	if err != nil {
		return fmt.Errorf("the model endpoint at %s is not answering; nothing was run: %w", modelURL, err)
	}
	for _, m := range models {
		if !containsString(served, m) {
			return fmt.Errorf("model %q is not served by %s — it has: %s", m, modelURL, strings.Join(served, ", "))
		}
	}
	return nil
}

func parseArms(spec string) ([]armSpec, error) {
	var out []armSpec
	for _, name := range splitList(spec) {
		switch name {
		case "ops":
			out = append(out, armSpec{name: "ops", surface: surfaceOps})
		case "wrapper/small":
			out = append(out, armSpec{name: name, surface: surfaceWrapper, tier: wrapper.TierSmall})
		case "wrapper/large":
			out = append(out, armSpec{name: name, surface: surfaceWrapper, tier: wrapper.TierLarge})
		default:
			return nil, fmt.Errorf("unknown arm %q — arms: wrapper/small, wrapper/large, ops", name)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no arms selected")
	}
	return out, nil
}

func selectTasks(filter string) ([]task, error) {
	all := tasks()
	if filter == "" {
		return all, nil
	}
	wanted := splitList(filter)
	var out []task
	for _, want := range wanted {
		found := false
		for _, t := range all {
			if t.Id == want {
				out = append(out, t)
				found = true
			}
		}
		if !found {
			var ids []string
			for _, t := range all {
				ids = append(ids, t.Id)
			}
			return nil, fmt.Errorf("unknown task %q — tasks: %s", want, strings.Join(ids, ", "))
		}
	}
	return out, nil
}

// resolveSpace picks the space fixtures are created in: the flag, else an
// existing space with the eval name, else a fresh one. A dedicated space
// keeps every run's fixtures out of the user's real notes — the API has no
// object delete, so fixtures accumulate and must accumulate somewhere
// harmless.
func resolveSpace(ctx context.Context, api *apiClient, opt options) (string, error) {
	if opt.spaceId != "" {
		return opt.spaceId, nil
	}
	spaces, err := api.listSpaces(ctx)
	if err != nil {
		return "", err
	}
	for _, s := range spaces {
		if s.Name == opt.spaceName {
			return s.Id, nil
		}
	}
	id, err := api.createSpace(ctx, opt.spaceName)
	if err != nil {
		return "", fmt.Errorf("create the eval space (pass -space to use an existing one): %w", err)
	}
	// a just-created space is not necessarily loaded yet; without this every
	// attempt of the run would fail at fixture creation and be recorded as an
	// environment failure, which is true but useless
	deadline := time.Now().Add(spaceReadyTimeout)
	for {
		if _, err := api.call(ctx, http.MethodGet, "/v2/spaces/"+url.PathEscape(id), nil, nil, nil); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("the eval space %s was created but not readable after %s", id, spaceReadyTimeout)
		}
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	fmt.Printf("created eval space %q (%s)\n", opt.spaceName, id)
	return id, nil
}

func printMatrix(w *os.File, models []string, arms []armSpec, selected []task, n int) {
	total := 0
	for _, m := range models {
		for _, a := range arms {
			for _, t := range selected {
				if !t.runsOnArm(a.surface) || (a.surface == surfaceWrapper && !t.runsOnTier(a.tier)) {
					continue
				}
				fmt.Fprintf(w, "%-14s %-14s %-16s ×%d\n", m, a.name, t.Id, n)
				total += n
			}
		}
	}
	fmt.Fprintf(w, "\n%d attempts\n", total)
}

// loadEnv reads a KEY=VALUE file. Values are never printed: the file holds
// the API key and nothing in this harness may put it in an output.
func loadEnv(path string) (map[string]string, error) {
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, fmt.Errorf("read env file %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		out[strings.TrimSpace(key)] = value
	}
	return out, nil
}

func newNonce(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
	}
	return hex.EncodeToString(b)
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
