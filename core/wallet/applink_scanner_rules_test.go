package wallet

// applink_scanner_rules_test.go pins the shipped secret-scanner rules
// (docs/secret-scanning/) against the key format this package mints: the
// rules must match a real minted key wherever it plausibly leaks, must
// reject the `anytype_…` identifiers that occur in this repo's own code and
// prose (a prefix-only rule would be permanent allowlist maintenance), and
// must not drift from the published range pattern — or from each other.
// Deliberately no full-shape key literal appears in this file: probe keys
// are minted or assembled at runtime, so the rules never flag their own
// test.

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// publishedAppKeyPattern is the range pattern the design spec publishes
// (P1 §1b): a RANGE for the body, never a fixed length, so a future body
// encoding cannot silently defeat third-party rules.
const publishedAppKeyPattern = `\banytype_[0-9A-Za-z]{40,60}_[0-9a-f]{8}\b`

// scannerRulePattern extracts the one rule regex from a shipped scanner
// config with a syntax-level probe — the test must fail if the file is
// restructured, rather than silently pinning nothing.
func scannerRulePattern(t *testing.T, file, capture string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "secret-scanning", file))
	require.NoError(t, err, "the shipped scanner rule must exist")
	matches := regexp.MustCompile(capture).FindStringSubmatch(string(raw))
	require.Len(t, matches, 2, "exactly one rule pattern expected in %s", file)
	return matches[1]
}

func TestSecretScannerRules(t *testing.T) {
	gitleaksPattern := scannerRulePattern(t, "gitleaks.toml", `regex = '''(.+)'''`)
	trufflehogPattern := scannerRulePattern(t, "trufflehog.yaml", `api_key: '(.+)'`)

	t.Run("both rules carry the published range pattern", func(t *testing.T) {
		assert.Equal(t, publishedAppKeyPattern, gitleaksPattern)
		assert.Equal(t, publishedAppKeyPattern, trufflehogPattern, "the two rules must never drift apart")
	})

	rule := regexp.MustCompile(gitleaksPattern)

	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	mintedKey := formatAppKey(raw)

	t.Run("a real minted key matches wherever it plausibly leaks", func(t *testing.T) {
		for _, leak := range []string{
			mintedKey,
			fmt.Sprintf(`"apiKey": "%s"`, mintedKey),               // MCP/CLI JSON config
			"Authorization: Bearer " + mintedKey,                   // a pasted curl trace
			"ANYTYPE_API_KEY=" + mintedKey,                         // a dotenv line
			fmt.Sprintf("the key (%s) stopped working", mintedKey), // prose
		} {
			assert.Equal(t, mintedKey, rule.FindString(leak), "must recover the key from %q", leak)
		}
	})

	t.Run("the repo's own anytype_ identifiers never match", func(t *testing.T) {
		for _, identifier := range []string{
			// every one of these occurs in this repository's code or prose
			"anytype_mcp",
			"anytype_profile_",
			"anytype_profile_settings",
			"anytype_backup",
			"anytype_downloaded_file_",
			"anytype_marketplace",
			"anytype_notion_import",
			"anytype_old",
			"anytype_", // the bare prefix — matching it is the failure mode the full pattern exists to avoid
		} {
			assert.Empty(t, rule.FindString(identifier), "identifier %q must not match", identifier)
		}
		prose := "the anytype_mcp server reads anytype_profile_ settings from anytype_backup"
		assert.Empty(t, rule.FindString(prose))
	})

	t.Run("near-keys are rejected", func(t *testing.T) {
		for name, nearKey := range map[string]string{
			"truncated checksum": mintedKey[:len(mintedKey)-1],
			"no checksum part":   mintedKey[:strings.LastIndex(mintedKey, "_")],
			"uppercase checksum": mintedKey[:len(mintedKey)-8] + "DEADBEEF",
			// a base64-alphabet body would break \b anchoring mid-token —
			// the format bans +, / and = precisely for the scanners' sake
			"base64 body": "anytype_" + strings.Repeat("a", 20) + "+" + strings.Repeat("b", 20) + "_00000000",
			"short body":  "anytype_" + strings.Repeat("a", 39) + "_00000000",
			"long body":   "anytype_" + strings.Repeat("a", 61) + "_00000000",
		} {
			assert.Empty(t, rule.FindString(nearKey), "%s: %q must not match", name, nearKey)
		}
	})

	t.Run("legacy-format keys are invisible to the rule, by design", func(t *testing.T) {
		// std-base64 keys carry no prefix; the rule cannot and does not try
		// to find them. Attrition retires the legacy JSON-API population,
		// but Limited/gRPC (clipper) credentials KEEP minting unprefixed
		// (generate() formats only JsonAPI-scope keys), so this blind spot
		// is permanent until those keys get their own prefix — recorded in
		// docs/secret-scanning/README.md.
		legacy := "qX8jP2mN5vK9wL3tR7yU1iO6eA4sD8fG0hJ2kZ5xC7vB9nM1qW3e"
		assert.Empty(t, rule.FindString(legacy))
	})

	t.Run("every full-shape match in the tracked tree is allowlisted", func(t *testing.T) {
		// The property a detection-only deliverable lives or dies by: the
		// shipped rule comes back CLEAN on the very repo that ships it. The
		// doc example key is committed in the swagger tag AND in the OpenAPI
		// documents generated from it, so the allowlist must keep covering
		// every copy as the docs regenerate — this walk makes that CI's
		// problem instead of the first adopter's.
		repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
		require.NoError(t, err)
		allowlist := shippedGitleaksAllowlist(t)
		require.NotEmpty(t, allowlist, "the shipped rule must carry its allowlist")

		out, err := exec.Command("git", "-C", repoRoot, "ls-files", "-z").Output()
		require.NoError(t, err, "the allowlist walk needs a git checkout")

		hits := map[string]bool{}
		for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
			if rel == "" {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
			if err != nil {
				continue // tracked but locally deleted
			}
			if !bytes.Contains(raw, []byte("anytype_")) {
				continue // the rule's own keyword gate
			}
			if rule.Find(raw) == nil {
				continue
			}
			hits[rel] = true
			allowed := false
			for _, allow := range allowlist {
				if allow.MatchString(rel) {
					allowed = true
					break
				}
			}
			assert.True(t, allowed,
				"%s carries a full-shape key match the shipped allowlist does not cover — a scan of this repo would flag it", rel)
		}
		// the walk must actually see the known example sites, or a broken
		// walk (wrong root, wrong pattern) would pass vacuously
		assert.True(t, hits["core/api/model/auth.go"], "the swagger example key must be visible to the walk")
	})
}

// shippedGitleaksAllowlist compiles the path allowlist from the shipped
// gitleaks rule, with the same syntax-level probe as scannerRulePattern —
// restructuring the file must fail the test, not silently pin nothing.
func shippedGitleaksAllowlist(t *testing.T) []*regexp.Regexp {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "secret-scanning", "gitleaks.toml"))
	require.NoError(t, err, "the shipped scanner rule must exist")
	// the closing bracket sits on its own line — a lone \] would stop at
	// the first ] INSIDE an entry's character class (e.g. v[12])
	block := regexp.MustCompile(`(?s)paths = \[(.*?)\n\]`).FindStringSubmatch(string(raw))
	require.Len(t, block, 2, "exactly one allowlist paths block expected")
	var allowlist []*regexp.Regexp
	for _, entry := range regexp.MustCompile(`'''(.+?)'''`).FindAllStringSubmatch(block[1], -1) {
		allowlist = append(allowlist, regexp.MustCompile(entry[1]))
	}
	return allowlist
}
