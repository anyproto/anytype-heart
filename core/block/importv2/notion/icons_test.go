package notion

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func namedIconValue(name, color string) *iconValue {
	return &iconValue{Type: "icon", Icon: &notionNamedIcon{Name: name, Color: color}}
}

func TestNamedIcon(t *testing.T) {
	cases := []struct {
		name       string
		icon       *iconValue
		wantName   string
		wantOption int64
		wantOk     bool
	}{
		{"a name Anytype already has passes through", namedIconValue("rocket", "blue"), "rocket", 7, true},
		{"a Notion-only name resolves through the alias table", namedIconValue("chat", "blue"), "chatbubble", 7, true},
		{"the -alternate suffix is Notion's variant marker, not a name", namedIconValue("clock-alternate", "yellow"), "time", 2, true},
		{"the -line suffix likewise", namedIconValue("checkmark-line", "lightgray"), "checkmark-circle", 1, true},
		{"an unlisted variant of a listed name still resolves", namedIconValue("rocket-filled", "blue"), "rocket", 7, true},
		{"an unlisted compound falls back to its head word", namedIconValue("book-open-with-bookmark", "red"), "book", 4, true},
		{"one variant suffix ending in another still resolves", namedIconValue("checkmark-outline", "blue"), "checkmark-circle", 7, true},
		{"names are case- and separator-insensitive, as Notion documents", namedIconValue("MAP_PIN alternate", "green"), "location", 10, true},
		{"an unknown color still yields the icon, in grey", namedIconValue("rocket", "chartreuse"), "rocket", 1, true},
		{"a name with no Anytype counterpart is not representable", namedIconValue("quokka-riding-a-bicycle", "blue"), "", 0, false},
		{"an emoji icon is not a named icon", &iconValue{Type: "emoji", Emoji: "🔥"}, "", 0, false},
		{"a file icon is not a named icon", &iconValue{Type: "external", External: &struct {
			Url string `json:"url"`
		}{Url: "https://example.com/i.png"}}, "", 0, false},
		{"no icon at all", nil, "", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// when
			gotName, gotOption, gotOk := c.icon.namedIcon()

			// then
			assert.Equal(t, c.wantOk, gotOk)
			assert.Equal(t, c.wantName, gotName)
			assert.Equal(t, c.wantOption, gotOption)
		})
	}
}

func TestNotionIconOption(t *testing.T) {
	// Notion's icon palette against Anytype's ten icon options. Grey is the
	// fallback: a color we do not know must not lose the icon.
	cases := map[string]int64{
		"gray": 1, "lightgray": 1, "light_gray": 1, "default": 1, "": 1,
		"yellow": 2, "orange": 3, "brown": 3, "red": 4, "pink": 5,
		"purple": 6, "blue": 7, "green": 10, "not-a-color": 1,
	}
	for color, want := range cases {
		t.Run(color, func(t *testing.T) {
			assert.Equal(t, want, notionIconOption(color))
		})
	}
}

// TestNamedIconTargetsAreRealIcons keeps the alias table honest: a target
// outside Anytype's named-icon vocabulary renders as nothing at all, which is
// exactly the failure the table exists to prevent.
func TestNamedIconTargetsAreRealIcons(t *testing.T) {
	// IconName's own decoder is the vocabulary check — it rejects anything
	// outside the 390 constants the client ships a sprite for.
	valid := func(name string) bool {
		var icon apimodel.IconName
		return json.Unmarshal([]byte(strconv.Quote(name)), &icon) == nil
	}
	require.False(t, valid("quokka-riding-a-bicycle"), "the vocabulary check must actually reject")

	require.NotEmpty(t, notionIconNames)
	for notionName, anytypeName := range notionIconNames {
		assert.True(t, valid(anytypeName), "%q maps to %q, which is not an Anytype icon", notionName, anytypeName)
	}
}

