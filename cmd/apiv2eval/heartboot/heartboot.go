// Package heartboot stands up a headless anytype-heart on throwaway storage:
// it builds (or takes) the cmd/grpcserver binary, starts it, creates a fresh
// local-only account, mints a JSON-API key for that account, and hands back
// the HTTP base URL plus the key. Stop tears the process down and deletes the
// data directory.
//
// It exists because the eval harness used to point at whatever account the
// desktop client happened to have open. That account accumulated eval spaces
// — eleven of them, eight named some variation of "eval tasks" — and a model
// asked to find "the" space then named the wrong one on 24 of 71 calls. Every
// failure in one 42-attempt baseline was that, not the API under test. A run
// that starts from an empty account is comparable to the next run; a run
// against a lived-in account is not. The second reason is mechanical: picking
// up a rebuilt heart used to mean restarting Electron by hand, and a stale
// process once meant the account never opened and the HTTP port never bound —
// a failure that looked exactly like a bad build.
//
// Every step here fails loudly and names itself. A bootstrap that half-worked
// and left the harness talking to someone's real account is worse than one
// that refuses to start: we have already lost runs to that exact confusion.
package heartboot

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	// grpcStartedPrefix is what cmd/grpcserver prints once its listener is up.
	// The address is parsed off this line rather than assumed: the server is
	// started on port 0 so the kernel picks a port no desktop client can be
	// holding.
	grpcStartedPrefix = "gRPC server started at: "

	// heartLogFile collects the heart's stdout+stderr. On failure its tail is
	// quoted into the returned error — a bootstrap that says only "timed out"
	// is not diagnosable.
	heartLogFile = "heart.log"

	// logTailLines is how much of that log an error quotes.
	logTailLines = 40

	// clientPlatform/clientVersion identify this harness to the heart.
	// InitialSetParameters is not optional: AccountCreate panics without a
	// client version (core/application/application.go requireClientWithVersion).
	clientPlatform = "apiv2eval"
	clientVersion  = "0.0.0-apiv2eval"
)

// Options configures Start. The zero value is usable — every field has a
// working default — except that callers who care about reproducibility should
// set AppName, since the key's app name is stamped on every object the eval
// creates.
type Options struct {
	// BinaryPath is a prebuilt cmd/grpcserver binary. Empty means build one
	// from RepoRoot, which is the default because it is the honest one: the
	// harness exists to measure the API in the working tree, and a stale
	// prebuilt binary would silently measure something else. The build is
	// incremental (~20s cold link, less after), so the cost is small next to
	// a run.
	BinaryPath string

	// RepoRoot is the module root used for the build. Empty means walk up
	// from the working directory looking for anytype-heart's go.mod.
	RepoRoot string

	// DataDir holds the wallet, the account and the heart log. Empty means a
	// fresh temp directory, deleted by Stop unless KeepDataDir is set. A
	// directory the caller names is never deleted — Stop removes only what
	// this package created.
	DataDir string

	// KeepDataDir leaves the data directory (and the log) behind. Debugging a
	// failed run needs the account it failed against.
	KeepDataDir bool

	// AccountName is the profile name of the throwaway account.
	AccountName string

	// AppName names the API key. It is recorded as creation provenance on
	// every object the key creates, and the heart refuses a nameless key.
	AppName string

	// Log receives the heart's stdout and stderr as they arrive. Nil means
	// the log file only.
	Log io.Writer

	// Timeouts. Each covers one step so a hang names the step that hung.
	BuildTimeout   time.Duration // go build
	StartTimeout   time.Duration // process start → gRPC banner
	AccountTimeout time.Duration // AccountCreate (starts the whole app)
	APITimeout     time.Duration // key minted → HTTP API answering
	StopTimeout    time.Duration // SIGTERM → exit, before SIGKILL
}

