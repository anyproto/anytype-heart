package state

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestParentIndex_DirectChildrenWriteWhitelist guards the parent index's core
// invariant: every code path that inserts an id into a ChildrenIds list while
// the index can be active (the BuildState replay window) must either go
// through the asserting helpers (Add/Set/prepend/append/insertChildrenIds),
// update the index itself, or run with the index disabled.
//
// If this test fails you added a direct ChildrenIds write to the state
// package. Either route it through the helpers, or — if the direct write is
// genuinely needed — make it index-safe (assert inserted ids / remove dropped
// ids / DisableParentIndex) and add the line to the whitelist below. Getting
// this wrong breaks unlinkForReplay's "no entry ⇒ no parent reference" claim
// and can silently duplicate blocks on change replay (see parentindex.go).
func TestParentIndex_DirectChildrenWriteWhitelist(t *testing.T) {
	whitelist := map[string][]string{
		// changeBuilder mutates a wire-bound copy, never a state block
		"change.go": {
			"m.ChildrenIds = nil",
		},
		// normalization runs with the index force-disabled (see Normalize)
		"normalize.go": {
			"d1.ChildrenIds = append(d1.ChildrenIds, d2.ChildrenIds...)",
		},
		"position.go": {
			// the chokepoint itself
			"parent.ChildrenIds = childrenIds",
			// updates the index
			"parent.ChildrenIds = slice.RemoveMut(parent.ChildrenIds, childrenId)",
			// wrapToRow slot write: hands the entry over and asserts
			"parent.Model().ChildrenIds[pos] = row.Model().Id",
		},
		// UnlinkAll fallback scan: leaves stale entries on purpose — they are
		// rejected by lookup verification (verified-cache model)
		"state.go": {
			"pm.ChildrenIds = slice.RemoveMut(pm.ChildrenIds, id)",
		},
	}

	writeRe := regexp.MustCompile(`ChildrenIds(\[[^\]]*\])?\s*=[^=]`)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || !writeRe.MatchString(trimmed) {
				continue
			}
			var allowed bool
			for _, w := range whitelist[name] {
				if strings.Contains(trimmed, w) {
					allowed = true
					break
				}
			}
			if !allowed {
				t.Errorf("%s:%d: direct ChildrenIds write outside the parent-index whitelist: %s", name, i+1, trimmed)
			}
		}
	}
}
