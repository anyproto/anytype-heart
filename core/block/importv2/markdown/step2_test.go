package markdown

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/template"
	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/source"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	notionIdA = "0123456789abcdef0123456789abcdef"
	notionIdB = "fedcba9876543210fedcba9876543210"
)

func storeMembers(t *testing.T, collections *types.Struct) []string {
	t.Helper()
	require.NotNil(t, collections)
	return pbtypes.GetStringList(collections, template.CollectionStoreKey)
}

func TestEmojiTitleEdgeCases(t *testing.T) {
	cases := []struct {
		name      string
		h1        string
		wantIcon  string
		wantTitle string
	}{
		{"single emoji token", "🚀 Launch Plan", "🚀", "Launch Plan"},
		{"flag survives whole", "🇺🇸 Trip", "🇺🇸", "Trip"},
		{"zwj family survives whole", "👨‍👩‍👧 Family", "👨‍👩‍👧", "Family"},
		{"skin tone survives whole", "👍🏽 Review", "👍🏽", "Review"},
		{"bmp symbol is an icon", "⏰ Reminders", "⏰", "Reminders"},
		{"star is an icon", "⭐ Favorites", "⭐", "Favorites"},
		{"emoji-only title stays the name", "🚀", "", "🚀"},
		{"plain title untouched", "Plain Title", "", "Plain Title"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// given / when
			sink, _ := runConverterWithParams(t, map[string]string{
				"a.md": "# " + tc.h1 + "\n\nBody.\n",
			}, Params{})

			// then
			page := sink.byKey("a.md")
			require.NotNil(t, page)
			assert.Equal(t, tc.wantIcon, page.Payload.Details.GetString(bundle.RelationKeyIconEmoji))
			assert.Equal(t, tc.wantTitle, page.Payload.Details.GetString(bundle.RelationKeyName))
		})
	}
}

func TestUnreadablePageBecomesPlaceholder(t *testing.T) {
	t.Run("read failure emits an empty page so references stay valid", func(t *testing.T) {
		// given — the file disappears between pass 1 and pass 2
		root := writeTree(t, map[string]string{
			"a.md":    "See [Gone](gone.md)\n",
			"gone.md": "# Gone\n",
		})
		src, err := source.Open(root)
		require.NoError(t, err)
		t.Cleanup(func() { src.Close() })
		converter := New(src, Params{}, stubFactory{})
		require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil }))
		require.NoError(t, os.Remove(filepath.Join(root, "gone.md")))

		// when
		sink := &recordingSink{}
		_, err = converter.Convert(context.Background(), sink)

		// then — the error is reported AND the claimed page still exists
		require.NoError(t, err)
		require.NotEmpty(t, sink.issues)
		assert.Equal(t, importv2.IssueObjectFailed, sink.issues[0].Code)
		placeholder := sink.byKey("gone.md")
		require.NotNil(t, placeholder, "claimed page must be emitted as a placeholder")
		assert.Empty(t, placeholder.Payload.Blocks)
		assert.Equal(t, "gone", placeholder.Payload.Details.GetString(bundle.RelationKeyName))
	})
}

func TestInlineFileLinkBecomesMention(t *testing.T) {
	t.Run("mid-sentence link to a local file imports it and mentions it", func(t *testing.T) {
		// given / when
		sink, _ := runConverterWithParams(t, map[string]string{
			"a.md":     "Read [the spec](spec.pdf) before starting.\n",
			"spec.pdf": "pdf-bytes",
		}, Params{})

		// then — the file object is emitted and the mark mentions it
		require.NotNil(t, sink.byKey("spec.pdf"), "referenced file must be imported")
		page := sink.byKey("a.md")
		require.NotNil(t, page)
		var mention *model.BlockContentTextMark
		for _, b := range page.Payload.Blocks {
			if text := b.GetText(); text != nil {
				for _, mark := range text.GetMarks().GetMarks() {
					if mark.Type == model.BlockContentTextMark_Mention {
						mention = mark
					}
				}
			}
		}
		require.NotNil(t, mention)
		assert.Equal(t, "spec.pdf", mention.Param)
	})
}

func TestFileBlockTargetingPage(t *testing.T) {
	t.Run("image embed of a page becomes a page link, not a file object", func(t *testing.T) {
		// given / when
		sink, _ := runConverterWithParams(t, map[string]string{
			"a.md":     "![embed](other.md)\n",
			"other.md": "# Other\n",
		}, Params{})

		// then — no file object under the page's source key, a link instead
		var pages, fileObjects int
		for _, o := range sink.objects {
			if o.SourceKey == "other.md" {
				if o.File != nil {
					fileObjects++
				} else {
					pages++
				}
			}
		}
		assert.Equal(t, 1, pages, "exactly the page object under other.md")
		assert.Zero(t, fileObjects, "no colliding file object")

		page := sink.byKey("a.md")
		require.NotNil(t, page)
		var link *model.BlockContentLink
		for _, b := range page.Payload.Blocks {
			if l := b.GetLink(); l != nil {
				link = l
			}
		}
		require.NotNil(t, link)
		assert.Equal(t, "other.md", link.TargetBlockId)
	})
}