func (o *Options) applyDefaults() {
	if o.AccountName == "" {
		o.AccountName = "apiv2eval"
	}
	if o.AppName == "" {
		o.AppName = "apiv2eval"
	}
	if o.BuildTimeout == 0 {
		o.BuildTimeout = 10 * time.Minute
	}
	if o.StartTimeout == 0 {
		o.StartTimeout = 2 * time.Minute
	}
	if o.AccountTimeout == 0 {
		// AccountCreate builds the whole app container and derives the
		// personal space; on a cold machine this is tens of seconds.
		o.AccountTimeout = 5 * time.Minute
	}
	if o.APITimeout == 0 {
		o.APITimeout = 2 * time.Minute
	}
	if o.StopTimeout == 0 {
		o.StopTimeout = 30 * time.Second
	}
}

// Heart is a running headless heart with a fresh account. Stop it when done.
type Heart struct {
	// APIURL is the base URL of the JSON API, e.g. http://127.0.0.1:52341.
	APIURL string
	// APIKey is a JSON-API-scoped app key for the fresh account — the value
	// the harness otherwise reads from ANYTYPE_API_KEY.
	APIKey string
	// AccountId is the created account's identity.
	AccountId string
	// GRPCAddr is where the heart's gRPC server listens, for ad-hoc probing.
	GRPCAddr string
	// DataDir holds wallet, account storage and heart.log.
	DataDir string
	// LogPath is DataDir/heart.log.
	LogPath string

	conn      *grpc.ClientConn
	logFile   *os.File
	tail      *logTail
	proc      *procWaiter
	dataTemp  bool
	keepData  bool
	stopGrace time.Duration

	stopOnce sync.Once
	stopErr  error
}

// Start brings up the heart and returns once the JSON API has answered an
// authenticated request with the minted key. Any failure before that leaves
// nothing running: the process is killed and (unless KeepDataDir) the data
// directory removed.
func Start(ctx context.Context, opt Options) (h *Heart, err error) {
	opt.applyDefaults()

	// Undo stack: every step that acquires something pushes its release here,
	// so a failure five steps in cannot leak a process or a directory.
	var undo []func()
	defer func() {
		if err == nil {
			return
		}
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
	}()

	dataDir := opt.DataDir
	dataTemp := false
	if dataDir == "" {
		dataDir, err = os.MkdirTemp("", "apiv2eval-heart-")
		if err != nil {
			return nil, fmt.Errorf("create temp data dir: %w", err)
		}
		dataTemp = true
	} else if err = os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir %s: %w", dataDir, err)
	}
	undo = append(undo, func() {
		if dataTemp && !opt.KeepDataDir {
			os.RemoveAll(dataDir)
		}
	})

	binary := opt.BinaryPath
	if binary == "" {
		binary, err = buildServer(ctx, opt)
		if err != nil {
			return nil, err
		}
	} else if _, err = os.Stat(binary); err != nil {
		return nil, fmt.Errorf("heart binary %s is not usable: %w", binary, err)
	}

	// The JSON API's port must be decided here: the heart takes it as a
	// literal listen address in AccountCreate and never reports back what it
	// bound (core/api/service.go logs a bind failure and returns nil), so a
	// collision would surface only as an API that never answers. Asking the
	// kernel for a free one is also what keeps a run off 31009, where a
	// desktop client would already be listening.
	apiPort, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("reserve a port for the JSON API: %w", err)
	}
	apiAddr := fmt.Sprintf("127.0.0.1:%d", apiPort)

	logPath := filepath.Join(dataDir, heartLogFile)
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("create heart log %s: %w", logPath, err)
	}
	undo = append(undo, func() { logFile.Close() })
	tail := &logTail{sink: logFile, echo: opt.Log}

	// Port 0 for both gRPC listeners: cmd/grpcserver resolves them through
	// net.Listen and prints what it got, so there is no port to collide over
	// and none to guess.
	cmd := exec.Command(binary, "127.0.0.1:0", "127.0.0.1:0")
	cmd.Dir = dataDir
	cmd.Env = os.Environ()
	// Own process group: the heart must not take the terminal's Ctrl-C out
	// from under the harness. The harness stops it deliberately in Stop, after
	// it has finished writing results.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("pipe heart stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("pipe heart stderr: %w", err)
	}
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("start heart %s: %w", binary, err)
	}
	// One waiter for the process, ever: Wait may be called only once, and
	// both the startup watch and the teardown need to observe the exit.
	proc := newProcWaiter(cmd, stdout, stderr, tail)
	undo = append(undo, func() { proc.stop(opt.StopTimeout) })

	grpcAddr, err := awaitAddr(ctx, proc, tail, opt.StartTimeout)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("connect to the heart at %s: %w%s", grpcAddr, err, tail.quote())
	}
	undo = append(undo, func() { conn.Close() })
	client := service.NewClientCommandsClient(conn)

	h = &Heart{
		APIURL:    "http://" + apiAddr,
		GRPCAddr:  grpcAddr,
		DataDir:   dataDir,
		LogPath:   logPath,
		conn:      conn,
		logFile:   logFile,
		tail:      tail,
		proc:      proc,
		dataTemp:  dataTemp,
		keepData:  opt.KeepDataDir,
		stopGrace: opt.StopTimeout,
	}

	if err = h.bootstrapAccount(ctx, client, opt, dataDir, apiAddr); err != nil {
		return nil, err
	}
	if err = h.awaitAPI(ctx, opt.APITimeout); err != nil {
		return nil, err
	}
	return h, nil
}

