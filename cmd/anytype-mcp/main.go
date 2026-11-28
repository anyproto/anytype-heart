// Command anytype-mcp is the stdio side of an AI app's connection to
// Anytype: it forwards MCP messages from stdin to the local Anytype's
// /mcp/<tier> endpoint with the key the AI app gives it.
//
//	anytype-mcp serve [--tier small|large|full] [--files-dir DIR]
//	anytype-mcp endpoint
//	anytype-mcp version [--json]
//
// The key comes from ANYTYPE_API_KEY, read once and unset; it never
// appears in arguments, files or logs. The port comes from the endpoint
// file the desktop app writes (internal/store), read before every request
// that carries the key and only through its safety rules. serve
// --files-dir (or ANYTYPE_MCP_FILES_DIR) names the one local folder the
// full tier's file tools may touch; without it they get a new private
// temporary folder per process. endpoint runs serve's read once and says
// whether serve would find Anytype safely. Nothing else steers the binary:
// no override flags and no test build. It imports nothing from heart
// beyond its own packages.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/contract"
	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/files"
	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/forward"
	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

// Exit codes. They are a contract with the desktop app.
const (
	exitOK       = 0
	exitInternal = 1 // serve: replies were lost; endpoint: the read failed
	exitUsage    = 2
	// 3, 4, 5 and 7 belonged to the removed key verbs and are never
	// returned.
	exitInvalidInput = 6
)

const usage = `usage:
  anytype-mcp serve [--tier small|large|full] [--files-dir DIR]   (key in ANYTYPE_API_KEY)
  anytype-mcp endpoint
  anytype-mcp version [--json]
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "serve":
		return serveCmd(args[1:], stdin, stdout, stderr)
	case "endpoint":
		return endpointCmd(args[1:], stdout, stderr)
	case "version":
		return versionCmd(args[1:], stdout, stderr)
	}
	fmt.Fprint(stderr, usage)
	return exitUsage
}

// newFlags builds a flag set that reports problems as a usage error
// instead of exiting on its own.
func newFlags(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	return fs
}

func usageError(stderr io.Writer, msg string) int {
	fmt.Fprintf(stderr, "anytype-mcp: %s\n", msg)
	fmt.Fprint(stderr, usage)
	return exitUsage
}

func serveCmd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// the key is read once and leaves the process environment at once
	key := os.Getenv("ANYTYPE_API_KEY")
	_ = os.Unsetenv("ANYTYPE_API_KEY")

	fs := newFlags("serve", stderr)
	tier := fs.String("tier", "full", "small, large or full")
	filesDir := fs.String("files-dir", "", "absolute path of the folder for the full tier's file tools")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return exitUsage
	}
	if !slices.Contains(contract.Tiers, *tier) {
		return usageError(stderr, "--tier must be small, large or full")
	}
	// the folder is fixed here, before anything is read from stdin. The
	// flag wins over the bundle's optional field, which arrives unresolved
	// when the user left it out.
	folder, given := *filesDir, false
	fs.Visit(func(f *flag.Flag) { given = given || f.Name == "files-dir" })
	what := "--files-dir"
	if !given {
		env := os.Getenv("ANYTYPE_MCP_FILES_DIR")
		folder, given, what = env, env != "" && env != "${user_config.files_dir}", "ANYTYPE_MCP_FILES_DIR"
	}
	var dir *files.Dir
	if given {
		if !filepath.IsAbs(folder) {
			return usageError(stderr, what+" must be an absolute path")
		}
		// never Anytype's own folder, nor a folder around it (the home
		// folder)
		var avoid []string
		if home, err := os.UserHomeDir(); err == nil && filepath.IsAbs(home) {
			avoid = append(avoid, filepath.Join(home, ".anytype"))
		}
		var err error
		if dir, err = files.Open(folder, avoid...); err != nil {
			fmt.Fprintf(stderr, "anytype-mcp: %s: %v\n", what, err)
			return exitInvalidInput
		}
	}

	// a closed stdout must arrive as a write error, not as a signal
	ignoreSIGPIPE()
	// the forwarder logs only through its own logger: Go's net/http prints
	// bytes heart never asked to send (on an idle connection) through the
	// standard logger, straight to stderr
	log.SetOutput(io.Discard)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return forward.Serve(ctx, forward.Options{
		Key:      key,
		Tier:     *tier,
		Endpoint: store.Open(namespace),
		Files:    dir,
		In:       stdin,
		Out:      stdout,
		Log:      stderr,
	})
}

type endpointResult struct {
	OK     bool   `json:"ok"`
	Port   int    `json:"port,omitempty"`
	Reason string `json:"reason,omitempty"`
	Path   string `json:"path,omitempty"`
	Rule   string `json:"rule,omitempty"`
}

// checkSelf judges the program the caller would name in a config: the
// path it started this process by, exactly as it spelled it (a bare name
// found on PATH leaves only the file the system reports), and that
// running file.
func checkSelf(f *store.File) error {
	running, err := os.Executable()
	if err != nil {
		return &store.Error{Reason: store.ReasonDenied, Rule: "the forwarder can't tell which file it runs: " + err.Error()}
	}
	// taken as it is: CheckExecutable refuses one that is not clean and
	// absolute, rather than cleaning away a folder it passes
	launch := os.Args[0]
	if !strings.ContainsAny(launch, `/\`) {
		launch = running
	}
	return f.CheckExecutable(launch, running)
}

// endpointCmd is serve's read, once: one JSON line, exit 0 when serve
// would find Anytype safely, 1 when it would not. It also judges its own
// executable (checkSelf), since that is the program an AI app would give
// its key to.
func endpointCmd(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("endpoint", stderr)
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return exitUsage
	}
	f := store.Open(namespace)
	type read struct {
		port int
		err  error
	}
	done := make(chan read, 1)
	go func() {
		port, err := f.Read(context.Background())
		if err == nil {
			err = checkSelf(f)
		}
		done <- read{port, err}
	}()
	var r read
	select {
	case r = <-done:
	case <-time.After(contract.EndpointReadTimeout):
		r.err = &store.Error{Reason: store.ReasonUnreachable, Rule: "the endpoint file did not answer in time"}
	}
	out := endpointResult{OK: r.err == nil, Port: r.port}
	code := exitOK
	if r.err != nil {
		e := store.AsError(r.err)
		out = endpointResult{Reason: e.Reason, Path: e.Path, Rule: e.Rule}
		if out.Rule == "" && e.Err != nil {
			out.Rule = e.Err.Error()
		}
		code = exitInternal
	}
	line, err := json.Marshal(out)
	if err != nil {
		return exitInternal
	}
	fmt.Fprintln(stdout, string(line))
	return code
}
