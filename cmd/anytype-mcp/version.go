package main

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"
	"strings"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

// Rev is the forwarder's revision: one integer, bumped by hand in any
// change to the forwarder's code, its dependencies or its Go toolchain.
// The desktop app never replaces a stable copy with a lower Rev, and the
// Claude bundle's version is 2.<Rev>.0. guard_test.go fails when the
// sources or a dependency version change without a bump.
const Rev = 4

// Set at link time (-ldflags -X main.…).
//
// linkNamespace is the whole marker, so the namespace of any build, for any
// OS, can be read off the file: "anytype-mcp-namespace:release" is linked
// only into the binary that release and nightly desktop builds pick. The
// namespace picks the items folder (items or items-dev), so every other
// build can't see a production item. There is no flag and no environment
// variable.
var (
	linkNamespace = namespaceMarker + store.NamespaceDev
	version       = "dev"
)

const namespaceMarker = "anytype-mcp-namespace:"

// namespace is fixed for the life of the process.
var namespace = store.NormalizeNamespace(strings.TrimPrefix(linkNamespace, namespaceMarker))

type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Rev       int    `json:"rev"`
	Namespace string `json:"namespace"`
	Impl      string `json:"impl"`
}

func currentVersion() versionInfo {
	info := versionInfo{Version: version, Rev: Rev, Namespace: store.NormalizeNamespace(namespace), Impl: "go"}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" {
				info.Commit = s.Value
			}
		}
	}
	return info
}

func versionCmd(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("version", stderr)
	asJSON := fs.Bool("json", false, "print one JSON line")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return exitUsage
	}
	info := currentVersion()
	if *asJSON {
		line, err := json.Marshal(info)
		if err != nil {
			return exitInternal
		}
		fmt.Fprintln(stdout, string(line))
		return exitOK
	}
	fmt.Fprintf(stdout, "anytype-mcp %s (rev %d, %s namespace)\n", info.Version, info.Rev, info.Namespace)
	return exitOK
}