func TestCollectionFrontMatter(t *testing.T) {
	t.Run("_collection key builds the store under any profile", func(t *testing.T) {
		// given — a schema declaring the Collection property with the
		// reserved x-key, as Anytype's exporter writes it
		collectionSchema := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type": "object",
			"title": "List",
			"x-app": "Anytype",
			"x-type-key": "custom_list",
			"properties": {
				"Collection": {
					"type": "array",
					"x-key": "_collection",
					"x-format": "object",
					"items": {"type": "string"}
				}
			}
		}`

		// when
		sink, _ := runConverterWithParams(t, map[string]string{
			"list.schema.json": collectionSchema,
			"list.md":          "---\ntype: List\nCollection:\n  - one.md\n  - sub/two.md\n---\n# My List\n",
			"one.md":           "# One\n",
			"sub/two.md":       "# Two\n",
		}, Params{})

		// then — the page carries the resolved member store, no Collection relation
		page := sink.byKey("list.md")
		require.NotNil(t, page)
		assert.Equal(t, []string{"one.md", "sub/two.md"}, storeMembers(t, page.Payload.Collections))
		for _, o := range sink.objects {
			if o.SourceKey == relationSourceKey("_collection") {
				t.Fatal("the collection pseudo-property must not become a relation")
			}
		}
	})

	t.Run("Collection by name requires the anytype profile", func(t *testing.T) {
		files := map[string]string{
			"list.md": "---\nCollection:\n  - one.md\n---\n# List\n",
			"one.md":  "# One\n",
		}

		// when forced anytype-export — the name match applies
		sink, _ := runConverterWithParams(t, files, Params{Flavour: FlavourAnytypeExport})
		page := sink.byKey("list.md")
		require.NotNil(t, page)
		assert.Equal(t, []string{"one.md"}, storeMembers(t, page.Payload.Collections))

		// when generic — same input stays a plain property
		sink, _ = runConverterWithParams(t, files, Params{})
		page = sink.byKey("list.md")
		require.NotNil(t, page)
		assert.Nil(t, page.Payload.Collections)
	})

	t.Run("missing members warn and are skipped", func(t *testing.T) {
		// given / when
		sink, _ := runConverterWithParams(t, map[string]string{
			"list.md": "---\nCollection:\n  - one.md\n  - missing.md\n---\n# List\n",
			"one.md":  "# One\n",
		}, Params{Flavour: FlavourAnytypeExport})

		// then
		page := sink.byKey("list.md")
		require.NotNil(t, page)
		assert.Equal(t, []string{"one.md"}, storeMembers(t, page.Payload.Collections))
		var missingWarned bool
		for _, issue := range sink.issues {
			if issue.Code == importv2.IssueMissingTarget {
				missingWarned = true
			}
		}
		assert.True(t, missingWarned)
	})
}

func TestNotionFieldBlock(t *testing.T) {
	pageA := "Page one " + notionIdA + ".md"
	pageB := "Other page " + notionIdB + ".md"

	t.Run("property lines become mentions with clean titles", func(t *testing.T) {
		// given — a Notion-shaped export (id suffixes trigger detection)
		sink, _ := runConverterWithParams(t, map[string]string{
			pageA: "# Page one\n\nRelated: Other%20page%20" + notionIdB + ".md\nStatus: Done\n",
			pageB: "# Other page\n",
		}, Params{})

		// when / then
		page := sink.byKey(pageA)
		require.NotNil(t, page)
		require.NotEmpty(t, page.Payload.Blocks)
		text := page.Payload.Blocks[0].GetText()
		require.NotNil(t, text)
		assert.Equal(t, "Related: Other page\nStatus: Done", text.Text)
		marks := text.GetMarks().GetMarks()
		require.Len(t, marks, 1)
		assert.Equal(t, model.BlockContentTextMark_Mention, marks[0].Type)
		assert.Equal(t, pageB, marks[0].Param)
		assert.Equal(t, int32(len("Related: ")), marks[0].Range.From)
		assert.Equal(t, int32(len("Related: Other page")), marks[0].Range.To)
	})

	t.Run("field-block lines are untouched under generic", func(t *testing.T) {
		// given — same content, no Notion signature (plain names)
		sink, _ := runConverterWithParams(t, map[string]string{
			"a.md": "# A\n\nRelated: b.md\n",
			"b.md": "# B\n",
		}, Params{})

		// then — the line stays literal text
		page := sink.byKey("a.md")
		require.NotNil(t, page)
		text := page.Payload.Blocks[0].GetText()
		require.NotNil(t, text)
		assert.Equal(t, "Related: b.md", text.Text)
		assert.Empty(t, text.GetMarks().GetMarks())
	})
}

func TestNotionIdLinkResolution(t *testing.T) {
	t.Run("a moved page still resolves by its trailing id", func(t *testing.T) {
		// given — the link's path and basename are both stale; only the id
		// survives the rename
		target := "archive/Other page v2 " + notionIdB + ".md"
		sink, _ := runConverterWithParams(t, map[string]string{
			"Page one " + notionIdA + ".md": "# One\n\n[Other](Other%20page%20" + notionIdB + ".md)\n",
			target:                          "# Other\n",
		}, Params{})

		// then — the whole-line link resolves to the moved page
		page := sink.byKey("Page one " + notionIdA + ".md")
		require.NotNil(t, page)
		var link *model.BlockContentLink
		for _, b := range page.Payload.Blocks {
			if l := b.GetLink(); l != nil {
				link = l
			}
		}
		require.NotNil(t, link)
		assert.Equal(t, target, link.TargetBlockId)
	})
}

func TestDirTreeCollapsesCommonPrefix(t *testing.T) {
	t.Run("deep single-branch trees root at the deepest common dir", func(t *testing.T) {
		// given / when
		sink, rootSpec := runConverterWithParams(t, map[string]string{
			"export/vault/notes/a.md":     "# A\n",
			"export/vault/notes/sub/b.md": "# B\n",
		}, Params{CreateDirectoryPages: true})

		// then — root is the deepest common dir, no empty chain pages
		assert.Equal(t, dirSourceKey("export/vault/notes"), rootSpec.RootObjectKey)
		assert.Nil(t, sink.byKey(dirSourceKey("export")), "no empty intermediate page")
		assert.Nil(t, sink.byKey(dirSourceKey("export/vault")), "no empty intermediate page")
		root := sink.byKey(dirSourceKey("export/vault/notes"))
		require.NotNil(t, root)
		assert.Equal(t, "notes", root.Payload.Details.GetString(bundle.RelationKeyName))
	})

	t.Run("documents in a directory page list alphabetically across md and csv", func(t *testing.T) {
		// given / when
		sink, _ := runConverterWithParams(t, map[string]string{
			"dir/b.md":  "# B\n",
			"dir/a.csv": "x\n",
			"dir/c.md":  "# C\n",
		}, Params{CreateDirectoryPages: true})

		// then
		dirPage := sink.byKey(dirSourceKey("dir"))
		require.NotNil(t, dirPage)
		targets := make([]string, 0, len(dirPage.Payload.Blocks))
		for _, b := range dirPage.Payload.Blocks {
			targets = append(targets, b.GetLink().GetTargetBlockId())
		}
		assert.Equal(t, []string{"dir/a.csv", "dir/b.md", "dir/c.md"}, targets)
	})
}

func TestCsvTypeSuggestion(t *testing.T) {
	dirName := "Tasks " + notionIdA

	t.Run("csv title types member pages under notion-export", func(t *testing.T) {
		// given / when — id-suffixed csv + dir trip the notion signature
		sink, _ := runConverterWithParams(t, map[string]string{
			dirName + ".csv":    "Name,Done\n",
			dirName + "/one.md": "# One\n",
			dirName + "/two.md": "---\ntype: Meeting\n---\n# Two\n",
		}, Params{})

		// then — typeless members become Tasks, explicit types win
		one := sink.byKey(dirName + "/one.md")
		require.NotNil(t, one)
		assert.Equal(t, []string{bundle.TypeKeyTask.String()}, one.Payload.ObjectTypes)

		two := sink.byKey(dirName + "/two.md")
		require.NotNil(t, two)
		assert.NotEqual(t, []string{bundle.TypeKeyTask.String()}, two.Payload.ObjectTypes,
			"explicit front-matter type wins over the suggestion")

		var suggested bool
		for _, issue := range sink.issues {
			if issue.Code == importv2.IssueTypeSuggested {
				suggested = true
				assert.Equal(t, importv2.SeverityInfo, issue.Severity)
			}
		}
		assert.True(t, suggested, "adopted suggestion must be reported")
	})

	t.Run("no suggestion under a forced generic profile", func(t *testing.T) {
		// given / when
		sink, _ := runConverterWithParams(t, map[string]string{
			"Tasks.csv":    "Name\n",
			"Tasks/one.md": "# One\n",
		}, Params{Flavour: FlavourGeneric})

		// then
		one := sink.byKey("Tasks/one.md")
		require.NotNil(t, one)
		assert.Equal(t, []string{bundle.TypeKeyPage.String()}, one.Payload.ObjectTypes)
	})
}

func TestTypeFeaturedRelations(t *testing.T) {
	t.Run("first three properties are featured, the rest recommended", func(t *testing.T) {
		// given / when
		sink, _ := runConverterWithParams(t, map[string]string{
			"a.md": "---\nAlpha: 1\nBeta: 2\nGamma: 3\nDelta: 4\ntype: Zettel\n---\n# A\n",
		}, Params{})

		// then
		typeObject := sink.byKey(typeSourceKey("Zettel"))
		require.NotNil(t, typeObject)
		featured := typeObject.Payload.Details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)
		rest := typeObject.Payload.Details.GetStringList(bundle.RelationKeyRecommendedRelations)
		assert.Len(t, featured, 3)
		assert.Len(t, rest, 1)
	})
}