// bootstrapAccount runs the five RPCs that turn a bare heart into an account
// with an API key. The order is load-bearing and each step is checked twice —
// once for the transport error, once for the response's own error field,
// which is where this API reports everything that actually goes wrong.
func (h *Heart) bootstrapAccount(ctx context.Context, client service.ClientCommandsClient, opt Options, dataDir, apiAddr string) error {
	// 1. InitialSetParameters. Not optional and not cosmetic: AccountCreate
	// panics outright if no client version was ever set. Logs are saved (they
	// are the diagnosis when a run misbehaves) and telemetry is not sent —
	// a throwaway account's activity is not product signal.
	initResp, err := client.InitialSetParameters(ctx, &pb.RpcInitialSetParametersRequest{
		Platform:           clientPlatform,
		Version:            clientVersion,
		Workdir:            dataDir,
		LogLevel:           os.Getenv("ANYTYPE_LOG_LEVEL"),
		DoNotSendLogs:      true,
		DoNotSendTelemetry: true,
	})
	if err != nil {
		return fmt.Errorf("InitialSetParameters: %w%s", err, h.tail.quote())
	}
	if e := initResp.GetError(); e != nil && e.Code != pb.RpcInitialSetParametersResponseError_NULL {
		return fmt.Errorf("InitialSetParameters rejected: %s: %s%s", e.Code, e.Description, h.tail.quote())
	}

	// 2. WalletCreate — derives the keys AccountCreate then requires, and
	// fixes the root path all account storage hangs off.
	walletResp, err := client.WalletCreate(ctx, &pb.RpcWalletCreateRequest{RootPath: dataDir})
	if err != nil {
		return fmt.Errorf("WalletCreate in %s: %w%s", dataDir, err, h.tail.quote())
	}
	if e := walletResp.GetError(); e != nil && e.Code != pb.RpcWalletCreateResponseError_NULL {
		return fmt.Errorf("WalletCreate rejected: %s: %s%s", e.Code, e.Description, h.tail.quote())
	}
	mnemonic := walletResp.GetMnemonic()
	if mnemonic == "" {
		return fmt.Errorf("WalletCreate returned no mnemonic — nothing can authenticate against this heart%s", h.tail.quote())
	}

	// 3. AccountCreate. LocalOnly plus DisableLocalNetworkSync is the whole
	// isolation story: the eval account must never reach the production
	// network, and must not answer mDNS on the LAN either. JsonApiListenAddr
	// brings the HTTP API up with the account, so there is no second call and
	// no window where the account exists but the API does not.
	accCtx, cancel := context.WithTimeout(ctx, opt.AccountTimeout)
	defer cancel()
	accResp, err := client.AccountCreate(accCtx, &pb.RpcAccountCreateRequest{
		Name:                    opt.AccountName,
		StorePath:               dataDir,
		NetworkMode:             pb.RpcAccount_LocalOnly,
		DisableLocalNetworkSync: true,
		JsonApiListenAddr:       apiAddr,
	})
	if err != nil {
		return fmt.Errorf("AccountCreate (after %s): %w%s", opt.AccountTimeout, err, h.tail.quote())
	}
	if e := accResp.GetError(); e != nil && e.Code != pb.RpcAccountCreateResponseError_NULL {
		return fmt.Errorf("AccountCreate rejected: %s: %s%s", e.Code, e.Description, h.tail.quote())
	}
	h.AccountId = accResp.GetAccount().GetId()
	if h.AccountId == "" {
		return fmt.Errorf("AccountCreate returned no account id%s", h.tail.quote())
	}

	// 4. WalletCreateSession with the mnemonic — the only auth that yields a
	// Full-scope session, and Full is what the next call needs (core/auth.go:
	// AccountLocalLinkCreateApp is outside the Limited allowlist).
	sessResp, err := client.WalletCreateSession(ctx, &pb.RpcWalletCreateSessionRequest{
		Auth: &pb.RpcWalletCreateSessionRequestAuthOfMnemonic{Mnemonic: mnemonic},
	})
	if err != nil {
		return fmt.Errorf("WalletCreateSession: %w%s", err, h.tail.quote())
	}
	if e := sessResp.GetError(); e != nil && e.Code != pb.RpcWalletCreateSessionResponseError_NULL {
		return fmt.Errorf("WalletCreateSession rejected: %s: %s%s", e.Code, e.Description, h.tail.quote())
	}
	token := sessResp.GetToken()
	if token == "" {
		return fmt.Errorf("WalletCreateSession returned no token%s", h.tail.quote())
	}

	// 5. AccountLocalLinkCreateApp mints the key directly. The desktop's
	// challenge flow (new challenge → 4-digit code shown in the UI → solve)
	// exists so a human can approve a pairing; there is no human here, and
	// this call is the same issuance with the consent step removed. The
	// session token rides in gRPC metadata under "token", which is where
	// core/auth.go's interceptor reads it.
	//
	// Scope JsonAPI is what the /v2 gate demands (server/middleware.go
	// ensureJsonApiScope admits JsonAPI and Full only, and Full keys cannot
	// be minted at all). No grant: a grant pins a fixed set of space ids, and
	// the harness creates its space after the key exists. ExpireAt 0 = never.
	appResp, err := client.AccountLocalLinkCreateApp(withToken(ctx, token), &pb.RpcAccountLocalLinkCreateAppRequest{
		App: &model.AccountAuthAppInfo{
			AppName: opt.AppName,
			Scope:   model.AccountAuth_JsonAPI,
		},
	})
	if err != nil {
		return fmt.Errorf("AccountLocalLinkCreateApp: %w%s", err, h.tail.quote())
	}
	if e := appResp.GetError(); e != nil && e.Code != pb.RpcAccountLocalLinkCreateAppResponseError_NULL {
		return fmt.Errorf("AccountLocalLinkCreateApp rejected: %s: %s%s", e.Code, e.Description, h.tail.quote())
	}
	h.APIKey = appResp.GetAppKey()
	if h.APIKey == "" {
		return fmt.Errorf("AccountLocalLinkCreateApp returned no app key%s", h.tail.quote())
	}
	return nil
}

