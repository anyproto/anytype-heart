package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

// TestVerbFlagsFromToolTable proves the CLI registers every tool argument
// as a flag and converts values to the tool-args shape — the one-definition
// contract on the CLI side.
func TestVerbFlagsFromToolTable(t *testing.T) {
	t.Run("every tool's flags parse its own example", func(t *testing.T) {
		for _, tool := range wrapper.Tools() {
			t.Run(tool.Name, func(t *testing.T) {
				var argv []string
				for name, value := range tool.Example {
					switch v := value.(type) {
					case string:
						argv = append(argv, "--"+name, v)
					case bool:
						argv = append(argv, "--"+name+"=true")
					case int:
						argv = append(argv, "--"+name, "1")
					case map[string]any:
						data, err := wrapper.EncodeJSON(v)
						require.NoError(t, err)
						argv = append(argv, "--"+name, string(data))
					}
				}
				args, _, err := parseVerbFlags(tool, argv, io.Discard)
				require.NoError(t, err)
				for name := range tool.Example {
					assert.Contains(t, args, name)
				}
			})
		}
	})

	t.Run("string and object flags convert", func(t *testing.T) {
		tool, ok := wrapper.ToolByVerb("set-properties")
		require.True(t, ok)
		args, opts, err := parseVerbFlags(tool, []string{
			"--object", "1", "--set", `{"status":"Done","priority":3}`, "--json",
		}, io.Discard)
		require.NoError(t, err)
		assert.Equal(t, "1", args["object"])
		assert.Equal(t, map[string]any{"status": "Done", "priority": float64(3)}, args["set"])
		assert.True(t, opts.jsonOut)
	})

	t.Run("integer flags land as ints, not strings", func(t *testing.T) {
		tool, ok := wrapper.ToolByVerb("find")
		require.True(t, ok)
		args, _, err := parseVerbFlags(tool, []string{"--space", "s1", "--limit", "25"}, io.Discard)
		require.NoError(t, err)
		assert.Equal(t, 25, args["limit"], "the ArgInteger branch must produce an int")
	})

	t.Run("boolean flags land as bools", func(t *testing.T) {
		tool, ok := wrapper.ToolByVerb("delete-block")
		require.True(t, ok)
		args, _, err := parseVerbFlags(tool, []string{"--object", "1", "--block", "ab123", "--recursive"}, io.Discard)
		require.NoError(t, err)
		assert.Equal(t, true, args["recursive"])
	})

	t.Run("an explicitly empty string flag is passed through, an unset one is not", func(t *testing.T) {
		tool, ok := wrapper.ToolByVerb("edit-text")
		require.True(t, ok)
		args, _, err := parseVerbFlags(tool, []string{
			"--object", "1", "--block", "ab3f2", "--find", " (draft)", "--replace", "",
		}, io.Discard)
		require.NoError(t, err)
		val, present := args["replace"]
		assert.True(t, present, `--replace "" means "delete the phrase" and must reach the tool`)
		assert.Equal(t, "", val)
		assert.NotContains(t, args, "after", "flags never given stay off the args map")
	})

	t.Run("cross-verb flags", func(t *testing.T) {
		tool, _ := wrapper.ToolByVerb("create")
		_, opts, err := parseVerbFlags(tool, []string{
			"--space", "s1", "--type", "task", "--name", "X",
			"--dry-run", "--create-missing", "--if-match", "abcd1234",
		}, io.Discard)
		require.NoError(t, err)
		assert.True(t, opts.dryRun)
		assert.True(t, opts.createMissing)
		assert.Equal(t, "abcd1234", opts.ifMatch)
	})

	t.Run("bad object JSON steers", func(t *testing.T) {
		tool, _ := wrapper.ToolByVerb("set-properties")
		_, _, err := parseVerbFlags(tool, []string{"--object", "1", "--set", "status=Done"}, io.Discard)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `expected a JSON object, e.g. '{"Status":"Done"}'`)
	})

	t.Run("positional arguments are rejected with steering", func(t *testing.T) {
		tool, _ := wrapper.ToolByVerb("find")
		_, _, err := parseVerbFlags(tool, []string{"space1"}, io.Discard)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "find takes flags only (--space …)")
	})

	t.Run("unset boolean flags stay off the args map", func(t *testing.T) {
		tool, _ := wrapper.ToolByVerb("delete-block")
		args, _, err := parseVerbFlags(tool, []string{"--object", "1", "--block", "ab123"}, io.Discard)
		require.NoError(t, err)
		assert.NotContains(t, args, "recursive")
	})
}

