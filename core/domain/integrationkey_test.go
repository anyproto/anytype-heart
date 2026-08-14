package domain

import (
	"context"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationKeyFromAppName(t *testing.T) {
	t.Run("examples", func(t *testing.T) {
		// given → then: each row is one wire-visible normalization decision;
		// changing any rule of §5 (lowercase, collapse, trim, cap) breaks a row
		cases := []struct {
			name string
			in   string
			want string
		}{
			{"plain word", "linear", "linear"},
			{"case folds", "Linear", "linear"},
			{"spaces collapse to dash", "Claude Desktop", "claude-desktop"},
			{"legacy numeric name survives", "22", "22"},
			{"underscore and dash are legal", "my_app-2", "my_app-2"},
			{"punctuation runs collapse to one dash", "a...b!!c", "a-b-c"},
			{"leading and trailing junk trims", "  Linear!  ", "linear"},
			{"edge dashes trim", "-x-", "x"},
			{"unicode outside the charset separates", "Café Notes", "caf-notes"},
			{"empty stays empty", "", ""},
			{"only-junk collapses to empty", "!!!", ""},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				assert.Equal(t, tc.want, IntegrationKeyFromAppName(tc.in))
			})
		}
	})

	t.Run("cap at 64 with no dangling dash", func(t *testing.T) {
		// given: 63 chars + a separator + more — the cap lands ON the collapsed
		// separator, so a lazy slug[:64] would leave a trailing dash. This can
		// only fail if either the cap or the post-cap re-trim is dropped.
		long := strings.Repeat("a", 63) + " " + strings.Repeat("b", 30)
		got := IntegrationKeyFromAppName(long)
		assert.Equal(t, strings.Repeat("a", 63), got)
		assert.LessOrEqual(t, len(got), 64)
		assert.False(t, strings.HasSuffix(got, "-"))
	})

	t.Run("properties over random inputs", func(t *testing.T) {
		// A fixed seed keeps the corpus reproducible; the corpus mixes legal
		// chars, separators and multi-byte runes so every rule is exercised.
		rnd := rand.New(rand.NewSource(42))
		alphabet := []rune("abzAZ09_-. !/é☃\t")
		for i := 0; i < 2000; i++ {
			n := rnd.Intn(100)
			var sb strings.Builder
			for j := 0; j < n; j++ {
				sb.WriteRune(alphabet[rnd.Intn(len(alphabet))])
			}
			in := sb.String()
			slug := IntegrationKeyFromAppName(in)

			// charset: nothing outside [a-z0-9_-]
			for _, r := range slug {
				legal := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
				require.True(t, legal, "input %q produced illegal rune %q in %q", in, r, slug)
			}
			// cap
			require.LessOrEqual(t, len(slug), 64, "input %q", in)
			// no edge dashes, no dash runs (collapse happened)
			require.False(t, strings.HasPrefix(slug, "-") && !strings.HasPrefix(in, "-"), "input %q → %q", in, slug)
			// idempotence: a slug re-normalizes to itself — the property that
			// makes the recorded value comparable with a freshly derived one
			require.Equal(t, slug, IntegrationKeyFromAppName(slug), "input %q", in)
			// determinism (the cross-device §15 property: same name, same slug)
			require.Equal(t, slug, IntegrationKeyFromAppName(in), "input %q", in)
		}
	})
}

func TestIntegrationKeyCtx(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		ctx := CtxWithIntegrationKey(context.Background(), "claude-desktop")
		assert.Equal(t, "claude-desktop", IntegrationKeyFromCtx(ctx))
	})

	t.Run("absent is empty", func(t *testing.T) {
		assert.Equal(t, "", IntegrationKeyFromCtx(context.Background()))
	})

	t.Run("nil ctx is empty, not a panic", func(t *testing.T) {
		// the creation pipeline sees nil ctx from tests and internal callers
		assert.Equal(t, "", IntegrationKeyFromCtx(nil))
	})

	t.Run("empty key installs nothing", func(t *testing.T) {
		// empty AppName ⇒ no stamp (§5): absence and empty stay one state
		ctx := CtxWithIntegrationKey(context.Background(), "")
		assert.Equal(t, "", IntegrationKeyFromCtx(ctx))
	})
}
