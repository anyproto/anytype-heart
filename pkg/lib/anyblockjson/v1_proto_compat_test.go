package anyblockjson

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode"
)

const anyBlockModule = "github.com/anyproto/any-block"

var (
	goPackageOption = regexp.MustCompile(`(?m)\boption\s+go_package\s*=\s*"[^"]*"\s*;`)
	v1ProtoImport   = regexp.MustCompile(`"(?:[^"]*/)?(models|events|changes|snapshot)\.proto"`)
)

func TestV1ProtosMatchAnyBlockCanonicalSources(t *testing.T) {
	heartRoot := repositoryRoot(t)
	anyBlockRoot := moduleRoot(t, heartRoot, anyBlockModule)

	files := []struct {
		heart    string
		anyBlock string
	}{
		{"pkg/lib/pb/model/protos/models.proto", "format/v1/proto/models.proto"},
		{"pb/protos/events.proto", "format/v1/proto/events.proto"},
		{"pb/protos/changes.proto", "format/v1/proto/changes.proto"},
		{"pb/protos/snapshot.proto", "format/v1/proto/snapshot.proto"},
	}

	for _, file := range files {
		t.Run(filepath.Base(file.heart), func(t *testing.T) {
			heartProto := readProto(t, filepath.Join(heartRoot, file.heart))
			canonicalProto := readProto(t, filepath.Join(anyBlockRoot, file.anyBlock))

			heartNormalized, err := normalizeProto(heartProto)
			if err != nil {
				t.Fatalf("normalize Heart proto %s: %v", file.heart, err)
			}
			canonicalNormalized, err := normalizeProto(canonicalProto)
			if err != nil {
				t.Fatalf("normalize AnyBlock proto %s: %v", file.anyBlock, err)
			}

			if heartNormalized != canonicalNormalized {
				t.Fatalf(
					"Heart proto %s has drifted from %s/%s (normalized SHA-256 %x != %x); synchronize the canonical AnyBlock v1 source and Heart mirror together",
					file.heart,
					anyBlockModule,
					file.anyBlock,
					sha256.Sum256([]byte(heartNormalized)),
					sha256.Sum256([]byte(canonicalNormalized)),
				)
			}
		})
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "../../.."))
}

func moduleRoot(t *testing.T, heartRoot, module string) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m", "-f={{.Dir}}", module)
	cmd.Dir = heartRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve %s module directory: %v\n%s", module, err, out)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		t.Fatalf("resolve %s module directory: go list returned an empty path", module)
	}
	return root
}

func readProto(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func normalizeProto(source string) (string, error) {
	withoutComments, err := stripProtoComments(source)
	if err != nil {
		return "", err
	}
	withoutGoPackage := goPackageOption.ReplaceAllString(withoutComments, "")
	normalizedImports := v1ProtoImport.ReplaceAllString(withoutGoPackage, `"$1.proto"`)
	return stripWhitespaceOutsideStrings(normalizedImports)
}

func stripProtoComments(source string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(source); {
		switch {
		case source[i] == '"':
			start := i
			closed := false
			i++
			for i < len(source) {
				if source[i] == '\\' {
					if i+1 >= len(source) {
						return "", fmt.Errorf("unterminated string literal")
					}
					i += 2
					continue
				}
				if source[i] == '"' {
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return "", fmt.Errorf("unterminated string literal")
			}
			out.WriteString(source[start:i])
		case i+1 < len(source) && source[i:i+2] == "//":
			i += 2
			for i < len(source) && source[i] != '\n' {
				i++
			}
		case i+1 < len(source) && source[i:i+2] == "/*":
			i += 2
			end := strings.Index(source[i:], "*/")
			if end < 0 {
				return "", fmt.Errorf("unterminated block comment")
			}
			i += end + 2
		default:
			out.WriteByte(source[i])
			i++
		}
	}
	return out.String(), nil
}

func stripWhitespaceOutsideStrings(source string) (string, error) {
	var out strings.Builder
	inString := false
	escaped := false
	for _, r := range source {
		if inString {
			out.WriteRune(r)
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
			} else if r == '"' {
				inString = false
			}
			continue
		}
		if r == '"' {
			inString = true
			out.WriteRune(r)
		} else if !unicode.IsSpace(r) {
			out.WriteRune(r)
		}
	}
	if inString {
		return "", fmt.Errorf("unterminated string literal")
	}
	return out.String(), nil
}
