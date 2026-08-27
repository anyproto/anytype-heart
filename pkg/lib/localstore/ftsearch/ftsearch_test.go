package ftsearch

import (
	"context"
	"fmt"
	"os"
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

	search, err := ft.Search("space1", "one", 0, true)
	require.NoError(t, err)
	require.Len(t, search, 1)

	search, err = ft.Search("space2", "one", 0, true)
	require.NoError(t, err)
	require.Len(t, search, 1)

	search, err = ft.Search("", "one", 0, true)
	require.NoError(t, err)
	require.Len(t, search, 2)

	_ = ft.Close(nil)
}

func TestNamePrefixSearch(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "")
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "id1/r/name",
		Title: "opa",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:   "id2/r/name",
		Text: "opa",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "id3/r/desc",
		Title: "one",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:   "id4/r/desc",
		Text: "opa",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:   "id5/r/desc",
		Text: "noone",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:   "id6/r/snippet",
		Text: "opa",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:   "id7/r/pluralName",
		Text: "opa",
	}))

	search, err := ft.NamePrefixSearch("", "o", 0)
	require.NoError(t, err)
	require.Len(t, search, 4)

	search, err = ft.NamePrefixSearch("", "n", 0)
	require.NoError(t, err)
	require.Len(t, search, 0)

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
		},
		{
			name:   "assertPrefix",
			tester: assertPrefix,
		},
		{
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

func assertPrefix(t *testing.T, tmpDir string) {
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft

	require.NoError(t, ft.Index(SearchDoc{
		Id:    "1",
		Title: "I love my mum",
		Text:  "",
	}))

	require.NoError(t, ft.Index(SearchDoc{
		Id:    "2",
		Title: "",
		Text:  "Something completely different",
	}))

	require.NoError(t, ft.Index(SearchDoc{
		Id:    "4",
		Title: "Just random filler",
		Text:  "",
	}))

	require.NoError(t, ft.Index(SearchDoc{
		Id:    "4",
		Title: "Another text for fun",
		Text:  "",
	}))

	validateSearch(t, ft, "", "I love", 1)
	validateSearch(t, ft, "", "I lo", 1)
	validateSearch(t, ft, "", "I", 1)
	validateSearch(t, ft, "", "lov", 1)

	validateSearch(t, ft, "", "Something", 1)
	validateSearch(t, ft, "", "Some", 1)
	validateSearch(t, ft, "", "comp", 1)
	validateSearch(t, ft, "", "diff", 1)
	validateSearch(t, ft, "", "Something c", 1)
	validateSearch(t, ft, "", "Something different", 1)
	validateSearch(t, ft, "", "different something", 1)

	_ = ft.Close(nil)
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
	batcher := ft.NewAutoBatcher()
	for _, doc := range docs {
		require.NoError(t, batcher.UpsertDoc(doc))
	}
	batcher.Finish()
	count, err := ft.DocCount()
	require.NoError(t, err)
	require.Equal(t, 100, int(count))

	batcher = ft.NewAutoBatcher()
	batcher.DeleteDoc(fmt.Sprintf("randomid%d/r/randomrel%d", 49, 149))
	batcher.Finish()

	count, _ = ft.DocCount()
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
	res, err := ft.Search(spaceID, qry, 0, true)
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
	validateSearch(t, ft, "", "of", 4)

	_ = ft.Close(nil)
}

func TestFtSearch_Close(t *testing.T) {
	// given
	fts := new(ftSearch)

	// when
	err := fts.Close(nil)

	// then
	assert.NoError(t, err)
}

func TestSearchChatScopes(t *testing.T) {
	indexAll := func(t *testing.T, ft FTSearch) {
		// object docs that match the query and would compete for the candidate budget
		require.NoError(t, ft.Index(SearchDoc{
			Id:      domain.NewObjectPathWithBlock("obj1", "b1").String(),
			SpaceId: "space1",
			Text:    "needle in an object block",
		}))
		require.NoError(t, ft.Index(SearchDoc{
			Id:      domain.NewObjectPathWithRelation("obj2", "name").String(),
			SpaceId: "space1",
			Title:   "needle in a relation",
		}))
		for chatId, msgs := range map[string]map[string]string{
			"chat1": {"msg1": "needle in chat1", "msg2": "nothing here"},
			"chat2": {"msg3": "needle in chat2"},
		} {
			for msgId, text := range msgs {
				require.NoError(t, ft.Index(SearchDoc{
					Id:        domain.NewObjectPathWithMessage(chatId, msgId).String(),
					SpaceId:   "space1",
					Text:      text,
					MessageId: msgId,
				}))
			}
		}
		require.NoError(t, ft.Index(SearchDoc{
			Id:        domain.NewObjectPathWithMessage("chat3", "msg4").String(),
			SpaceId:   "space2",
			Text:      "needle in chat3",
			MessageId: "msg4",
		}))
	}

	collectIds := func(results []*DocumentMatch) []string {
		ids := make([]string, 0, len(results))
		for _, r := range results {
			ids = append(ids, r.ID)
		}
		return ids
	}

	t.Run("single chat scope is unchanged", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "")
		fx := newFixture(tmpDir, t)
		defer func() { _ = fx.ft.Close(nil) }()
		indexAll(t, fx.ft)

		results, err := fx.ft.SearchChat("space1", "chat1", "needle", nil, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"chat1/m/msg1"}, collectIds(results))
	})

	t.Run("empty chatId searches message docs of all chats in the space", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "")
		fx := newFixture(tmpDir, t)
		defer func() { _ = fx.ft.Close(nil) }()
		indexAll(t, fx.ft)

		results, err := fx.ft.SearchChat("space1", "", "needle", nil, 0)
		require.NoError(t, err)
		// message docs only: no object docs, no other-space messages
		assert.ElementsMatch(t, []string{"chat1/m/msg1", "chat2/m/msg3"}, collectIds(results))
	})

	t.Run("empty spaceId and chatId searches message docs of all spaces", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "")
		fx := newFixture(tmpDir, t)
		defer func() { _ = fx.ft.Close(nil) }()
		indexAll(t, fx.ft)

		results, err := fx.ft.SearchChat("", "", "needle", nil, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"chat1/m/msg1", "chat2/m/msg3", "chat3/m/msg4"}, collectIds(results))
		// hits carry their stored space for attribution
		for _, r := range results {
			want := "space1"
			if r.ID == "chat3/m/msg4" {
				want = "space2"
			}
			assert.Equal(t, want, r.SpaceId, r.ID)
		}
	})

	t.Run("creator filter restricts to the given authors", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "")
		fx := newFixture(tmpDir, t)
		defer func() { _ = fx.ft.Close(nil) }()

		// identities are raw-indexed: exact, case-sensitive match
		index := func(msgId, author string) {
			require.NoError(t, fx.ft.Index(SearchDoc{
				Id:        domain.NewObjectPathWithMessage("chat1", msgId).String(),
				SpaceId:   "space1",
				Text:      "needle from " + author,
				Author:    author,
				MessageId: msgId,
			}))
		}
		index("msgAlice1", "A5aLiCe")
		index("msgBob", "B7bob")
		index("msgAlice2", "A5aLiCe")
		index("msgCarol", "C9carol")

		single, err := fx.ft.SearchChat("space1", "chat1", "needle", []string{"A5aLiCe"}, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"chat1/m/msgAlice1", "chat1/m/msgAlice2"}, collectIds(single))

		multi, err := fx.ft.SearchChat("space1", "", "needle", []string{"A5aLiCe", "B7bob"}, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"chat1/m/msgAlice1", "chat1/m/msgAlice2", "chat1/m/msgBob"}, collectIds(multi))

		// wrong case must not match the raw-indexed identity
		wrongCase, err := fx.ft.SearchChat("space1", "", "needle", []string{"a5alice"}, 0)
		require.NoError(t, err)
		assert.Empty(t, wrongCase)

		// empty creator strings are ignored, not match-nothing
		blank, err := fx.ft.SearchChat("space1", "", "needle", []string{""}, 0)
		require.NoError(t, err)
		assert.Len(t, blank, 4)
	})

	t.Run("marker matches whole m path segments only, never letters inside ids", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "")
		fx := newFixture(tmpDir, t)
		defer func() { _ = fx.ft.Close(nil) }()

		// "m" letters embedded in id tokens must not match the marker term:
		// the id tokenizer splits on non-alphanumerics, so every segment is one
		// token and term queries never match substrings
		require.NoError(t, fx.ft.Index(SearchDoc{
			Id:      domain.NewObjectPathWithBlock("form", "mango").String(),
			SpaceId: "space1",
			Text:    "needle in an object block",
		}))
		// a message doc whose chat/message ids contain plenty of m letters
		require.NoError(t, fx.ft.Index(SearchDoc{
			Id:        domain.NewObjectPathWithMessage("bafymchatm", "msgmmm1").String(),
			SpaceId:   "space1",
			Text:      "needle in a chat message",
			MessageId: "msgmmm1",
		}))
		// a lone "m" path segment on a non-message doc is the known false
		// positive: returned at this layer, dropped by the caller's
		// path.HasMessage() filter
		require.NoError(t, fx.ft.Index(SearchDoc{
			Id:      domain.NewObjectPathWithBlock("obj1", "m").String(),
			SpaceId: "space1",
			Text:    "needle in a block named m",
		}))

		results, err := fx.ft.SearchChat("space1", "", "needle", nil, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"bafymchatm/m/msgmmm1", "obj1/b/m"}, collectIds(results))
	})

	t.Run("messages do not compete with object docs for the candidate budget", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "")
		fx := newFixture(tmpDir, t)
		defer func() { _ = fx.ft.Close(nil) }()

		// many object docs matching the query better than the single message
		for i := 0; i < 150; i++ {
			require.NoError(t, fx.ft.Index(SearchDoc{
				Id:      domain.NewObjectPathWithBlock(fmt.Sprintf("obj%d", i), "b").String(),
				SpaceId: "space1",
				Text:    "needle needle needle",
			}))
		}
		require.NoError(t, fx.ft.Index(SearchDoc{
			Id:        domain.NewObjectPathWithMessage("chat1", "msg1").String(),
			SpaceId:   "space1",
			Text:      "a long message that mentions the needle once between other words",
			MessageId: "msg1",
		}))

		// a small limit still returns the message: object docs are out of the scoped set
		results, err := fx.ft.SearchChat("space1", "", "needle", nil, 10)
		require.NoError(t, err)
		assert.Equal(t, []string{"chat1/m/msg1"}, collectIds(results))
	})
}
