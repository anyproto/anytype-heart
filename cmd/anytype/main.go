// Command anytype is the task-tool CLI (APIV2.md §2 Phase 5 / §7): the same
// ~11 task tools the on-device manifest serves, delivered as verbs for
// coding-agent harnesses. The verb set is GENERATED from the wrapper
// package's tool table — one definition, two deliveries; this file only
// parses flags and prints results.
//
//	anytype find --space s1 --type task --filter 'done = false'
//	anytype read --object 1 --mode outline
//	anytype add-blocks --object 1 --markdown '- [ ] follow up'
//	anytype tools            # the machine-readable manifest (JSON)
//
// Configuration: ANYTYPE_API_URL (default http://127.0.0.1:31009) and
// ANYTYPE_API_KEY (bearer key from the app's API settings). Handle state
// (find's numbered results, block labels) persists in a session file
// (ANYTYPE_CLI_SESSION overrides the location) so verbs compose across
// invocations.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(argv []string, stdout, stderr *os.File) int {
	if len(argv) == 0 || argv[0] == "help" || argv[0] == "--help" || argv[0] == "-h" {
		printUsage(stdout)
		if len(argv) == 0 {
			return 2
		}
		return 0
	}
	verb := argv[0]

	if verb == "tools" {
		manifest, err := wrapper.ManifestJSON()
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		fmt.Fprintln(stdout, string(manifest))
		return 0
	}

	tool, ok := wrapper.ToolByVerb(verb)
	if !ok {
		fmt.Fprintf(stderr, "unknown verb %q — verbs: %s, tools\n", verb, strings.Join(verbs(), ", "))
		return 2
	}

	args, opts, err := parseVerbFlags(tool, argv[1:])
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}

	runner, err := buildRunner(opts)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	result, err := runner.Run(context.Background(), tool.Name, args)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if opts.jsonOut && result.JSON != nil {
		printJSON(stdout, stderr, result.JSON)
		return 0
	}
	fmt.Fprintln(stdout, result.Text)
	return 0
}

// cliOptions are the cross-verb flags.
type cliOptions struct {
	jsonOut       bool
	dryRun        bool
	ifMatch       string
	createMissing bool
}

// parseVerbFlags registers one flag per tool argument (from the same table
// the manifest serves) plus the cross-verb flags, and builds the args map.
func parseVerbFlags(tool wrapper.Tool, argv []string) (map[string]any, *cliOptions, error) {
	fs := flag.NewFlagSet(tool.Verb(), flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	strFlags := map[string]*string{}
	boolFlags := map[string]*bool{}
	intFlags := map[string]*int{}
	for _, a := range tool.Args {
		switch a.Type {
		case wrapper.ArgBoolean:
			boolFlags[a.Name] = fs.Bool(a.Name, false, a.Description)
		case wrapper.ArgInteger:
			intFlags[a.Name] = fs.Int(a.Name, 0, a.Description)
		default:
			// object args are passed as JSON strings on the CLI
			desc := a.Description
			if a.Type == wrapper.ArgObject {
				desc += ` (JSON, e.g. '{"status":"Done"}')`
			}
			strFlags[a.Name] = fs.String(a.Name, "", desc)
		}
	}
	opts := &cliOptions{}
	fs.BoolVar(&opts.jsonOut, "json", false, "print the machine-readable result")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "validate without committing (?dry_run=true)")
	fs.StringVar(&opts.ifMatch, "if-match", "", "advanced: require this etag (C7); the task tools omit it by default")
	fs.BoolVar(&opts.createMissing, "create-missing", false, "advanced: let unknown select option names be created (R9)")

	if err := fs.Parse(argv); err != nil {
		return nil, nil, err
	}
	if fs.NArg() > 0 {
		return nil, nil, fmt.Errorf("unexpected argument %q — %s takes flags only (--%s …)", fs.Arg(0), tool.Verb(), tool.Args[0].Name)
	}

	args := map[string]any{}
	for name, v := range strFlags {
		if *v == "" {
			continue
		}
		if a, _ := toolArg(tool, name); a.Type == wrapper.ArgObject {
			obj, err := wrapper.ParseObjectFlag(*v)
			if err != nil {
				return nil, nil, fmt.Errorf("--%s: %w", name, err)
			}
			args[name] = obj
			continue
		}
		args[name] = *v
	}
	for name, v := range boolFlags {
		if flagWasSet(fs, name) {
			args[name] = *v
		}
	}
	for name, v := range intFlags {
		if flagWasSet(fs, name) {
			args[name] = *v
		}
	}
	return args, opts, nil
}

func toolArg(tool wrapper.Tool, name string) (wrapper.Arg, bool) {
	for _, a := range tool.Args {
		if a.Name == name {
			return a, true
		}
	}
	return wrapper.Arg{}, false
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

func buildRunner(opts *cliOptions) (*wrapper.Runner, error) {
	sessionPath, err := wrapper.DefaultSessionPath()
	if err != nil {
		return nil, err
	}
	client := wrapper.NewClient(os.Getenv("ANYTYPE_API_URL"), os.Getenv("ANYTYPE_API_KEY"))
	runner := wrapper.NewRunner(client, &wrapper.FileStore{Path: sessionPath})
	runner.DryRun = opts.dryRun
	runner.IfMatch = opts.ifMatch
	runner.AllowNewOptions = opts.createMissing
	return runner, nil
}

func printJSON(stdout, stderr *os.File, v any) {
	data, err := wrapper.EncodeJSON(v)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return
	}
	fmt.Fprintln(stdout, string(data))
}

func verbs() []string {
	var out []string
	for _, t := range wrapper.Tools() {
		out = append(out, t.Verb())
	}
	sort.Strings(out)
	return out
}

func printUsage(w *os.File) {
	fmt.Fprintln(w, "anytype — task tools over the local Anytype API (v2)")
	fmt.Fprintln(w, "\nusage: anytype <verb> [--flag value …]")
	fmt.Fprintln(w, "\nverbs:")
	for _, t := range wrapper.Tools() {
		fmt.Fprintf(w, "  %-15s %s\n", t.Verb(), firstSentence(t.Description))
	}
	fmt.Fprintf(w, "  %-15s %s\n", "tools", "print the machine-readable tool manifest (JSON)")
	fmt.Fprintln(w, "\ncross-verb flags: --json, --dry-run, --if-match <etag>, --create-missing")
	fmt.Fprintln(w, "environment: ANYTYPE_API_URL (default "+wrapper.DefaultBaseURL+"), ANYTYPE_API_KEY, ANYTYPE_CLI_SESSION")
	fmt.Fprintln(w, "\nstart with: anytype find --space <spaceId> --query … ; results are numbered handles the other verbs take as --object")
}

func firstSentence(s string) string {
	if idx := strings.IndexAny(s, "."); idx > 0 {
		return s[:idx+1]
	}
	return s
}
