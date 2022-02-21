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
