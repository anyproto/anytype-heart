// Command apiv2eval is a development harness, not a shipped tool: it runs
// small local models through real read→edit loops against a real local
// Anytype API and records what happened. Token counts come from the model
// host's own usage numbers.
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
//
// -fresh-account replaces the first two: the harness starts its own headless
// heart on a throwaway account (see cmd/apiv2eval/heartboot) and tears it
// down afterwards. Prefer it for anything whose numbers get compared. A
// shared desktop account accumulates the spaces every past run created, and
// a model asked to find "the" space starts naming the wrong one — that alone
// accounted for every failure in one 42-attempt baseline, and it makes runs
// incomparable rather than merely worse.
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
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anyproto/anytype-heart/cmd/apiv2eval/heartboot"
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
	envFile         string
	models          string
	arms            string
	taskFilter      string
	n               int
	maxTurns        int
	temperature     float64
	modelTimout     time.Duration
	modelRetry      time.Duration
	replayReasoning bool
	topP            float64
	topK            int
	presencePenalty float64
	thinking        string
	captureRaw      bool
	salvage         bool
	systemSuffix    string
	spaceId         string
	spaceName       string
	outDir          string
	freshAccount    bool
	keepAccount     bool
	heartBinary     string
	list            bool
	probe           bool
	// constAsEnum is the probe's one diagnostic deviation from a served
	// schema — see rewriteConstAsEnum.
	constAsEnum bool
}

