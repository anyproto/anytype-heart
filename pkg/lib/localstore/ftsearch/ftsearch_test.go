package ftsearch

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/blevesearch/bleve/v2"
	"go.uber.org/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/wallet"
)

type fixture struct {
	ft   FTSearch
	ta   *app.App
	ctrl *gomock.Controller
}

func newFixture(path string, t *testing.T) *fixture {
	ft := New()
	ta := new(app.App)

	ta.Register(wallet.NewWithRepoDirAndRandomKeys(path)).
		Register(ft)

	require.NoError(t, ta.Start(context.Background()))
	return &fixture{
		ft: ft,
		ta: ta,
	}
}

func TestNewFTSearch(t *testing.T) {
	testCases := []struct {
		name   string
		tester func(t *testing.T, tmpDir string)
	}{
		{
			name:   "assertSearch",
			tester: assertSearch,
		},
		{
			name:   "assertThaiSubstrFound",
			tester: assertThaiSubstrFound,
		},
		{
			name:   "assertChineseFound",
			tester: assertChineseFound,
		},
		{
			name:   "assertFoundPartsOfTheWords",
			tester: assertFoundPartsOfTheWords,
		},
		{
			name:   "assertFoundCaseSensitivePartsOfTheWords",
			tester: assertFoundCaseSensitivePartsOfTheWords,
		},
		{
			name:   "assertNonEscapedQuery",
			tester: assertNonEscapedQuery,
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
		Text:  "你好",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "2",
		Title: "",
		Text:  "交代",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "3",
		Title: "",
		Text:  "长江大桥",
	}))

	queries := []string{
		"你好世界",
		"亲口交代",
		"长江",
	}

	for _, qry := range queries {
		validateSearch(t, ft, "", qry, 1)
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

func assertFoundPartsOfTheWords(t *testing.T, tmpDir string) {
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "1",
		Title: "This is the title",
		Text:  "two",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "2",
		Title: "is the title",
		Text:  "two",
	}))

	validateSearch(t, ft, "", "this", 1)
	validateSearch(t, ft, "", "his", 1)
	validateSearch(t, ft, "", "is", 2)
	validateSearch(t, ft, "", "i t", 2)

	_ = ft.Close(nil)
}

func validateSearch(t *testing.T, ft FTSearch, spaceID, qry string, times int) {
	res, err := ft.Search(spaceID, qry)
	require.NoError(t, err)
	assert.Len(t, res, times)
}

func TestChineseSearch(t *testing.T) {
	// given
	index := givenPrefilledChineseIndex()
	defer func() { _ = index.Close() }()

	expected := givenExpectedChinese()

	// when
	queries := []string{
		"你好世界",
		"亲口交代",
		"长江",
	}

	// then
	result := validateChinese(queries, index)
	assert.Equal(t, expected, result)
}

func prettify(res *bleve.SearchResult) string {
	type Result struct {
		Id    string  `json:"id"`
		Score float64 `json:"score"`
	}
	results := []Result{}
	for _, item := range res.Hits {
		results = append(results, Result{item.ID, item.Score})
	}
	b, err := json.Marshal(results)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func validateChinese(queries []string, index bleve.Index) [3]string {
	result := [3]string{}
	for i, q := range queries {
		req := bleve.NewSearchRequest(bleve.NewQueryStringQuery(q))
		req.Highlight = bleve.NewHighlight()
		res, err := index.Search(req)
		if err != nil {
			panic(err)
		}
		result[i] = prettify(res)
	}
	return result
}

func givenExpectedChinese() [3]string {
	return [3]string{
		`[{"id":"1","score":0.3192794660708729}]`,
		`[{"id":"2","score":0.3192794660708729}]`,
		`[{"id":"3","score":0.8888941720598743}]`,
	}
}

func givenPrefilledChineseIndex() bleve.Index {
	tmpDir, _ := os.MkdirTemp("", "")
	messages := []struct {
		Id   string
		Text string
	}{
		{
			Id:   "1",
			Text: "你好",
		},
		{
			Id:   "2",
			Text: "交代",
		},
		{
			Id:   "3",
			Text: "长江大桥",
		},
	}

	indexMapping := makeMapping()

	index, err := bleve.New(tmpDir, indexMapping)
	if err != nil {
		panic(err)
	}
	for _, msg := range messages {
		if err := index.Index(msg.Id, msg); err != nil {
			panic(err)
		}
	}
	return index
}

func assertNonEscapedQuery(t *testing.T, tmpDir string) {
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "1",
		Title: "This is the title",
		Text:  "two",
	}))

	validateSearch(t, ft, "", ".*?([])", 0)

	require.NoError(t, ft.Index(SearchDoc{
		Id:    "1",
		Title: "This is the title",
		Text:  ".*?([])",
	}))
	validateSearch(t, ft, "", ".*?([])", 1)

	_ = ft.Close(nil)
}

func assertMultiSpace(t *testing.T, tmpDir string) {
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:      "1.1",
		SpaceID: "first",
		Title:   "Dashboard of first space",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:      "1.2",
		SpaceID: "first",
		Title:   "Advanced of first space",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:      "2.1",
		SpaceID: "second",
		Title:   "Dashboard of second space",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:      "2.2",
		SpaceID: "second",
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
	validateSearch(t, ft, "", "board", 2)
	validateSearch(t, ft, "", "space", 4)
	validateSearch(t, ft, "", "of", 5)

	_ = ft.Close(nil)
}
