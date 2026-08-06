package main

import (
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
				args, _, err := parseVerbFlags(tool, argv)
				require.NoError(t, err)
				for name := range tool.Example {
					assert.Contains(t, args, name)
				}
			})
		}
	})

	t.Run("string, bool, int and object flags convert", func(t *testing.T) {
		tool, ok := wrapper.ToolByVerb("set-properties")
		require.True(t, ok)
		args, opts, err := parseVerbFlags(tool, []string{
			"--object", "1", "--set", `{"status":"Done","priority":3}`, "--json",
		})
		require.NoError(t, err)
		assert.Equal(t, "1", args["object"])
		assert.Equal(t, map[string]any{"status": "Done", "priority": float64(3)}, args["set"])
		assert.True(t, opts.jsonOut)
	})

	t.Run("cross-verb flags", func(t *testing.T) {
		tool, _ := wrapper.ToolByVerb("create")
		_, opts, err := parseVerbFlags(tool, []string{
			"--space", "s1", "--type", "task", "--name", "X",
			"--dry-run", "--create-missing", "--if-match", "abcd1234",
		})
		require.NoError(t, err)
		assert.True(t, opts.dryRun)
		assert.True(t, opts.createMissing)
		assert.Equal(t, "abcd1234", opts.ifMatch)
	})

	t.Run("bad object JSON steers", func(t *testing.T) {
		tool, _ := wrapper.ToolByVerb("set-properties")
		_, _, err := parseVerbFlags(tool, []string{"--object", "1", "--set", "status=Done"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `expected a JSON object, e.g. '{"status":"Done"}'`)
	})

	t.Run("positional arguments are rejected with steering", func(t *testing.T) {
		tool, _ := wrapper.ToolByVerb("find")
		_, _, err := parseVerbFlags(tool, []string{"space1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "find takes flags only (--space …)")
	})

	t.Run("unset boolean flags stay off the args map", func(t *testing.T) {
		tool, _ := wrapper.ToolByVerb("delete-block")
		args, _, err := parseVerbFlags(tool, []string{"--object", "1", "--block", "ab123"})
		require.NoError(t, err)
		assert.NotContains(t, args, "recursive")
	})
}