// awaitAPI polls the authenticated whoami until the JSON API answers. This is
// the step that catches a listen address the heart could not bind: startServer
// logs that failure and returns nil, so without this probe the harness would
// go on to run a whole matrix against a URL nothing is listening on.
func (h *Heart) awaitAPI(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 10 * time.Second}
	var last error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.APIURL+"/v2/auth/whoami", nil)
		if err != nil {
			return fmt.Errorf("build whoami request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+h.APIKey)
		resp, err := client.Do(req)
		if err == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			// A 401/403 is terminal, not a warm-up: the key the heart just
			// minted is being refused by the heart that minted it, and
			// retrying cannot fix that.
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				return fmt.Errorf("the fresh account's own API key was refused at %s: HTTP %d: %s%s",
					h.APIURL, resp.StatusCode, strings.TrimSpace(string(body)), h.tail.quote())
			}
			last = fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		} else {
			last = err
		}
		select {
		case <-h.proc.done:
			return fmt.Errorf("the heart exited while waiting for its API (%v)%s", h.proc.err, h.tail.quote())
		case <-ctx.Done():
			return fmt.Errorf("canceled waiting for the API at %s: %w", h.APIURL, ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the JSON API at %s never answered within %s (last: %v)%s",
				h.APIURL, timeout, last, h.tail.quote())
		}
	}
}