func TestApplyDatabaseTypeIcon(t *testing.T) {
	planned := func() *domain.Details {
		details := domain.NewDetails()
		details.SetString(bundle.RelationKeyIconName, "document")
		details.SetInt64(bundle.RelationKeyIconOption, 3)
		return details
	}

	t.Run("a built-in Notion icon replaces the planned one", func(t *testing.T) {
		// given
		details := planned()

		// when
		left := applyDatabaseTypeIcon(details, namedIconValue("rocket", "blue"))

		// then
		assert.Nil(t, left, "nothing left for applyIcon to materialize")
		assert.Equal(t, "rocket", details.GetString(bundle.RelationKeyIconName))
		assert.Equal(t, int64(7), details.GetInt64(bundle.RelationKeyIconOption))
	})

	t.Run("an emoji icon clears the planned name so the emoji renders", func(t *testing.T) {
		// given — the client prefers a named icon over an emoji, so leaving
		// the planned name in place would hide the database's own icon
		details := planned()
		emoji := &iconValue{Type: "emoji", Emoji: "🔥"}

		// when
		left := applyDatabaseTypeIcon(details, emoji)

		// then
		assert.Same(t, emoji, left)
		assert.False(t, details.Has(bundle.RelationKeyIconName))
		assert.False(t, details.Has(bundle.RelationKeyIconOption))
	})

	t.Run("an icon Anytype cannot render leaves the planned one alone", func(t *testing.T) {
		// given — this is the regression: blanking the planned icon for an
		// icon we then fail to set left the type on the default glyph
		details := planned()

		// when
		left := applyDatabaseTypeIcon(details, namedIconValue("quokka-riding-a-bicycle", "blue"))

		// then
		assert.Nil(t, left)
		assert.Equal(t, "document", details.GetString(bundle.RelationKeyIconName))
		assert.Equal(t, int64(3), details.GetInt64(bundle.RelationKeyIconOption))
	})

	t.Run("no icon at all leaves the planned one alone", func(t *testing.T) {
		// given — a database with only a cover reaches here too
		details := planned()

		// when
		left := applyDatabaseTypeIcon(details, nil)

		// then
		assert.Nil(t, left)
		assert.Equal(t, "document", details.GetString(bundle.RelationKeyIconName))
	})
}

// TestNamedIconsInRecordedWorkspace measures the table against real data
// rather than against itself: every built-in icon name and color a real
// Notion export used must resolve, or that export imports types wearing the
// planner's guess instead of the icon its author picked.
func TestNamedIconsInRecordedWorkspace(t *testing.T) {
	cas, err := cassette.Load(workspaceCassette)
	if err != nil {
		t.Skip("no cassette recorded yet")
	}
	named := regexp.MustCompile(`"icon":\{"name":"([^"]+)","color":"([^"]+)"\}`)
	names, colors := map[string]int{}, map[string]int{}
	for _, interaction := range cas.Interactions {
		for _, match := range named.FindAllStringSubmatch(interaction.Response.Body, -1) {
			names[match[1]]++
			colors[match[2]]++
		}
	}
	require.NotEmpty(t, names, "the recorded workspace has no built-in icons")

	var unresolvedNames, unknownColors []string
	for name := range names {
		if _, _, ok := resolveNotionIconName(name); !ok {
			unresolvedNames = append(unresolvedNames, name)
		}
	}
	for color := range colors {
		if _, ok := notionIconColors[color]; !ok {
			unknownColors = append(unknownColors, color)
		}
	}
	sort.Strings(unresolvedNames)
	sort.Strings(unknownColors)
	assert.Empty(t, unresolvedNames)
	assert.Empty(t, unknownColors)
	t.Logf("%d built-in icons, %d distinct names, %d distinct colors", total(names), len(names), len(colors))
}

func total(counts map[string]int) int {
	sum := 0
	for _, count := range counts {
		sum += count
	}
	return sum
}

