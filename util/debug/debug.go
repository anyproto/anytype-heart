package debug

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const logsPath = "logs"

var (
	startLinePattern = regexp.MustCompile(`^goroutine\s+(\d+)\s+\[(.*)\]:$`)
	addressPattern   = regexp.MustCompile(`\+?0x[0-9a-z]*`)
)

func ParseGoroutinesDump(trace string, pattern string) string {
	var sb strings.Builder

	scanner := bufio.NewScanner(strings.NewReader(trace))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	var lineCount int
	for scanner.Scan() {
		line := scanner.Text()

		if startLinePattern.MatchString(line) {
			results := startLinePattern.FindAllStringSubmatch(line, -1)
			sb.WriteString(results[0][2])
			sb.WriteString(" ")
			lineCount++
		} else if line == "" {
			sb.Reset()
			lineCount = 0
		} else {
			if lineCount < 3 {
				sb.WriteString(strings.Replace(addressPattern.ReplaceAllString(line, ""), "\t", "", -1))
				sb.WriteString(" ")
			}
			if strings.Contains(line, pattern) {
				return strings.Trim(sb.String(), " ")
			}
			lineCount++
		}
	}
	return ""
}

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
	return CompressBytes(Stack(allGoroutines))
}

func CompressBytes(s []byte) string {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write(s)
	_ = gz.Close()

	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// SaveStackToRepo collects current stack of goroutines and saves it into /logs folder inside provided directory
func SaveStackToRepo(repoPath string, allGoroutines bool) error {
	dirPath := filepath.Join(repoPath, logsPath)
	if err := os.Mkdir(dirPath, 0777); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to create /logs directory: %w", err)
	}
	filePath := filepath.Join(dirPath, fmt.Sprintf("stack.%s.log", time.Now().Format("20060102.150405.99")))
	stack := Stack(allGoroutines)
	// nolint: gosec
	if err := os.WriteFile(filePath, stack, 0644); err != nil {
		return fmt.Errorf("failed to write stacktrace to file: %w", err)
	}
	return nil
}
