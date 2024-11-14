package ftsearch

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/wallet"
)

type fixture struct {
	ft FTSearch
	ta *app.App
}

func newFixture(path string, t *testing.T) *fixture {
	ft := TantivyNew()
	ta := new(app.App)

	ta.Register(wallet.NewWithRepoDirAndRandomKeys(path)).
		Register(ft)

	require.NoError(t, ta.Start(context.Background()))
	return &fixture{
		ft: ft,
		ta: ta,
	}
}

func TestListIndexedIds(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "")
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:    domain.NewObjectPathWithBlock("o", "1").String(),
		Title: "one",
		Text:  "two",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:    domain.NewObjectPathWithBlock("o", "2").String(),
		Title: "one",
		Text:  "two",
	}))

	require.NoError(t, ft.Index(SearchDoc{
		Id:    domain.NewObjectPathWithBlock("a", "3").String(),
		Title: "one",
		Text:  "two",
	}))
	dc, err := ft.DocCount()
	require.NoError(t, err)
	require.Equal(t, 3, int(dc))

	_ = ft.Close(nil)
}

func TestDifferentSpaces(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "")
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:      "1",
		Title:   "one",
		SpaceId: "space1",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:      "2",
		Title:   "one",
		SpaceId: "space2",
	}))

	search, err := ft.Search([]string{"space1"}, "one")
	require.NoError(t, err)
	require.Len(t, search, 1)

	search, err = ft.Search([]string{"space2"}, "one")
	require.NoError(t, err)
	require.Len(t, search, 1)

	search, err = ft.Search([]string{"space1", "space2"}, "one")
	require.NoError(t, err)
	require.Len(t, search, 2)

	search, err = ft.Search([]string{""}, "one")
	require.NoError(t, err)
	require.Len(t, search, 2)

	search, err = ft.Search(nil, "one")
	require.NoError(t, err)
	require.Len(t, search, 2)

	_ = ft.Close(nil)
}

func TestNewFTSearch(t *testing.T) {
	testCases := []struct {
		name   string
		tester func(t *testing.T, tmpDir string)
	}{
		{
			name:   "assertProperIds",
			tester: assertProperIds,
		},
		{
			name:   "assertSearch",
			tester: assertSearch,
		},
		{
			name:   "assertFoundCaseSensitivePartsOfTheWords",
			tester: assertFoundCaseSensitivePartsOfTheWords,
		}, {
			name:   "assertChineseFound",
			tester: assertChineseFound,
		},
		{
			name:   "assertMultiSpace",
			tester: assertMultiSpace,
		},
	}

	for _, testCase := range testCases {
		tmpDir, _ := os.MkdirTemp("", "")
		t.Run(testCase.name, func(t *testing.T) {
			testCase.tester(t, tmpDir)
		})
	}
}

func assertFoundCaseSensitivePartsOfTheWords(t *testing.T, tmpDir string) {
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft

	require.NoError(t, ft.Index(SearchDoc{
		Id:    "2",
		Title: "Advanced",
		Text:  "first second",
	}))

	require.NoError(t, ft.Index(SearchDoc{
		Id:    "3",
		Title: "Another object",
		Text:  "third",
	}))

	require.NoError(t, ft.Index(SearchDoc{
		Id:    "4",
		Title: "This object is Interesting",
		Text:  "third",
	}))

	validateSearch(t, ft, "", "Advanced", 1)

	validateSearch(t, ft, "", "advanced", 1)
	validateSearch(t, ft, "", "Advanc", 1)
	validateSearch(t, ft, "", "advanc", 1)

	validateSearch(t, ft, "", "first", 1)
	validateSearch(t, ft, "", "second", 1)
	validateSearch(t, ft, "", "Interesting", 1)
	validateSearch(t, ft, "", "Interes", 1)
	validateSearch(t, ft, "", "interes", 1)
	validateSearch(t, ft, "", "third", 2)

	_ = ft.Close(nil)
}

func assertChineseFound(t *testing.T, tmpDir string) {
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "1",
		Title: "",
		Text:  "张华考上了北京大学；李萍进了中等技术学校；我在百货公司当售货员：我们都有光明的前途",
	}))

	require.NoError(t, ft.Index(SearchDoc{
		Id:    "2",
		Title: "张华考上了北京大学；李萍进了中等技术学校；我在百货公司当售货员：我们都有光明的前途",
		Text:  "",
	}))

	queries := []string{
		"售货员",
	}

	for _, qry := range queries {
		validateSearch(t, ft, "", qry, 2)
	}

	_ = ft.Close(nil)
}

