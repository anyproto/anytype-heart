package ftsearch

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFTSearch(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)
	ft, err := NewFTSearch(tmpDir)
	require.NoError(t, err)
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "test",
		Title: "one",
		Text:  "two",
	}))
	res, err := ft.Search("one")
	require.NoError(t, err)
	assert.Len(t, res, 1)
	ft.Close()
	ft, err = NewFTSearch(tmpDir)
	require.NoError(t, err)
	res, err = ft.Search("one")
	require.NoError(t, err)
	assert.Len(t, res, 1)
	ft.Close()
}

func TestFtSearch_Search(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)
	ft, err := NewFTSearch(tmpDir)
	require.NoError(t, err)
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
			"some tex",
			[]string{"6", "2", "4", "3", "5"},
		},
	}

	for _, st := range searches {
		ids, err := ft.Search(st.Query)
		require.NoError(t, err)
		assert.Equal(t, st.Result, ids, st.Query)
		t.Logf("%s:\t%v\t%v", st.Query, ids, st.Result)
	}
}
