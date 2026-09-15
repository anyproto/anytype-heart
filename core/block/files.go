package block

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/miolini/datacounter"

	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/anyerror"
)

// TODO Move residual file methods here

// TODO Extract to a new service FileDownloader
func (s *Service) DownloadFile(ctx context.Context, req *pb.RpcFileDownloadRequest) (string, error) {
	targetDir, err := s.downloadTargetDir(req.Path)
	if err != nil {
		return "", err
	}

	err = os.MkdirAll(targetDir, 0755)
	if err != nil {
		return "", fmt.Errorf("mkdir -p: %w", anyerror.CleanupError(err))
	}
	progress := process.NewProgress(&pb.ModelProcessMessageOfSaveFile{SaveFile: &pb.ModelProcessSaveFile{}})
	defer progress.Finish(nil)

	err = s.ProcessAdd(progress)
	if err != nil {
		return "", fmt.Errorf("add process: %w", err)
	}

	progress.SetProgressMessage("saving file")
	var countReader *datacounter.ReaderCounter
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-progress.Canceled():
				cancel()
			case <-time.After(time.Second):
				if countReader != nil {
					progress.SetDone(int64(countReader.Count()))
				}
			}
		}
	}()

	f, err := s.fileObjectService.GetFileData(ctx, req.ObjectId)
	if err != nil {
		return "", fmt.Errorf("get file by hash: %w", err)
	}

	progress.SetTotal(f.Meta().Size)

	r, err := f.Reader(rpcstore.ContextWithWaitAvailable(ctx))
	if err != nil {
		return "", fmt.Errorf("get file reader: %w", err)
	}
	countReader = datacounter.NewReaderCounter(r)
	fileName := f.Meta().Name
	if fileName == "" {
		fileName = f.Name()
	}

	path, err := files.WriteReaderIntoFileReuseSameExistingFile(filepath.Join(targetDir, downloadFileName(fileName)), countReader)
	if err != nil {
		return "", fmt.Errorf("save file: %w", err)
	}

	progress.SetDone(f.Meta().Size)
	return path, nil
}

// downloadTargetDir decides which directory a download may be written to.
//
// In gRPC-server mode the caller is untrusted, so a caller-chosen destination
// is confined to the temp scope: a caller that reached the port cannot aim the
// write at a shell rc file or a LaunchAgent. Mobile runs in-process — the
// caller is the app itself — so it carries no scope and its path is honored.
func (s *Service) downloadTargetDir(reqPath string) (string, error) {
	if reqPath == "" {
		reqPath = filepath.Join(s.tempDirProvider.TempDir(), "anytype-download")
	}
	if s.downloadScopeDir == "" {
		return reqPath, nil
	}
	return ensureWithinScope(s.downloadScopeDir, reqPath)
}

// downloadFileName confines a file name to a single path element. The name
// comes from file metadata, which whoever shared the file controls, so it must
// not be able to walk out of the directory the scope check just approved.
func downloadFileName(name string) string {
	base := filepath.Base(name)
	switch base {
	case ".", "..", string(os.PathSeparator):
		return "file"
	}
	return base
}

// ensureWithinScope confirms dir is scopeDir or a descendant of it, returning
// the fully resolved path to write to. It is the sandbox check the gRPC server
// applies to caller-supplied download paths so a reachable-but-untrusted caller
// cannot direct a write outside the temp scope. Mobile runs in-process with a
// trusted caller, passes an empty scope, and never reaches this.
//
// Both sides are symlink-resolved before they are compared: the temp location
// is shared with other local users, so a textual prefix check alone would let
// a planted link inside the scope redirect the write anywhere. Resolving also
// reconciles the two spellings of the same directory that platforms hand out
// (macOS /var is a symlink to /private/var), which a textual check would
// reject.
//
// The resolved path is what the caller must write to. Resolving does not close
// the window between this check and the write — a component swapped for a
// symlink in between still redirects it — but that race needs a local attacker
// with write access to the scope and precise timing, where the pre-fix state
// needed neither.
func ensureWithinScope(scopeDir, dir string) (string, error) {
	scope, err := resolvePath(scopeDir)
	if err != nil {
		return "", fmt.Errorf("resolve scope dir: %w", err)
	}

	target, err := resolvePath(dir)
	if err != nil {
		return "", fmt.Errorf("resolve download dir: %w", err)
	}

	if target != scope && !strings.HasPrefix(target, scope+string(os.PathSeparator)) {
		return "", fmt.Errorf("download path %q is outside the allowed directory %q", target, scope)
	}
	return target, nil
}

// resolvePath makes path absolute and follows every symlink along it. The leaf
// normally does not exist yet — DownloadFile creates it — so it resolves the
// longest existing ancestor and re-appends the components below it.
func resolvePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	current := filepath.Clean(abs)
	remainder := ""
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(resolved, remainder), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached the root without finding anything that exists; nothing
			// is left to resolve.
			return filepath.Join(current, remainder), nil
		}
		remainder = filepath.Join(filepath.Base(current), remainder)
		current = parent
	}
}

// downloadScopeForGOOS returns the directory FileDownload writes must stay
// within, or "" to disable the sandbox. Mobile (ios/android) calls the
// middleware in-process from the trusted app and keeps writing to the
// caller-chosen path; every other platform runs the gRPC server, where the
// caller is untrusted, so writes are confined to the temp dir.
func downloadScopeForGOOS(goos, tempDir string) string {
	switch goos {
	case "ios", "android":
		return ""
	default:
		return tempDir
	}
}