// runCLI captures run()'s exit code and both output channels (stdin empty —
// only the mcp verb reads it; runCLIWithStdin feeds it).
func runCLI(argv ...string) (code int, stdout, stderr string) {
	return runCLIWithStdin("", argv...)
}

func runCLIWithStdin(stdin string, argv ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(argv, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRunExitCodes(t *testing.T) {
	t.Run("no arguments prints usage and exits 2", func(t *testing.T) {
		code, stdout, _ := runCLI()
		assert.Equal(t, 2, code)
		assert.Contains(t, stdout, "usage: anytype <verb>")
	})

	t.Run("help exits 0", func(t *testing.T) {
		for _, h := range []string{"help", "--help", "-h"} {
			code, stdout, stderr := runCLI(h)
			assert.Equal(t, 0, code, h)
			assert.Contains(t, stdout, "usage: anytype <verb>")
			assert.NotContains(t, stderr, "error:")
		}
	})

	t.Run("verb --help exits 0 without an error line", func(t *testing.T) {
		code, _, stderr := runCLI("find", "--help")
		assert.Equal(t, 0, code, "--help is a request, not a mistake")
		assert.NotContains(t, stderr, "error:")
		assert.Contains(t, stderr, "-space", "the flag listing is shown")
	})

	t.Run("unknown verb exits 2 with the verb list", func(t *testing.T) {
		code, _, stderr := runCLI("archive")
		assert.Equal(t, 2, code)
		assert.Contains(t, stderr, `unknown verb "archive"`)
		assert.Contains(t, stderr, "tools")
	})

	t.Run("bad flags exit 2", func(t *testing.T) {
		code, _, stderr := runCLI("find", "--nope")
		assert.Equal(t, 2, code)
		assert.NotEmpty(t, stderr)
	})

	t.Run("tools prints the machine-readable manifest and exits 0", func(t *testing.T) {
		code, stdout, _ := runCLI("tools")
		assert.Equal(t, 0, code)
		var m wrapper.Manifest
		require.NoError(t, json.Unmarshal([]byte(stdout), &m))
		require.Len(t, m.Tools, len(wrapper.Tools()))
	})

	t.Run("tools --tier small prints the small-tier manifest", func(t *testing.T) {
		code, stdout, _ := runCLI("tools", "--tier", "small")
		assert.Equal(t, 0, code)
		var m wrapper.Manifest
		require.NoError(t, json.Unmarshal([]byte(stdout), &m))
		require.Len(t, m.Tools, len(wrapper.ToolsForTier(wrapper.TierSmall)))
	})

	t.Run("a bad tier exits 2 naming the tiers", func(t *testing.T) {
		for _, verb := range []string{"tools", "mcp"} {
			code, _, stderr := runCLI(verb, "--tier", "medium")
			assert.Equal(t, 2, code, verb)
			assert.Contains(t, stderr, `unknown tier "medium" — tiers: small, large`, verb)
		}
	})
}

// TestRunMCP drives the mcp verb through stdio: the verb is the §8.20
// long-lived delivery, so an initialize → tools/list script must answer
// over stdout and EOF must end the process cleanly with exit 0.
func TestRunMCP(t *testing.T) {
	t.Run("EOF on stdin exits 0", func(t *testing.T) {
		code, stdout, stderr := runCLIWithStdin("", "mcp")
		assert.Equal(t, 0, code, stderr)
		assert.Empty(t, stdout)
	})

	t.Run("initialize and tier-filtered tools/list over stdio", func(t *testing.T) {
		script := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n"
		code, stdout, stderr := runCLIWithStdin(script, "mcp", "--tier", "small")
		require.Equal(t, 0, code, stderr)

		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		require.Len(t, lines, 2, "two requests, one notification → two responses")

		var initResp struct {
			Result struct {
				ProtocolVersion string `json:"protocolVersion"`
				Instructions    string `json:"instructions"`
			} `json:"result"`
		}
		require.NoError(t, json.Unmarshal([]byte(lines[0]), &initResp))
		assert.Equal(t, "2025-06-18", initResp.Result.ProtocolVersion)
		assert.NotEmpty(t, initResp.Result.Instructions)

		var listResp struct {
			Result struct {
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			} `json:"result"`
		}
		require.NoError(t, json.Unmarshal([]byte(lines[1]), &listResp))
		var names []string
		for _, tool := range listResp.Result.Tools {
			names = append(names, tool.Name)
		}
		assert.Equal(t, wrapper.ToolNamesForTier(wrapper.TierSmall), names)
	})

	t.Run("a tools/call reaches the API server", func(t *testing.T) {
		stubServer(t, func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/v2/spaces", r.URL.Path)
			require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
			fmt.Fprint(w, `{"data":[{"id":"bafyspace1","name":"Work"}],"total":1,"offset":0,"limit":25,"has_more":false}`)
		})
		script := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"spaces"}}` + "\n"
		code, stdout, stderr := runCLIWithStdin(script, "mcp")
		require.Equal(t, 0, code, stderr)
		assert.Contains(t, stdout, "Work — bafyspace1")
	})
}

// stubServer runs a stub API server and points the CLI's env at it, with an
// isolated session file.
func stubServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("ANYTYPE_API_URL", server.URL)
	t.Setenv("ANYTYPE_API_KEY", "test-key")
	t.Setenv("ANYTYPE_CLI_SESSION", t.TempDir()+"/session.json")
}

func TestRunEndToEnd(t *testing.T) {
	t.Run("a verb runs against the API and prints the text form", func(t *testing.T) {
		stubServer(t, func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "GET", r.Method)
			require.Equal(t, "/v2/spaces", r.URL.Path)
			fmt.Fprint(w, `{"data":[{"id":"bafyspace1","name":"Work"}],"total":1,"offset":0,"limit":25,"has_more":false}`)
		})

		code, stdout, stderr := runCLI("spaces")

		assert.Equal(t, 0, code, stderr)
		assert.Contains(t, stdout, "Work — bafyspace1")
	})

	t.Run("--json selects the machine shape", func(t *testing.T) {
		stubServer(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"data":[{"id":"bafyspace1","name":"Work"}],"total":1,"offset":0,"limit":25,"has_more":false}`)
		})

		code, stdout, stderr := runCLI("spaces", "--json")

		assert.Equal(t, 0, code, stderr)
		var got struct {
			Spaces []struct {
				Id   string `json:"id"`
				Name string `json:"name"`
			} `json:"spaces"`
			Total int `json:"total"`
		}
		require.NoError(t, json.Unmarshal([]byte(stdout), &got))
		require.Len(t, got.Spaces, 1)
		assert.Equal(t, "bafyspace1", got.Spaces[0].Id)
	})

	t.Run("a tool error exits 1 with the error text", func(t *testing.T) {
		stubServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"status":400,"code":"validation_failed","message":"filter parse error at offset 3"}`)
		})

		code, _, stderr := runCLI("find", "--space", "s1", "--filter", "x ~ 1")

		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "error:")
		assert.Contains(t, stderr, "filter parse error at offset 3")
	})
}
