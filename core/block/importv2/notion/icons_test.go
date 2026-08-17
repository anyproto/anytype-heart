package notion

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
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
		if _, ok := resolveNotionIconName(name); !ok {
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
