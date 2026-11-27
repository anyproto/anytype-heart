// Command anytype-mcp is the stdio side of an AI app's connection to
// Anytype: it forwards MCP messages from stdin to the local Anytype's
// /mcp/<tier> endpoint and owns the one private file that holds the
// connection's key.
//
//	anytype-mcp serve    --connection <id>
//	anytype-mcp key set      --connection <id> --expect absent|<fp>@<rev>   # payload on stdin
//	anytype-mcp key retarget --connection <id> --expect <fp>@<rev> --account <id> --port N --tier T
//	anytype-mcp key delete   --connection <id> --expect <fp>@<rev> --account <id>
//	anytype-mcp key check    --connection <id>
//	anytype-mcp version [--json]
//
// The item is one private file per connection under the user's home
// (internal/store); the forwarder runs as its caller and reads only what
// its caller could. The key never appears in arguments, the environment or
// logs. Nothing but the home directory steers the binary: there are no
// override flags and no test build. It imports nothing from heart beyond
// its own packages.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/forward"
	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

// Exit codes. They are a contract with the desktop app.
const (
	exitOK       = 0
	exitInternal = 1
	exitUsage    = 2
	exitNoItem   = 3
	// 4 is reserved (it once meant "no key store") and never returned.
	exitPrecondition = 5
	exitInvalidInput = 6
	// exitTemporary: nothing was read or written; try again.
	exitTemporary = 7
)

const usage = `usage:
  anytype-mcp serve    --connection <id>
  anytype-mcp key set      --connection <id> --expect absent|<fp>@<rev>   (payload on stdin)
  anytype-mcp key retarget --connection <id> --expect <fp>@<rev> --account <id> --port N --tier T
  anytype-mcp key delete   --connection <id> --expect <fp>@<rev> --account <id>
  anytype-mcp key check    --connection <id>
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
	case "key":
		return keyCmd(args[1:], stdin, stdout, stderr)
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
	fs := newFlags("serve", stderr)
	connection := fs.String("connection", "", "connection id")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return exitUsage
	}
	if !store.ValidConnectionID(*connection) {
		return usageError(stderr, "--connection must match ^[a-z0-9-]{1,64}$")
	}

	// a closed stdout must arrive as a write error, not as a signal
	ignoreSIGPIPE()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return forward.Serve(ctx, forward.Options{
		ConnectionID: *connection,
		Store:        store.Open(namespace),
		In:           stdin,
		Out:          stdout,
		Log:          stderr,
	})
}
