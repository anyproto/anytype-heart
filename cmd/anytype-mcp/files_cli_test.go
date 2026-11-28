package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var folderRe = regexp.MustCompile(`Upload a file from (.+) on the computer running`)

// listedFolder is the folder the file tools name in their descriptions,
// or empty when they are not listed.
func listedFolder(t *testing.T, list map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(list["result"])
	require.NoError(t, err)
	var r struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(raw, &r))
	for _, tool := range r.Tools {
		if tool.Name == "upload_local_file" {
			m := folderRe.FindStringSubmatch(tool.Description)
			require.Len(t, m, 2, tool.Description)
			return m[1]
		}
	}
	return ""
}

const toolsLine = `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`

// H1 case 13: --files-dir "", relative and unusable absolute exit 2, 2
// and 6; ANYTYPE_MCP_FILES_DIR empty or unresolved is unset, a folder is
// used, and the flag wins over it.
func TestH1_13_FilesDir(t *testing.T) {
	t.Run("a flag that is not an absolute path is a usage error", func(t *testing.T) {
		e := newEnv(t)
		for _, value := range []string{"", "files", "./files", "../files", "~/files"} {
			r := e.run("", "serve", "--files-dir", value)
			assert.Equal(t, exitUsage, r.code, value)
			assert.Contains(t, r.stderr, "--files-dir must be an absolute path")
			assert.Empty(t, r.stdout)
		}
		assert.Equal(t, exitUsage, e.run("", "serve", "--files-dir").code)
		assert.Equal(t, exitUsage, e.run("", "endpoint", "--files-dir", e.home).code, "serve only")
	})

	t.Run("the variable follows the flag's rules and exit codes", func(t *testing.T) {
		e := newEnv(t)
		e.extra = []string{"ANYTYPE_MCP_FILES_DIR=files"}
		r := e.run("", "serve")
		assert.Equal(t, exitUsage, r.code)
		assert.Contains(t, r.stderr, "ANYTYPE_MCP_FILES_DIR must be an absolute path")
		e.extra = []string{"ANYTYPE_MCP_FILES_DIR=" + e.home}
		r = e.run(initLine+"\n", "serve")
		assert.Equal(t, exitInvalidInput, r.code)
		assert.Contains(t, r.stderr, "ANYTYPE_MCP_FILES_DIR: ")
		assert.Empty(t, r.stdout)
	})

	t.Run("a folder that breaks a rule ends serve before it reads anything, naming the rule", func(t *testing.T) {
		e := newEnv(t)
		file := filepath.Join(e.home, "file")
		require.NoError(t, os.WriteFile(file, nil, 0o600))
		broken := map[string]string{
			file:                              "must be a directory",
			filepath.Join(file, "below"):      "",
			filepath.Join(e.home, ".anytype"): "apart from Anytype's own folder",
		}
		for dir, rule := range broken {
			r := e.run(initLine+"\n", "serve", "--files-dir", dir)
			assert.Equal(t, exitInvalidInput, r.code, dir)
			assert.Contains(t, r.stderr, "--files-dir: ")
			assert.Contains(t, r.stderr, rule, dir)
			assert.Empty(t, r.stdout)
		}
	})

	t.Run("the folder is never Anytype's own, nor around it", func(t *testing.T) {
		e := newEnv(t)
		e.writeEndpoint(1)
		for _, dir := range []string{e.home, filepath.Join(e.home, ".anytype"), filepath.Dir(e.endpointPath()), filepath.Join(e.home, ".anytype", "files"),
			e.home + string(filepath.Separator) + "x" + string(filepath.Separator) + ".." + string(filepath.Separator) + "y"} {
			r := e.run(initLine+"\n", "serve", "--files-dir", dir)
			assert.Equal(t, exitInvalidInput, r.code, dir)
			assert.Regexp(t, `apart from Anytype's own folder|without ".."`, r.stderr, dir)
			assert.Empty(t, r.stdout)
		}
		_, err := os.Stat(filepath.Join(e.home, ".anytype", "files"))
		assert.True(t, os.IsNotExist(err) || err == nil)
	})

	t.Run("a missing folder is created private, with or without Anytype", func(t *testing.T) {
		e := newEnv(t)
		dir := filepath.Join(e.home, "ai", "files")
		r := e.run(initLine+"\n", "serve", "--files-dir", dir)
		assert.Equal(t, exitOK, r.code)
		assert.Contains(t, r.stdout, `"code":-32001`)
		fi, err := os.Lstat(dir)
		require.NoError(t, err)
		assert.True(t, fi.IsDir())
		if runtime.GOOS != "windows" {
			assert.Equal(t, os.FileMode(0o700), fi.Mode().Perm())
		}
	})

	t.Run("any folder of the user's choice is taken: shared, or spelled through a link", func(t *testing.T) {
		e := newEnv(t)
		shared := filepath.Join(e.home, "shared")
		require.NoError(t, os.Mkdir(shared, 0o700))
		require.NoError(t, os.Chmod(shared, 0o777))
		dirs := []string{shared}
		if err := os.Symlink(shared, filepath.Join(e.home, "link")); err == nil {
			dirs = append(dirs, filepath.Join(e.home, "link"))
		}
		for _, dir := range dirs {
			r := e.run(initLine+"\n", "serve", "--files-dir", dir)
			assert.Equal(t, exitOK, r.code, "%s: %s", dir, r.stderr)
			assert.Contains(t, r.stdout, `"code":-32001`)
		}
		if runtime.GOOS != "windows" {
			fi, err := os.Stat(shared)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o777), fi.Mode().Perm(), "its mode is left alone")
		}
	})

	// listed is the folder the full tier's file tools name for one setup
	listed := func(t *testing.T, e *env, args ...string) string {
		t.Helper()
		heart := newHeartStub(t)
		heart.tools.Store(true)
		e.writeEndpoint(heart.port())
		c := e.serve(args...)
		require.Zero(t, errCode(c.call(initLine)))
		dir := listedFolder(t, c.call(toolsLine))
		c.close()
		return dir
	}
	isTemp := func(t *testing.T, dir string) {
		t.Helper()
		require.True(t, filepath.IsAbs(dir), dir)
		assert.True(t, strings.HasPrefix(filepath.Base(dir), "anytype-mcp-files-"), dir)
	}

	t.Run("the flag names the folder; the log says so", func(t *testing.T) {
		e := newEnv(t)
		dir := filepath.Join(e.home, "ai files")
		heart := newHeartStub(t)
		heart.tools.Store(true)
		e.writeEndpoint(heart.port())
		c := e.serve("--files-dir", dir)
		require.Zero(t, errCode(c.call(initLine)))
		// the folder is named as pinned: with the links in its path resolved
		pinned, err := filepath.EvalSymlinks(dir)
		require.NoError(t, err)
		assert.Equal(t, pinned, listedFolder(t, c.call(toolsLine)))
		c.close()
		assert.Contains(t, c.stderr.String(), "file tools: folder "+pinned)
	})

	t.Run("the variable: empty and unresolved are unset, a folder is used, the flag wins", func(t *testing.T) {
		e := newEnv(t)
		for _, value := range []string{"", "${user_config.files_dir}"} {
			e.extra = []string{"ANYTYPE_MCP_FILES_DIR=" + value}
			isTemp(t, listed(t, e))
		}
		fromEnv := filepath.Join(e.home, "from env")
		e.extra = []string{"ANYTYPE_MCP_FILES_DIR=" + fromEnv}
		pinned := func(dir string) string {
			p, err := filepath.EvalSymlinks(dir)
			require.NoError(t, err)
			return p
		}
		got := listed(t, e)
		assert.Equal(t, pinned(fromEnv), got)
		fromFlag := filepath.Join(e.home, "from flag")
		got = listed(t, e, "--files-dir", fromFlag)
		assert.Equal(t, pinned(fromFlag), got)
		// the flag wins even over a variable that would fail
		e.extra = []string{"ANYTYPE_MCP_FILES_DIR=relative"}
		assert.Equal(t, pinned(fromFlag), listed(t, e, "--files-dir", fromFlag))
	})

	t.Run("without either every process gets a new private temporary folder, left behind empty", func(t *testing.T) {
		e := newEnv(t)
		var dirs []string
		for range 2 {
			dir := listed(t, e)
			isTemp(t, dir)
			fi, err := os.Lstat(dir)
			require.NoError(t, err, "the folder outlives the process")
			if runtime.GOOS != "windows" {
				assert.Equal(t, os.FileMode(0o700), fi.Mode().Perm())
			}
			list, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Empty(t, list)
			dirs = append(dirs, dir)
		}
		assert.NotEqual(t, dirs[0], dirs[1])
	})

	t.Run("a curated tier lists no file tools and makes no folder", func(t *testing.T) {
		e := newEnv(t)
		heart := newHeartStub(t)
		heart.tools.Store(true)
		heart.session.Store(true)
		e.writeEndpoint(heart.port())
		c := e.serve("--tier", "small")
		require.Zero(t, errCode(c.call(initLine)))
		assert.Empty(t, listedFolder(t, c.call(toolsLine)))
		c.close()
		matches, err := filepath.Glob(filepath.Join(e.home, "anytype-mcp-files-*"))
		require.NoError(t, err)
		assert.Empty(t, matches)
	})
}