func assertThaiSubstrFound(t *testing.T, tmpDir string) {
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "test",
		Title: "ตัวอย่าง",
		Text:  "พรระเจ้า \n kumamon",
	}))

	validateSearch(t, ft, "", "ระเ", 1)
	validateSearch(t, ft, "", "ระเ ma", 1)

	_ = ft.Close(nil)
}

func assertProperIds(t *testing.T, tmpDir string) {
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	var docs []SearchDoc
	for i := range 50 {
		docs = append(docs, SearchDoc{
			Id:      fmt.Sprintf("randomid%d/r/randomrel%d", i, i+100),
			SpaceId: fmt.Sprintf("randomspaceid%d", i),
		})
		docs = append(docs, SearchDoc{
			Id:      fmt.Sprintf("randomid%d/r/randomrel%d", i, i+1000),
			SpaceId: fmt.Sprintf("randomspaceid%d", i),
		})
	}
	assert.NoError(t, ft.BatchIndex(context.Background(), docs, nil))

	assert.NoError(t, ft.DeleteObject(fmt.Sprintf("randomid%d/r/randomrel%d", 49, 149)))

	count, _ := ft.DocCount()
	require.Equal(t, 99, int(count))

	_ = ft.Close(nil)
}

func assertSearch(t *testing.T, tmpDir string) {
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "test",
		Title: "one",
		Text:  "two",
	}))

	validateSearch(t, ft, "", "one", 1)
	validateSearch(t, ft, "", "two", 1)

	_ = ft.Close(nil)
}

func validateSearch(t *testing.T, ft FTSearch, spaceID, qry string, times int) {
	res, err := ft.Search([]string{spaceID}, qry)
	require.NoError(t, err)
	assert.Len(t, res, times)
}

func assertMultiSpace(t *testing.T, tmpDir string) {
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:      "1/1",
		SpaceId: "first",
		Title:   "Dashboard of first space",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:      "1/2",
		SpaceId: "first",
		Title:   "Advanced of first space",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:      "2/1",
		SpaceId: "second",
		Title:   "Dashboard of second space",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:      "2/2",
		SpaceId: "second",
		Title:   "Get Started of second space",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "0",
		Title: "My favorite coffee brands",
	}))

	validateSearch(t, ft, "first", "Dashboard", 1)
	validateSearch(t, ft, "first", "art", 0)
	validateSearch(t, ft, "second", "space", 2)
	validateSearch(t, ft, "second", "coffee", 0)
	validateSearch(t, ft, "", "Advanced", 1)
	validateSearch(t, ft, "", "dash", 2)
	validateSearch(t, ft, "", "space", 4)
	validateSearch(t, ft, "", "of", 5)

	_ = ft.Close(nil)
}

func TestEscapeQuery(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{strings.Repeat("a", 99) + " aa", `("` + strings.Repeat("a", 99) + `" OR ` + strings.Repeat("a", 99) + `)`},
		{`""`, ``},
		{"simpleQuery", `("simplequery" OR simplequery)`},
		{"with+special^chars", `("withspecialchars" OR withspecialchars)`},
		{"text`with:brackets{}", `("textwithbrackets" OR textwithbrackets)`},
		{"escaped[]symbols()", `("escapedsymbols" OR escapedsymbols)`},
		{"multiple!!special~~", `("multiplespecial" OR multiplespecial)`},
	}

	for _, test := range tests {
		actual := prepareQuery(test.input)
		if actual != test.expected {
			t.Errorf("For input '%s', expected '%s', but got '%s'", test.input, test.expected, actual)
		}
	}
}

// Tests
func TestGetSpaceIdsQuery(t *testing.T) {
	// Test with empty slice of ids
	assert.Equal(t, "", getSpaceIdsQuery([]string{}))

	// Test with slice containing only empty strings
	assert.Equal(t, "", getSpaceIdsQuery([]string{"", "", ""}))

	// Test with a single id
	assert.Equal(t, "(SpaceID:123)", getSpaceIdsQuery([]string{"123"}))

	// Test with multiple ids
	assert.Equal(t, "(SpaceID:123 OR SpaceID:456 OR SpaceID:789)", getSpaceIdsQuery([]string{"123", "456", "789"}))

	// Test with some empty ids
	assert.Equal(t, "(SpaceID:123 OR SpaceID:789)", getSpaceIdsQuery([]string{"123", "", "789"}))
}

func TestFtSearch_Close(t *testing.T) {
	// given
	fts := new(ftSearchTantivy)

	// when
	err := fts.Close(nil)

	// then
	assert.NoError(t, err)
}