// TestIconEmojiAreEmoji keeps the table honest. A value that is not an emoji
// (a maths dingbat, a technical symbol) reaches the client as an icon it
// cannot render — worse than the bare page it replaces — and the table is
// hand-written, so nothing else catches it.
func TestIconEmojiAreEmoji(t *testing.T) {
	// Ranges with Emoji_Presentation=Yes, plus anything carrying the
	// emoji-presentation selector U+FE0F.
	emojiRange := func(r rune) bool {
		switch {
		case r >= 0x1F000:
			return true
		case r >= 0x231A && r <= 0x231B, r >= 0x23E9 && r <= 0x23F3, r >= 0x23F8 && r <= 0x23FA:
			return true
		case r >= 0x25FD && r <= 0x25FE, r >= 0x2600 && r <= 0x27BF:
			return true
		case r >= 0x2934 && r <= 0x2935, r >= 0x2B05 && r <= 0x2B07:
			return true
		case r >= 0x2B1B && r <= 0x2B1C, r == 0x2B50, r == 0x2B55:
			return true
		}
		return false
	}
	require.NotEmpty(t, notionIconEmoji)
	require.NotEmpty(t, notionEmoji)
	tables := map[string]map[string]string{"notionIconEmoji": notionIconEmoji, "notionEmoji": notionEmoji}
	for table, entries := range tables {
		for name, emoji := range entries {
			_ = table
			var renderable bool
			for _, r := range emoji {
				if r == 0xFE0F || emojiRange(r) {
					renderable = true
				}
			}
			assert.True(t, renderable, "%s: %q maps to %q (%U), which is not an emoji", table, name, emoji, []rune(emoji))
		}
	}
}

// TestEveryNotionIconResolves holds the mapping to the inventory in
// testdata/notion-icons.txt: every icon Notion's picker can produce must land
// on an Anytype icon, or that icon silently becomes no icon at all.
func TestEveryNotionIconResolves(t *testing.T) {
	raw, err := os.ReadFile("testdata/notion-icons.txt")
	require.NoError(t, err)

	var unresolved, noEmoji []string
	total := 0
	for _, line := range strings.Split(string(raw), "\n") {
		name := strings.TrimSpace(line)
		if name == "" || strings.HasPrefix(name, "#") {
			continue
		}
		total++
		resolved, _, ok := resolveNotionIconName(name)
		if !ok {
			unresolved = append(unresolved, name)
			continue
		}
		// Types wear the named icon; everything else needs the emoji, so a
		// name with no emoji is a page that imports bare.
		_ = resolved
		if emojiForNotionIcon(&iconValue{Type: "icon", Icon: &notionNamedIcon{Name: name}}) == "" {
			noEmoji = append(noEmoji, name+" → "+resolved)
		}
	}
	require.Greater(t, total, 400, "the inventory should hold Notion's whole picker")
	sort.Strings(unresolved)
	sort.Strings(noEmoji)
	assert.Empty(t, unresolved, "%d of %d Notion icons map to nothing", len(unresolved), total)
	assert.Empty(t, noEmoji, "%d of %d Notion icons reach a page with no emoji", len(noEmoji), total)

	// Every override must name an icon Notion actually has, or it is a line
	// nothing will ever read.
	inventory := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		if name := strings.TrimSpace(line); name != "" && !strings.HasPrefix(name, "#") {
			inventory[name] = true
		}
	}
	for name := range notionEmoji {
		assert.True(t, inventory[name], "notionEmoji overrides %q, which is not a Notion icon", name)
	}
}

func TestEmojiPrefersTheRicherVocabulary(t *testing.T) {
	emojiOf := func(name string) string {
		return emojiForNotionIcon(&iconValue{Type: "icon", Icon: &notionNamedIcon{Name: name, Color: "blue"}})
	}
	cases := []struct{ notion, want, why string }{
		{"banana", "🍌", "sixteen Notion foods share one Anytype icon; emoji has the fruit"},
		{"broccoli", "🥦", ""},
		{"mosque", "🕌", "church, mosque and synagogue all become one office block otherwise"},
		{"chess-king", "♟️", "chess pieces are not dice"},
		{"potted-plant", "🪴", ""},
		{"rocket", "🚀", "no override: the Anytype icon already says it"},
		{"book", "📖", ""},
		{"BANANA_alternate", "🍌", "an override is found through the same normalization as the icon"},
	}
	for _, c := range cases {
		t.Run(c.notion, func(t *testing.T) {
			assert.Equal(t, c.want, emojiOf(c.notion), c.why)
		})
	}

	t.Run("a name with no counterpart has no emoji", func(t *testing.T) {
		assert.Empty(t, emojiOf("quokka-riding-a-bicycle"))
	})

	t.Run("a type still takes the named icon, not the emoji", func(t *testing.T) {
		// given — the two channels differ deliberately
		name, _, ok := (&iconValue{Type: "icon", Icon: &notionNamedIcon{Name: "banana"}}).namedIcon()
		require.True(t, ok)
		assert.Equal(t, "nutrition", name)
	})
}
