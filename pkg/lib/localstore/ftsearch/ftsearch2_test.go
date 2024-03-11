package ftsearch

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/blevesearch/bleve/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/wallet"
)

type fixture2 struct {
	ft   FTSearch
	ta   *app.App
	ctrl *gomock.Controller
}

func newFixture2(path string, t *testing.T) *fixture2 {
	ftsDir2 = ""
	ft := New2()
	ta := new(app.App)

	ta.Register(wallet.NewWithRepoDirAndRandomKeys(path)).
		Register(ft)

	require.NoError(t, ta.Start(context.Background()))
	return &fixture2{
		ft: ft,
		ta: ta,
	}
}

func TestNewFTSearch2(t *testing.T) {
	testCases := []struct {
		name   string
		tester func(t *testing.T, tmpDir string)
	}{
		{
			name:   "assertSearch",
			tester: assertSearch2,
		},
		{
			name:   "assertThaiSubstrFound",
			tester: assertThaiSubstrFound2,
		},
		{
			name:   "assertChineseFound",
			tester: assertChineseFound2,
		},
		{
			name:   "assertFoundPartsOfTheWords",
			tester: assertFoundPartsOfTheWords2,
		},
		// {
		// 	name:   "assertFoundCaseSensitivePartsOfTheWords",
		// 	tester: assertFoundCaseSensitivePartsOfTheWords2,
		// },
		{
			name:   "assertNonEscapedQuery2",
			tester: assertNonEscapedQuery2,
		},
		{
			name:   "assertMultiSpace",
			tester: assertMultiSpace2,
		},
	}

	for _, testCase := range testCases {
		tmpDir, _ := os.MkdirTemp("", "")
		t.Run(testCase.name, func(t *testing.T) {
			testCase.tester(t, tmpDir)
		})
	}
}

func assertFoundCaseSensitivePartsOfTheWords2(t *testing.T, tmpDir string) {
	fixture2 := newFixture2(tmpDir, t)
	ft := fixture2.ft

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

	validateSearch2(t, ft, "", "Advanced", 1)

	validateSearch2(t, ft, "", "advanced", 1)
	validateSearch2(t, ft, "", "Advanc", 1)
	validateSearch2(t, ft, "", "advanc", 1)

	validateSearch2(t, ft, "", "first", 1)
	validateSearch2(t, ft, "", "second", 1)
	validateSearch2(t, ft, "", "Interesting", 1)
	validateSearch2(t, ft, "", "Interes", 1)
	validateSearch2(t, ft, "", "interes", 1)
	validateSearch2(t, ft, "", "third", 2)

	_ = ft.Close(nil)
}

func assertChineseFound2(t *testing.T, tmpDir string) {
	fixture2 := newFixture2(tmpDir, t)
	ft := fixture2.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:      "1",
		Title:   "",
		Text:    "你好",
		SpaceID: "spaceId",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:      "2",
		Title:   "",
		Text:    "交代",
		SpaceID: "spaceId",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:      "3",
		Title:   "",
		Text:    "长江大桥",
		SpaceID: "spaceId",
	}))

	queries := []string{
		"你好世界",
		"亲口交代",
		"长江",
	}

	for _, qry := range queries {
		validateSearch2(t, ft, "spaceId", qry, 1)
	}

	_ = ft.Close(nil)
}

func assertThaiSubstrFound2(t *testing.T, tmpDir string) {
	fixture2 := newFixture2(tmpDir, t)
	ft := fixture2.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "test",
		Title: "ตัวอย่าง",
		Text:  "พรระเจ้า \n kumamon",
	}))

	validateSearch2(t, ft, "", "ระเ", 1)
	validateSearch2(t, ft, "", "ระเ ma", 1)

	_ = ft.Close(nil)
}

func assertSearch2(t *testing.T, tmpDir string) {
	fixture2 := newFixture2(tmpDir, t)
	ft := fixture2.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "test",
		Title: "one",
		Text:  "two",
	}))

	validateSearch2(t, ft, "", "one", 1)
	validateSearch2(t, ft, "", "two", 1)

	_ = ft.Close(nil)
}

func assertFoundPartsOfTheWords2(t *testing.T, tmpDir string) {
	fixture2 := newFixture2(tmpDir, t)
	ft := fixture2.ft
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

	validateSearch2(t, ft, "", "this", 1)
	validateSearch2(t, ft, "", "his", 1)
	validateSearch2(t, ft, "", "is", 2)
	validateSearch2(t, ft, "", "i t", 2)

	_ = ft.Close(nil)
}

func validateSearch2(t *testing.T, ft FTSearch, spaceID, qry string, times int) {
	res, err := ft.Search(spaceID, qry)
	require.NoError(t, err)
	assert.Len(t, res, times)
}

func TestChineseSearch2(t *testing.T) {
	// given
	index := givenPrefilledChineseIndex2()
	defer func() { _ = index.Close() }()

	expected := givenExpectedChinese2()

	// when
	queries := []string{
		"你好世界",
		"亲口交代",
		"长江",
	}

	// then
	result := validateChinese2(queries, index)
	assert.Equal(t, expected, result)
}

func prettify2(res *bleve.SearchResult) string {
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

func validateChinese2(queries []string, index bleve.Index) [3]string {
	result := [3]string{}
	for i, q := range queries {
		req := bleve.NewSearchRequest(bleve.NewQueryStringQuery(q))
		req.Highlight = bleve.NewHighlight()
		res, err := index.Search(req)
		if err != nil {
			panic(err)
		}
		result[i] = prettify2(res)
	}
	return result
}

func givenExpectedChinese2() [3]string {
	return [3]string{
		`[{"id":"1","score":0.3192794660708729}]`,
		`[{"id":"2","score":0.3192794660708729}]`,
		`[{"id":"3","score":0.8888941720598743}]`,
	}
}

func givenPrefilledChineseIndex2() bleve.Index {
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

func assertNonEscapedQuery2(t *testing.T, tmpDir string) {
	fixture2 := newFixture2(tmpDir, t)
	ft := fixture2.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "1",
		Title: "This is the title",
		Text:  "two",
	}))

	validateSearch2(t, ft, "", "*", 0)

	require.NoError(t, ft.Index(SearchDoc{
		Id:    "1",
		Title: "This is the title",
		Text:  ".*?([])",
	}))
	validateSearch2(t, ft, "", ".*?([])", 1)

	_ = ft.Close(nil)
}

func assertMultiSpace2(t *testing.T, tmpDir string) {
	fixture2 := newFixture2(tmpDir, t)
	ft := fixture2.ft
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

	validateSearch2(t, ft, "first", "Dashboard", 1)
	validateSearch2(t, ft, "first", "art", 0)
	validateSearch2(t, ft, "second", "space", 2)
	validateSearch2(t, ft, "second", "coffee", 0)
	validateSearch2(t, ft, "", "Advanced", 1)
	validateSearch2(t, ft, "", "board", 2)
	validateSearch2(t, ft, "", "space", 4)
	validateSearch2(t, ft, "", "of", 5)

	_ = ft.Close(nil)
}
