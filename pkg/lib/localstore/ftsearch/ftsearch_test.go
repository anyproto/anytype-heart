package ftsearch

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"runtime"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/standard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/wallet"
)

type fixture struct {
	ft   FTSearch
	ta   *app.App
	ctrl *gomock.Controller
}

func newFixture(path string, t *testing.T) *fixture {
	fts := New()
	testApp := new(app.App)

	testApp.Register(wallet.NewWithRepoDirAndRandomKeys(path)).
		Register(fts)

	require.NoError(t, testApp.Start(context.Background()))
	return &fixture{
		ft: fts,
		ta: testApp,
	}
}

func TestNewFTSearch(t *testing.T) {
	testCases := []func(t *testing.T, tmpDir string){
		assertSearch,
		assertThaiSubstrFound,
		assertExactQueryFound,
		assertChineseFound,
		assertFoundPartsOfTheWords,
		assertFoundCaseSensitivePartsOfTheWords,
		assertNonEscapedQuery,
		assertMultiSpace,
	}

	for _, testCase := range testCases {
		tmpDir, _ := os.MkdirTemp("", "")
		t.Run(GetFunctionName(testCase), func(t *testing.T) {
			testCase(t, tmpDir)
		})
	}
}

func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
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
	//analyzerName = cjk.AnalyzerName
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "1",
		Title: "你好",
		Text:  "",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "2",
		Title: "交代",
		Text:  "",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "3",
		Title: "长江大桥",
		Text:  "",
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
	analyzerName = standard.Name
}

func assertExactQueryFound(t *testing.T, tmpDir string) {
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "test1",
		Title: "https://community.anytype.io/c/bug-reports/7/none",
		Text:  "name:section.name",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "test2",
		Title: "Some random text name:section.name and there some random text",
		Text:  "I have this nice link I can't find https://community.anytype.io/c/bug-reports/7/none but I'll try to find it",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "test3",
		Title: "Some strange filler strings section community https",
		Text:  "Some strange symbols :// :strange: :none:",
	}))

	validateSearch(t, ft, "", "https://community.anytype.io/c/bug-reports/7/none", 2)
	validateSearch(t, ft, "", "name:section.name", 2)

	_ = ft.Close(nil)
}

func assertThaiSubstrFound(t *testing.T, tmpDir string) {
	//analyzerName = th.AnalyzerName
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "test",
		Title: "พรระเจ้า \n kumamon",
		Text:  "ตัวอย่าง",
	}))

	validateSearch(t, ft, "", "ระเ", 1)
	validateSearch(t, ft, "", "ระเ ma", 1)
	validateSearch(t, ft, "", "ตัวอย่", 1)

	_ = ft.Close(nil)
	analyzerName = standard.Name
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

	indexMapping := makeMapping(standard.Name)

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

	validateSearch(t, ft, "", "*", 0)

	require.NoError(t, ft.Index(SearchDoc{
		Id:    "1",
		Title: ".*?([])",
		Text:  "This is the text",
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
