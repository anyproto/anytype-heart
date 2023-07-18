package debug

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const logsPath = "logs"

func Stack(allGoroutines bool) []byte {
	buf := make([]byte, 1024)
	for {
		n := runtime.Stack(buf, allGoroutines)
		if n < len(buf) {
			return buf[:n]
		}
		buf = make([]byte, 2*len(buf))
	}
}

// StackCompact returns base64 of gzipped stack
func StackCompact(allGoroutines bool) string {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write(Stack(allGoroutines))
	_ = gz.Close()

	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// SaveStackToRepo collects current stack of goroutines and saves it into /logs folder inside provided directory
func SaveStackToRepo(repoPath string, allGoroutines bool) error {
	dirPath := filepath.Join(repoPath, logsPath)
	if err := os.Mkdir(dirPath, 0777); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to create /logs directory: %v", err)
	}
	filePath := filepath.Join(dirPath, fmt.Sprintf("stack.%s.log", time.Now().Format("20060102.150405.99")))
	stack := Stack(allGoroutines)
	if err := os.WriteFile(filePath, stack, 0644); err != nil {
		return fmt.Errorf("failed to write stacktrace to file: %v", err)
	}
	return nil
}