// Stop shuts the heart down and removes the data directory unless the caller
// asked to keep it. Safe to call more than once; safe to defer.
func (h *Heart) Stop() error {
	h.stopOnce.Do(func() {
		if h.conn != nil {
			h.conn.Close()
		}
		// SIGTERM, not SIGKILL: cmd/grpcserver's signal handler runs
		// AppShutdown, which closes the object stores cleanly. A half-written
		// store is not a problem for a directory we are about to delete, but
		// it is one for a -keep-account directory someone wants to inspect.
		h.proc.stop(h.stopGrace)
		// Only now: stop returns once both output scanners have finished, so
		// nothing can still be writing to the file.
		if h.logFile != nil {
			h.logFile.Close()
		}
		if h.dataTemp && !h.keepData {
			if err := os.RemoveAll(h.DataDir); err != nil {
				h.stopErr = fmt.Errorf("remove data dir %s: %w", h.DataDir, err)
			}
		}
	})
	return h.stopErr
}

// buildServer builds cmd/grpcserver from the working tree. Building rather
// than taking a prebuilt binary is the default on purpose: the harness's whole
// job is to measure the API as it stands in this tree, and a path to a binary
// someone built last week measures last week's tree while reporting today's.
// The build tags match makefiles/build.mk so the process under test is the
// process we ship.
func buildServer(ctx context.Context, opt Options) (string, error) {
	root := opt.RepoRoot
	if root == "" {
		var err error
		root, err = findRepoRoot()
		if err != nil {
			return "", err
		}
	}
	// One stable output path, overwritten every run: a per-run path would
	// leave a ~200MB binary behind for every kept data directory.
	outDir := filepath.Join(os.TempDir(), "apiv2eval-heart")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create build output dir %s: %w", outDir, err)
	}
	out := filepath.Join(outDir, "grpcserver")

	buildCtx, cancel := context.WithTimeout(ctx, opt.BuildTimeout)
	defer cancel()
	cmd := exec.CommandContext(buildCtx, "go", "build", "-o", out,
		"--tags", "nosigar nowatchdog", "./cmd/grpcserver")
	cmd.Dir = root
	combined, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build cmd/grpcserver in %s (pass -heart-binary to use a prebuilt one): %w\n%s",
			root, err, strings.TrimSpace(string(combined)))
	}
	return out, nil
}

// findRepoRoot walks up from the working directory for anytype-heart's
// go.mod. Any other module's go.mod is skipped rather than accepted: building
// ./cmd/grpcserver in the wrong module fails with a confusing message.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	start := dir
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(data), "module github.com/anyproto/anytype-heart") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no anytype-heart go.mod above %s — run from the repo, or pass the repo root / a prebuilt binary", start)
		}
		dir = parent
	}
}

// awaitAddr waits for the server's own report of where it is listening. It
// watches the process at the same time: a heart that dies on startup (a busy
// port makes it log.Fatal) must be reported as a dead heart, not as a timeout.
func awaitAddr(ctx context.Context, proc *procWaiter, tail *logTail, timeout time.Duration) (string, error) {
	select {
	case addr := <-proc.addrCh:
		return addr, nil
	case <-proc.done:
		return "", fmt.Errorf("the heart exited before it started serving (%v)%s", proc.err, tail.quote())
	case <-ctx.Done():
		return "", fmt.Errorf("canceled while starting the heart: %w", ctx.Err())
	case <-time.After(timeout):
		return "", fmt.Errorf("the heart never printed %q within %s%s", grpcStartedPrefix, timeout, tail.quote())
	}
}

// freePort asks the kernel for an unused TCP port and immediately gives it
// back. There is a race between here and the heart binding it, but the
// alternative is a fixed port — and 31009, the fixed port, is where a running
// desktop client already is.
func freePort() (int, error) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("listen on an ephemeral port: %w", err)
	}
	defer lis.Close()
	return lis.Addr().(*net.TCPAddr).Port, nil
}

