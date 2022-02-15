package ftsearch

import (
	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/golang/mock/gomock"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixture struct {
	ft   FTSearch
	ta   *testapp.TestApp
	ctrl *gomock.Controller
}

func newFixture(path string, t *testing.T) *fixture {
	ft := New()
	ta := testapp.New().
		With(wallet.NewWithRepoPathAndKeys(path, nil, nil)).
		With(ft)

	require.NoError(t, ta.Start())
	return &fixture{
		ft: ft,
		ta: ta,
	}
}

func TestNewFTSearch(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "test",
		Title: "one",
		Text:  "two",
	}))
	res, err := ft.Search("one")
	require.NoError(t, err)
	assert.Len(t, res, 1)
	ft.Close()
	fixture = newFixture(tmpDir, t)
	ft = fixture.ft

	require.NoError(t, err)
	res, err = ft.Search("one")
	require.NoError(t, err)
	assert.Len(t, res, 1)
	ft.Close()
}

func TestFtSearch_Search(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	defer ft.Close()
	var docs = [...]SearchDoc{
		{
			Id:    "1",
			Title: "First one",
			Text:  "one two two",
		},
		{
			Id:    "2",
			Title: "Second two",
			Text:  "one two three",
		},
		{
			Id:    "3",
			Title: "Third three",
			Text:  "some text with 3",
		},
		{
			Id:    "4",
			Title: "Fours four",
			Text:  "some text with four and some text five",
		},
		{
			Id:    "5",
			Title: "Fives five",
			Text:  "some text with five and one and two ans rs",
		},
		{
			Id:    "6",
			Title: "Rs six some",
			Text:  "some text with six",
		},
		{
			Id: "somelongidentifier",
		},
		{
			Id:    "eczq5t",
			Title: "FERRARI styling CENter with somethinglong ",
		},
		{
			Id:    "sometitle",
			Title: "Some title with words",
		},
	}
	for _, d := range docs {
		require.NoError(t, ft.Index(d))
	}

	searches := [...]struct {
		Query  string
		Result []string
	}{
		{
			"one",
			[]string{"1", "2", "5"},
		},
		{
			"rs",
			[]string{"6", "1", "4", "5"},
		},
		{
			"two",
			[]string{"2", "1", "5"},
		},
		{
			"six",
			[]string{"6"},
		},
		{
			"some text",
			[]string{"4", "3", "6", "5"},
		},
		{
			"somelongidentifier",
			[]string{"somelongidentifier"},
		},
		{
			"FeRRa",
			[]string{"eczq5t"},
		},
		{
			"Ferrari st",
			[]string{"eczq5t"},
		},
		{
			"Some ti",
			[]string{"sometitle"},
		},
	}

	for _, st := range searches {
		ids, err := ft.Search(st.Query)
		require.NoError(t, err)
		assert.Equal(t, st.Result, ids, st.Query)
		t.Logf("%s:\t%v\t%v", st.Query, ids, st.Result)
	}
}
