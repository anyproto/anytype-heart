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

func TestServeFilesDir(t *testing.T) {
	t.Run("a value that is not an absolute path is a usage error", func(t *testing.T) {
		e := newEnv(t)
		for _, value := range []string{"", "files", "./files", "../files", "~/files"} {
			r := e.run("", "serve", "--connection", "conn-a", "--files-dir", value)
			assert.Equal(t, exitUsage, r.code, value)
			assert.Contains(t, r.stderr, "--files-dir must be an absolute path")
			assert.Empty(t, r.stdout)
		}
		assert.Equal(t, exitUsage, e.run("", "serve", "--connection", "conn-a", "--files-dir").code)
		assert.Equal(t, exitUsage, e.run("", "key", "check", "--connection", "conn-a", "--files-dir", e.home).code, "serve only")
	})

	t.Run("a folder that breaks a rule ends serve before it reads anything, naming the rule", func(t *testing.T) {
		e := newEnv(t)
		real := filepath.Join(e.home, "real")
		require.NoError(t, os.Mkdir(real, 0o700))
		broken := map[string]string{}
		if err := os.Symlink(real, filepath.Join(e.home, "link")); err == nil {
			broken[filepath.Join(e.home, "link")] = "not a real directory"
		}
		file := filepath.Join(e.home, "file")
		require.NoError(t, os.WriteFile(file, nil, 0o600))
		broken[file] = "not a real directory"
		if runtime.GOOS != "windows" {
			shared := filepath.Join(e.home, "shared")
			require.NoError(t, os.Mkdir(shared, 0o700))
			require.NoError(t, os.Chmod(shared, 0o777))
			broken[shared] = "writable by other users"
			if os.Geteuid() != 0 {
				broken["/"] = "belongs to another user"
			}
		}
		for dir, rule := range broken {
			r := e.run(initLine+"\n", "serve", "--connection", "conn-a", "--files-dir", dir)
			assert.Equal(t, exitInvalidInput, r.code, dir)
			assert.Contains(t, r.stderr, "--files-dir: ")
			assert.Contains(t, r.stderr, rule, dir)
			assert.Empty(t, r.stdout)
		}
	})

	t.Run("the folder is never where the connection keys live, nor around it", func(t *testing.T) {
		e := newEnv(t)
		require.Equal(t, exitOK, e.set("absent", canary, "A", 1, "full").code)
		for _, dir := range []string{e.home, filepath.Join(e.home, ".anytype"), e.items(), filepath.Join(e.home, ".anytype", "files"),
			e.home + string(filepath.Separator) + "x" + string(filepath.Separator) + ".." + string(filepath.Separator) + "y"} {
			r := e.run(initLine+"\n", "serve", "--connection", "conn-a", "--files-dir", dir)
			assert.Equal(t, exitInvalidInput, r.code, dir)
			assert.Regexp(t, `apart from Anytype's own folder|without ".."`, r.stderr, dir)
			assert.Empty(t, r.stdout)
		}
		_, err := os.Stat(filepath.Join(e.home, ".anytype", "files"))
		assert.True(t, os.IsNotExist(err) || err == nil)
		assert.Len(t, e.holders(), 1, "the key is in its item and nowhere else")
	})

	t.Run("a missing folder is created private, with or without a connection", func(t *testing.T) {
		e := newEnv(t)
		dir := filepath.Join(e.home, "ai", "files")
		r := e.run(initLine+"\n", "serve", "--connection", "conn-a", "--files-dir", dir)
		assert.Equal(t, exitOK, r.code)
		assert.Contains(t, r.stdout, `"code":-32001`)
		fi, err := os.Lstat(dir)
		require.NoError(t, err)
		assert.True(t, fi.IsDir())
		if runtime.GOOS != "windows" {
			assert.Equal(t, os.FileMode(0o700), fi.Mode().Perm())
		}
	})

	t.Run("the full tier's file tools name the folder given", func(t *testing.T) {
		e := newEnv(t)
		heart := newHeartStub(t)
		heart.tools.Store(true)
		require.Equal(t, exitOK, e.set("absent", canary, "A", heart.port(), "full").code)
		dir := filepath.Join(e.home, "ai files")
		c := e.serve("--files-dir", dir)
		require.Zero(t, errCode(c.call(initLine)))
		assert.Equal(t, dir, listedFolder(t, c.call(toolsLine)))
		require.NoError(t, c.stdin.Close())
		assert.Equal(t, 0, c.wait())
		assert.Contains(t, c.stderr.String(), "file tools: folder "+dir)
		assert.NotContains(t, c.stderr.String(), canary)
	})

	t.Run("without the flag every process gets a new private temporary folder, left behind empty", func(t *testing.T) {
		e := newEnv(t)
		heart := newHeartStub(t)
		heart.tools.Store(true)
		require.Equal(t, exitOK, e.set("absent", canary, "A", heart.port(), "full").code)
		var dirs []string
		for range 2 {
			c := e.serve()
			require.Zero(t, errCode(c.call(initLine)))
			dir := listedFolder(t, c.call(toolsLine))
			require.NoError(t, c.stdin.Close())
			assert.Equal(t, 0, c.wait())
			require.True(t, filepath.IsAbs(dir), dir)
			assert.True(t, strings.HasPrefix(filepath.Base(dir), "anytype-mcp-files-"), dir)
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
		require.Equal(t, exitOK, e.set("absent", canary, "A", heart.port(), "small").code)
		c := e.serve()
		require.Zero(t, errCode(c.call(initLine)))
		assert.Empty(t, listedFolder(t, c.call(toolsLine)))
		require.NoError(t, c.stdin.Close())
		assert.Equal(t, 0, c.wait())
		matches, err := filepath.Glob(filepath.Join(e.home, "anytype-mcp-files-*"))
		require.NoError(t, err)
		assert.Empty(t, matches)
	})
}