// procWaiter owns the child process's lifecycle: it drains both pipes, calls
// Wait exactly once (Wait may not be called twice, and both the startup watch
// and the teardown need the exit), and reports the gRPC address the server
// announces on stdout. err is written before done closes, so any reader that
// has seen done may read it.
type procWaiter struct {
	cmd    *exec.Cmd
	addrCh chan string
	done   chan struct{}
	err    error
}

func newProcWaiter(cmd *exec.Cmd, stdout, stderr io.Reader, tail *logTail) *procWaiter {
	p := &procWaiter{cmd: cmd, addrCh: make(chan string, 1), done: make(chan struct{})}
	var scanners sync.WaitGroup
	scanners.Add(2)
	go func() { defer scanners.Done(); tail.scan(stdout, p.addrCh) }()
	go func() { defer scanners.Done(); tail.scan(stderr, nil) }()
	go func() {
		// Drain both pipes before Wait: Wait closes them, and a scanner
		// reading a closed pipe would drop the last lines — exactly the lines
		// that explain an early exit.
		scanners.Wait()
		p.err = cmd.Wait()
		close(p.done)
	}()
	return p
}

// stop asks the process group to stop, then insists. The group, not the
// process: the heart is its own group leader (Setpgid), so signalling the
// group catches anything it spawned. Returns once the process is reaped or
// the grace period has expired.
func (p *procWaiter) stop(grace time.Duration) {
	if p == nil || p.cmd.Process == nil {
		return
	}
	select {
	case <-p.done:
		return
	default:
	}
	pgid := -p.cmd.Process.Pid
	if err := syscall.Kill(pgid, syscall.SIGTERM); err != nil {
		// Already gone, or never became a group leader — fall back to the
		// process itself and let the kill below finish the job.
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
	}
	select {
	case <-p.done:
	case <-time.After(grace):
		_ = syscall.Kill(pgid, syscall.SIGKILL)
		_ = p.cmd.Process.Kill()
		<-p.done
	}
}

// withToken attaches a session token the way this codebase's gRPC interceptor
// reads it: metadata key "token" (core/auth.go).
func withToken(ctx context.Context, token string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "token", token)
}

// logTail mirrors the heart's output to a file (and optionally to the
// caller's writer) while keeping the last lines in memory, so an error can
// quote what the heart was saying when it went wrong.
type logTail struct {
	mu    sync.Mutex
	lines []string
	sink  io.Writer
	echo  io.Writer
}

// scan reads one of the child's pipes to EOF. When addrCh is non-nil, the
// gRPC address is reported on it the first time the server announces one.
func (t *logTail) scan(r io.Reader, addrCh chan<- string) {
	scanner := bufio.NewScanner(r)
	// The heart logs whole JSON payloads on some paths; the default 64KB
	// token limit would turn one long line into a scanner error and cost us
	// the rest of the stream.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	sent := false
	for scanner.Scan() {
		line := scanner.Text()
		t.add(line)
		if !sent && addrCh != nil {
			if addr, ok := strings.CutPrefix(line, grpcStartedPrefix); ok {
				sent = true
				addrCh <- strings.TrimSpace(addr)
			}
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, os.ErrClosed) {
		t.add(fmt.Sprintf("[heartboot] reading heart output: %v", err))
	}
}

func (t *logTail) add(line string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sink != nil {
		fmt.Fprintln(t.sink, line)
	}
	if t.echo != nil {
		fmt.Fprintln(t.echo, line)
	}
	t.lines = append(t.lines, line)
	if len(t.lines) > logTailLines {
		t.lines = t.lines[len(t.lines)-logTailLines:]
	}
}

// quote renders the tail for embedding in an error, already prefixed with a
// newline so callers can append it unconditionally.
func (t *logTail) quote() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.lines) == 0 {
		return "\n--- the heart printed nothing ---"
	}
	return "\n--- last " + fmt.Sprint(len(t.lines)) + " lines from the heart ---\n" +
		strings.Join(t.lines, "\n") + "\n--- end ---"
}