func run() error {
	var opt options
	flag.StringVar(&opt.envFile, "env", ".env", "file holding ANYTYPE_API_URL / ANYTYPE_API_KEY / OLLAMA_BASE_URL")
	flag.StringVar(&opt.models, "models", "gemma4:e2b,gemma4:e4b", "comma-separated model ids to evaluate")
	flag.StringVar(&opt.arms, "arms", strings.Join(defaultArms, ","),
		"comma-separated surfaces: "+strings.Join(allArms, ", "))
	flag.StringVar(&opt.taskFilter, "tasks", "", "comma-separated task ids (default: all)")
	flag.IntVar(&opt.n, "n", 3, "attempts per cell — report this number, never present one sample as a rate")
	flag.IntVar(&opt.maxTurns, "max-turns", 8, "turn budget per attempt")
	flag.Float64Var(&opt.temperature, "temperature", 0, "sampling temperature")
	flag.DurationVar(&opt.modelTimout, "model-timeout", 5*time.Minute, "per-completion timeout")
	flag.DurationVar(&opt.modelRetry, "model-retry-budget", 20*time.Minute,
		"how long one completion retries an endpoint that cannot answer — a laptop serving the model sleeps, and without this the outage is recorded as a failed cell")
	flag.StringVar(&opt.spaceId, "space", "", "space to work in (default: reuse or create the eval space)")
	flag.StringVar(&opt.spaceName, "space-name", "APIv2 eval", "name of the eval space when one must be created")
	flag.StringVar(&opt.outDir, "out", "eval-out", "output directory for attempts.jsonl and summary.txt")
	flag.Float64Var(&opt.topP, "top-p", 0, "nucleus sampling (0 = server default)")
	flag.IntVar(&opt.topK, "top-k", 0, "top-k sampling (0 = server default)")
	flag.Float64Var(&opt.presencePenalty, "presence-penalty", 0, "presence penalty (0 = server default)")
	flag.StringVar(&opt.thinking, "thinking", "", `chat_template_kwargs.enable_thinking: "on", "off", or "" to leave the server default`)
	flag.BoolVar(&opt.captureRaw, "capture-raw", false,
		"record the raw choices[0] of every turn — tells a model that said nothing from a tool call the host dropped")
	flag.StringVar(&opt.systemSuffix, "system-suffix", "",
		"text appended to the system prompt — the A/B knob for steering that is NOT part of the published surface (e.g. a rule about which channel tool calls belong in)")
	flag.BoolVar(&opt.salvage, "salvage-tool-calls", false,
		"read back tool calls the host failed to parse — measured: LM Studio's MLX path drops well-formed calls emitted inside the thinking channel")
	flag.BoolVar(&opt.replayReasoning, "replay-reasoning", false,
		"feed each turn's reasoning back into the next turn — the A/B for whether a model's own thinking helps or breaks the loop")
	flag.BoolVar(&opt.freshAccount, "fresh-account", false,
		"start a headless heart on a throwaway account instead of using ANYTYPE_API_URL/ANYTYPE_API_KEY — the only way one run's spaces cannot confuse the next one's model")
	flag.BoolVar(&opt.keepAccount, "keep-account", false,
		"-fresh-account only: leave the temp data dir (and the heart log) behind — a failed run is unreadable without the account it failed against")
	flag.StringVar(&opt.heartBinary, "heart-binary", "",
		"-fresh-account only: a prebuilt cmd/grpcserver to run (default: build one from this tree, so the run measures the tree it is in)")
	flag.BoolVar(&opt.list, "list", false, "print the run matrix and exit")
	flag.BoolVar(&opt.probe, "probe", false, "run the one-turn schema-emission probe instead of the loop (needs no live API)")
	flag.BoolVar(&opt.constAsEnum, "probe-const-as-enum", false,
		"probe only, diagnostic: spell the op discriminator as a single-value enum instead of const (the default is the schema as served)")
	flag.Parse()

	env, err := loadEnv(opt.envFile)
	if err != nil {
		return err
	}
	// The process environment wins over the env FILE, not the other way round:
	// the file is the default, an explicit `VAR=… apiv2eval …` is the override.
	// (Reversed, a stale host in .env silently beat the command line — which is
	// how a run went to a LAN address after the host moved to Tailscale.)
	apiURL := firstNonEmpty(os.Getenv("ANYTYPE_API_URL"), env["ANYTYPE_API_URL"], wrapper.DefaultBaseURL)
	apiKey := firstNonEmpty(os.Getenv("ANYTYPE_API_KEY"), env["ANYTYPE_API_KEY"])
	modelURL := firstNonEmpty(os.Getenv("OLLAMA_BASE_URL"), env["OLLAMA_BASE_URL"], "http://127.0.0.1:11434/v1")
	modelKey := firstNonEmpty(os.Getenv("OPENAI_API_KEY"), env["OPENAI_API_KEY"], "ollama")

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
	if err := checkTaskGating(); err != nil {
		return err
	}
	cells, skipped, err := planCells(models, arms, selected)
	if err != nil {
		return err
	}

	if opt.list {
		printMatrix(os.Stdout, models, arms, selected, cells, skipped, opt.n)
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if opt.probe {
		// the probe asks only what the model WRITES given a published schema,
		// so it skips the API preflight entirely
		chat := newChatClient(modelURL, modelKey, opt.modelTimout)
		chat.retryBudget = opt.modelRetry
		applySampling(chat, opt)
		if err := checkModels(ctx, chat, modelURL, models); err != nil {
			return err
		}
		return runProbe(ctx, chat, models, opt)
	}

	if opt.freshAccount {
		heart, err := startFreshAccount(ctx, opt)
		if err != nil {
			return err
		}
		defer func() {
			if err := heart.Stop(); err != nil {
				fmt.Fprintln(os.Stderr, "warning: tearing down the eval heart:", err)
			}
		}()
		apiURL, apiKey = heart.APIURL, heart.APIKey
	}

	rec := &recorder{}
	transport := &recordingTransport{base: http.DefaultTransport, rec: rec}
	api := newAPIClient(apiURL, apiKey, transport)
	chat := newChatClient(modelURL, modelKey, opt.modelTimout)
	chat.retryBudget = opt.modelRetry
	applySampling(chat, opt)

	// preflight — both halves fail fast, and they fail differently
	if apiKey == "" {
		return fmt.Errorf("no ANYTYPE_API_KEY in %s or the environment — the API refuses every call without one", opt.envFile)
	}
	if err := api.whoami(ctx); err != nil {
		if opt.freshAccount {
			// heartboot already proved this URL and key work, so a failure
			// here means the heart died between bootstrap and now.
			return fmt.Errorf("the freshly bootstrapped API at %s stopped answering before the run started: %w", apiURL, err)
		}
		var ae *apiError
		if errors.As(err, &ae) && ae.Status == http.StatusUnauthorized {
			return fmt.Errorf("the Anytype API at %s rejected the key — create a fresh one in the app (Settings → API keys) and update %s, or run with -fresh-account: %w", apiURL, opt.envFile, err)
		}
		return fmt.Errorf("the Anytype API at %s is not answering — start the app (or the build under test), or run with -fresh-account; nothing was run: %w", apiURL, err)
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
	fmt.Printf("run %s — space %s, %d models × %d arms × %d tasks × n=%d = %d attempts\n",
		runId, spaceId, len(models), len(arms), len(selected), opt.n, len(cells)*opt.n)
	printSkipped(os.Stdout, skipped)

	var attempts []attemptRecord
	for _, model := range models {
		// one model at a time: a shared host reloads weights on every switch
		for seq := 1; seq <= opt.n; seq++ {
			for _, arm := range arms {
				for _, t := range selected {
					if !cells[cellKey{model, arm.name, t.Id}] {
						continue
					}
					if ctx.Err() != nil {
						fmt.Println("interrupted — writing what ran")
						return finish(writer, attempts, skipped, opt, runId)
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
	return finish(writer, attempts, skipped, opt, runId)
}

func finish(writer *bufio.Writer, attempts []attemptRecord, skipped []skippedCell, opt options, runId string) error {
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush attempts file: %w", err)
	}
	summary := buildSummary(runId, attempts, skipped, opt)
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

// Arm names.
const (
	armWrapperSmall = "wrapper/small"
	armWrapperLarge = "wrapper/large"
	armOps          = "ops"
)

// defaultArms is the matrix; allArms is everything -arms accepts.
var (
	defaultArms = []string{armWrapperSmall, armWrapperLarge, armOps}
	allArms     = []string{armWrapperSmall, armWrapperLarge, armOps}
)

// armSpec is one surface under test.
type armSpec struct {
	name    string
	surface string
	tier    wrapper.Tier
}

// publishedTools returns the tool names this arm serves the model, read from
// the same table the arm publishes from: the tier filter for the wrapper,
// the op list for the ops arm. The capability gate asks THIS rather than a
// hand-kept note of which task runs where, so removing a tool from a tier
// updates the gate with it.
func (a armSpec) publishedTools() []string {
	switch a.surface {
	case surfaceWrapper:
		return wrapper.ToolNamesForTier(a.tier)
	case surfaceOps:
		return append([]string{"read_object"}, opsArmOps...)
	default:
		return nil
	}
}

// skippedCell is one (model, arm, task) the run did not measure, with the
// reason. Skips are reported, never silent: a cell that quietly disappears
// reads as "not applicable" when it may mean "we stopped measuring this".
type skippedCell struct {
	Model  string
	Arm    string
	Task   string
	Reason string
}

// planCells derives which cells a run measures. A cell is skipped when the
// arm's published tool set has no tool for a capability the task requires —
// asking a model to fill a table cell with no set_cell in its tier is not a
// measurement of anything, and scoring the answer as a failure moves the
// headline rate by the number of such cells.
func planCells(models []string, arms []armSpec, selected []task) (map[cellKey]bool, []skippedCell, error) {
	cells := map[cellKey]bool{}
	var skipped []skippedCell
	for _, model := range models {
		for _, arm := range arms {
			published := map[string]bool{}
			for _, name := range arm.publishedTools() {
				published[name] = true
			}
			for _, t := range selected {
				key := cellKey{model, arm.name, t.Id}
				if !t.runsOnArm(arm.surface) {
					skipped = append(skipped, skippedCell{model, arm.name, t.Id,
						fmt.Sprintf("the task does not run on the %s surface", arm.surface)})
					continue
				}
				missing := ""
				for _, c := range t.Requires {
					if surfaceCannotExpress(c, arm.surface) {
						missing = fmt.Sprintf("the %s surface cannot %s at all", arm.surface, c)
						break
					}
					tool, err := capabilityTool(c, arm.surface)
					if err != nil {
						return nil, nil, fmt.Errorf("gate %s on %s: %w", t.Id, arm.name, err)
					}
					if !published[tool] {
						missing = fmt.Sprintf("%s publishes no %s — the task cannot %s without it",
							arm.name, tool, c)
						break
					}
				}
				if missing != "" {
					skipped = append(skipped, skippedCell{model, arm.name, t.Id, missing})
					continue
				}
				cells[key] = true
			}
		}
	}
	return cells, skipped, nil
}

// printSkipped writes the skipped cells to a stream, or says there are none.
func printSkipped(w io.Writer, skipped []skippedCell) {
	if len(skipped) == 0 {
		fmt.Fprintln(w, "no cells skipped — every arm publishes a tool for every capability its tasks require")
		return
	}
	fmt.Fprintf(w, "%d cells skipped:\n", len(skipped))
	for _, s := range skipped {
		fmt.Fprintf(w, "  %-14s %-14s %-16s — %s\n", s.Model, s.Arm, s.Task, s.Reason)
	}
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
		Surface: arm.surface, Tier: string(arm.tier),
		Task: t.Id, Seq: seq, SpaceId: deps.spaceId,
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
		// EVERY seeded object has to be searchable, not just the fixture:
		// a task whose difficulty IS the ambiguity (three pages sharing a
		// stem) silently becomes trivial if the siblings have not indexed —
		// find returns one match, the model cannot pick wrong, and the
		// attempt passes for a reason that has nothing to do with the model.
		for _, sib := range t.Siblings {
			sibTitle, sibId := fx.Extra[sib.Key], fx.Extra[sib.Key+"Id"]
			if sibId == "" {
				continue
			}
			sibOK, _, sibErr := deps.api.waitSearchable(ctx, deps.spaceId, sibTitle, sibId, fixtureIndexTimeout)
			if sibErr != nil {
				att.Outcome, att.EnvError = outcomeEnv, fmt.Errorf("wait for sibling %q: %w", sib.Key, sibErr).Error()
				return finishRecord()
			}
			if !sibOK {
				att.Outcome = outcomeEnv
				att.EnvError = fmt.Sprintf("sibling %q (%s) was not searchable after %s — the ambiguity the task measures would not have existed", sib.Key, sibTitle, fixtureIndexTimeout)
				return finishRecord()
			}
		}
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
	if deps.opt.systemSuffix != "" {
		// the suffix is recorded IN att.System, so a run's own record shows
		// exactly what the model was told — a steer that is not in the
		// transcript is a confound nobody can audit later
		att.System += "\n\n" + deps.opt.systemSuffix
	}
	att.Prompt = t.Prompt(fx)

	tr, err := runAgent(ctx, agentConfig{
		chat: deps.chat, model: model, temperature: deps.opt.temperature,
		maxTurns: deps.opt.maxTurns, rec: deps.rec,
		replayReasoning:  deps.opt.replayReasoning,
		salvageToolCalls: deps.opt.salvage,
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
		att.Signals.SpaceIdAsObject = countSpaceIdAsObject(tr.Calls, deps.spaceId)
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
	if err := readSiblings(ctx, deps.api, deps.spaceId, fx, t); err != nil {
		att.Outcome, att.EnvError = outcomeEnv, err.Error()
		return finishRecord()
	}
	// A task whose product is not the fixture document (a created type, a
	// space's schema) reads its result back from the live API instead, and
	// has no Check at all — calling both would deref a nil one.
	var verdict checkResult
	if t.CheckAPI != nil {
		verdict = t.CheckAPI(ctx, deps.api, deps.spaceId, fx)
	} else {
		verdict = t.Check(doc, fx)
	}
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
//
// The space id names its ARGUMENT here. The first version said only "Work in
// the Anytype space <id>", and a model opened its attempt with
// read {"object": "<that space id>"} — the one id in its context, handed to
// it in a position that named no argument, and `read`'s object takes "a full
// object id". That was the harness's doing, not the surface's: the wrapper
// refused it with "no working session … run find first" and the model
// recovered on the next turn. The sentence below is the product's own — it
// is what the `spaces` tool prints under its listing — so labelling the
// channel adds no steering the model would not have had if it had asked for
// the space itself.
func armPreamble(arm armSpec, spaceId string) string {
	if arm.surface == surfaceOps {
		return "The object named in the request is the one your tools already act on. " +
			"When the work is done, reply with one short sentence and no tool call."
	}
	return "Work in the Anytype space " + spaceId + " — pass that id as space to find, describe and create. " +
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
		small := armSpec{name: name, surface: surfaceWrapper, tier: wrapper.TierSmall}
		switch name {
		case armOps:
			out = append(out, armSpec{name: name, surface: surfaceOps})
		case armWrapperSmall:
			out = append(out, small)
		case armWrapperLarge:
			out = append(out, armSpec{name: name, surface: surfaceWrapper, tier: wrapper.TierLarge})
		default:
			return nil, fmt.Errorf("unknown arm %q — arms: %s", name, strings.Join(allArms, ", "))
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

// startFreshAccount boots a private heart with an empty account and returns
// it ready to use. This is the answer to a measured problem: the shared
// desktop account had eleven spaces, eight of them named like eval spaces,
// and the model under test then picked the wrong one on a third of its
// lookups — every failure in one 42-attempt baseline. A run that cannot see
// another run's spaces is a run whose numbers mean something.
func startFreshAccount(ctx context.Context, opt options) (*heartboot.Heart, error) {
	if opt.spaceId != "" {
		// A space id from another account cannot exist in an account that was
		// created seconds ago; refuse instead of failing later as a 404 that
		// reads like an API bug.
		return nil, fmt.Errorf("-space %s cannot be used with -fresh-account: the account is created empty and has no such space", opt.spaceId)
	}
	fmt.Println("bootstrapping a throwaway account (building the heart if needed)…")
	started := time.Now()
	heart, err := heartboot.Start(ctx, heartboot.Options{
		BinaryPath:  opt.heartBinary,
		KeepDataDir: opt.keepAccount,
		AccountName: "apiv2eval",
		AppName:     "apiv2eval",
	})
	if err != nil {
		return nil, fmt.Errorf("bootstrap a fresh account: %w", err)
	}
	fmt.Printf("fresh account %s ready in %s — api %s\n",
		heart.AccountId, time.Since(started).Round(time.Second), heart.APIURL)
	if opt.keepAccount {
		fmt.Printf("keeping account data at %s (log: %s)\n", heart.DataDir, heart.LogPath)
	}
	return heart, nil
}

// resolveSpace picks the space fixtures are created in: the flag, else an
// existing space with the eval name, else a fresh one. A dedicated space
// keeps every run's fixtures out of the user's real notes. The harness does
// not delete its fixtures, so they accumulate somewhere harmless.
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

func printMatrix(w io.Writer, models []string, arms []armSpec, selected []task, cells map[cellKey]bool, skipped []skippedCell, n int) {
	total := 0
	for _, m := range models {
		for _, a := range arms {
			for _, t := range selected {
				if !cells[cellKey{m, a.name, t.Id}] {
					continue
				}
				fmt.Fprintf(w, "%-14s %-14s %-16s ×%d\n", m, a.name, t.Id, n)
				total += n
			}
		}
	}
	fmt.Fprintf(w, "\n%d attempts\n", total)
	printSkipped(w, skipped)
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

// applySampling copies the decoding knobs onto the client. They are separate
// flags rather than one preset because each is its own A/B: the vendor
// recommends all of them together, but presence_penalty in particular is
// suspect for tool calling, where the output is JSON full of repeated
// punctuation and key names.
func applySampling(chat *chatClient, opt options) {
	chat.topP = opt.topP
	chat.topK = opt.topK
	chat.presencePenalty = opt.presencePenalty
	chat.thinking = opt.thinking
	chat.captureRaw = opt.captureRaw
}
