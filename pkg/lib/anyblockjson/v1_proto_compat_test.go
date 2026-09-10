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

			if filepath.Base(file.heart) == "models.proto" {
				heartNormalized = withoutExportReportAPI(heartNormalized)
				canonicalNormalized = withoutExportReportAPI(canonicalNormalized)
			}

			if filepath.Base(file.heart) == "events.proto" {
				heartNormalized = withoutAccountRecoveryAPI(heartNormalized)
				canonicalNormalized = withoutAccountRecoveryAPI(canonicalNormalized)
			}
			heartNormalized = withoutIntegrationMetadata(filepath.Base(file.heart), heartNormalized)
			canonicalNormalized = withoutIntegrationMetadata(filepath.Base(file.heart), canonicalNormalized)

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

// Grants and pairing prompts belong to Heart's live authentication API. Change
// provenance is stored by Heart but is not consumed by the snapshot codec. Keep
// these deliberate differences explicit: exact declarations are removed or
// translated, so an unexpected field type, number, or snapshot change still fails
// the canonical-source comparison.
func withoutIntegrationMetadata(file, normalized string) string {
	switch file {
	case "models.proto":
		normalized = strings.ReplaceAll(normalized, `AppGrantgrant=9;`, "")
		normalized = strings.ReplaceAll(normalized, `messageAppGrant{repeatedstringspaceIds=1;Permperm=2;boolallSpaces=3;enumPerm{Read=0;ReadWrite=1;}}`, "")
	case "events.proto":
		normalized = strings.NewReplacer(
			`Account.LinkApprovalRequestaccountLinkApprovalRequest=204;`, `Account.LinkChallengeaccountLinkChallenge=204;`,
			`Account.LinkApprovalHideaccountLinkApprovalHide=205;`, `Account.LinkChallengeHideaccountLinkChallengeHide=205;`,
			`messageLinkApprovalRequest{messageClientInfo{stringprocessName=1;stringprocessPath=2;stringname=4;boolsignatureVerified=3;stringorigin=5;}reserved1;reserved"challenge";ClientInfoclientInfo=2;model.Account.Auth.LocalApiScopescope=3;reserved4;reserved"requestedGrant";model.Account.Auth.AppGrant.PermrequestedPerm=5;}`,
			`messageLinkChallenge{messageClientInfo{stringprocessName=1;stringprocessPath=2;stringname=4;boolsignatureVerified=3;}stringchallenge=1;ClientInfoclientInfo=2;model.Account.Auth.LocalApiScopescope=3;}`,
			`messageLinkApprovalHide{reserved1;reserved"challenge";LinkApprovalRequest.ClientInfoclientInfo=2;}`,
			`messageLinkChallengeHide{stringchallenge=1;}`,
		).Replace(normalized)
	case "changes.proto":
		normalized = strings.ReplaceAll(normalized, `uint32changeType=9;stringintegrationName=10;`, `uint32changeType=9;`)
	}
	return normalized
}

func TestIntegrationMetadataExclusionPreservesSchemaChecks(t *testing.T) {
	for _, tc := range []struct {
		file, canonical, extended, unexpected string
	}{
		{
			"models.proto",
			`messageAppInfo{boolisActive=8;}`,
			`messageAppInfo{boolisActive=8;AppGrantgrant=9;}`,
			`messageAppInfo{boolisActive=7;AppGrantgrant=9;}`,
		},
		{
			"events.proto",
			`messageLinkChallengeHide{stringchallenge=1;}`,
			`messageLinkApprovalHide{reserved1;reserved"challenge";LinkApprovalRequest.ClientInfoclientInfo=2;}`,
			`messageLinkApprovalHide{reserved1;reserved"challenge";LinkApprovalRequest.ClientInfoclientInfo=3;}`,
		},
		{
			"changes.proto",
			`messageChange{uint32changeType=9;}`,
			`messageChange{uint32changeType=9;stringintegrationName=10;}`,
			`messageChange{uint32changeType=9;stringintegrationName=11;}`,
		},
	} {
		t.Run(tc.file, func(t *testing.T) {
			if got := withoutIntegrationMetadata(tc.file, tc.extended); got != tc.canonical {
				t.Fatalf("expected the explicit Heart extension to normalize: %s", got)
			}
			if withoutIntegrationMetadata(tc.file, tc.unexpected) == tc.canonical {
				t.Fatal("normalization hid an unexpected field number")
			}
			if withoutIntegrationMetadata(tc.file, tc.canonical) != tc.canonical {
				t.Fatal("normalization changed the canonical schema")
			}
		})
	}
}

// Export diagnostics are Heart RPC/notification metadata, never part of a v1
// snapshot. Allow only these additive API fields and their separate proto import;
// every other definition (including the notification's existing fields) must
// still match the pinned canonical source.
func withoutExportReportAPI(normalized string) string {
	normalized = strings.ReplaceAll(normalized, `import"pkg/lib/pb/model/protos/export_report.proto";`, "")
	return strings.ReplaceAll(normalized, `model.Export.FormatexportType=3;ExportReportreport=4;stringpath=5;`, `model.Export.FormatexportType=3;`)
}

// Account recovery progress is a live API stream added on develop, not a v1
// snapshot event. Exclude only its new payload and Account.Recovery namespace;
// existing account events and all persisted block/object definitions still match.
func withoutAccountRecoveryAPI(normalized string) string {
	normalized = strings.ReplaceAll(normalized, `Account.Recovery.UpdateaccountRecoveryUpdate=206;`, "")
	account := strings.Index(normalized, "messageAccount{")
	if account < 0 {
		return normalized
	}
	accountEnd := normalizedMessageEnd(normalized, account)
	if accountEnd < 0 {
		return normalized
	}
	recovery := strings.Index(normalized[account:accountEnd], "messageRecovery{")
	if recovery < 0 {
		return normalized
	}
	recovery += account
	end := normalizedMessageEnd(normalized, recovery)
	if end < 0 || end > accountEnd {
		return normalized
	}
	return normalized[:recovery] + normalized[end:]
}

// Return the offset after a normalized message's closing brace. Quoted defaults
// can contain braces, so only structural braces change the nesting depth.
func normalizedMessageEnd(source string, start int) int {
	depth := 0
	quoted, escaped := false, false
	for n := start; n < len(source); n++ {
		c := source[n]
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		switch c {
		case '"':
			quoted = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return n + 1
			}
		}
	}
	return -1
}

func TestAccountRecoveryExclusionPreservesExistingSchemaChecks(t *testing.T) {
	canonical := `messageEvent{messageMessage{Account.UpdateaccountUpdate=203;}messageAccount{messageUpdate{stringname=1;}}messageObject{messageRecovery{stringid=1;}}}`
	withRecovery := strings.Replace(canonical, `Account.UpdateaccountUpdate=203;`, `Account.UpdateaccountUpdate=203;Account.Recovery.UpdateaccountRecoveryUpdate=206;`, 1)
	withRecovery = strings.Replace(withRecovery, `messageAccount{`, `messageAccount{messageRecovery{messageUpdate{stringphase=1[default="}"];}}`, 1)
	if got := withoutAccountRecoveryAPI(withRecovery); got != canonical {
		t.Fatalf("recovery exclusion changed an existing definition: %s", got)
	}
	changed := strings.Replace(withRecovery, "stringname=1;", "stringname=2;", 1)
	if withoutAccountRecoveryAPI(changed) == canonical {
		t.Fatal("recovery exclusion hid a changed existing field number")
	}
	if withoutAccountRecoveryAPI(canonical) != canonical {
		t.Fatal("recovery exclusion changed the canonical schema")
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
