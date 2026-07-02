package markdown

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type recordingSink struct {
	objects []*importv2.Object
	issues  []importv2.Issue
}

func (s *recordingSink) Object(ctx context.Context, o *importv2.Object) error {
	s.objects = append(s.objects, o)
	return nil
}

func (s *recordingSink) Issue(i importv2.Issue) { s.issues = append(s.issues, i) }
func (s *recordingSink) Progress(delta int64)   {}

func (s *recordingSink) byKey(sourceKey string) *importv2.Object {
	for _, o := range s.objects {
		if o.SourceKey == sourceKey {
			return o
		}
	}
	return nil
}

func (s *recordingSink) keys() []string {
	keys := make([]string, 0, len(s.objects))
	for _, o := range s.objects {
		keys = append(keys, o.SourceKey)
	}
	return keys
}

type stubFactory struct{}

func (stubFactory) MakeCollection(name string, memberSourceKeys []string) (*importv2.Object, error) {
	details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName: domain.String(name),
	})
	snapshot := &importv2.Snapshot{Details: details, ObjectTypes: []string{bundle.TypeKeyCollection.String()}}
	_ = memberSourceKeys // membership travels via the store in the real factory
	return &importv2.Object{SbType: coresb.SmartBlockTypePage, Payload: snapshot}, nil
}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	return root
}

func runConverter(t *testing.T, files map[string]string) (*recordingSink, importv2.RootSpec, []importv2.IdentityClaim) {
	t.Helper()
	src, err := source.Open(writeTree(t, files))
	require.NoError(t, err)
	t.Cleanup(func() { src.Close() })
	converter := New(src, Params{}, stubFactory{})

	var claims []importv2.IdentityClaim
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(c importv2.IdentityClaim) error {
		claims = append(claims, c)
		return nil
	}))
	sink := &recordingSink{}
	rootSpec, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)
	return sink, rootSpec, claims
}

func TestEnumerate(t *testing.T) {
	t.Run("claims pages and csv collections, rejects md-less source", func(t *testing.T) {
		// given / when
		_, _, claims := runConverter(t, map[string]string{
			"a.md":       "# A",
			"Tasks.csv":  "unused",
			"img.png":    "binary",
			"Tasks/b.md": "# B",
		})

		// then — md + csv claimed, binary not
		require.Len(t, claims, 3)
		for _, c := range claims {
			assert.Equal(t, coresb.SmartBlockTypePage, c.SbType)
			assert.NotEmpty(t, c.SourceFilePath)
		}

		// and an md-less source is fatal
		src, err := source.Open(writeTree(t, map[string]string{"only.png": "x"}))
		require.NoError(t, err)
		defer src.Close()
		enumErr := New(src, Params{}, stubFactory{}).EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil })
		issue := importv2.AsIssue(enumErr, importv2.SeverityFatal, importv2.IssueStoreError)
		assert.Equal(t, importv2.IssueNoObjects, issue.Code)
	})
}

func TestFrontMatter(t *testing.T) {
	t.Run("emits relation, option and type definitions before the page", func(t *testing.T) {
		// given / when
		sink, _, _ := runConverter(t, map[string]string{
			"book.md": "---\nAuthor: Roman\nStatus: In Progress\ntype: Zettel\n---\n# My Title\n\nBody text.\n",
		})

		// then — order: definitions first, page last
		keys := sink.keys()
		require.NotEmpty(t, keys)
		assert.Equal(t, "book.md", keys[len(keys)-1])

		var relation, option, typeObject, page *importv2.Object
		for _, o := range sink.objects {
			switch o.SbType {
			case coresb.SmartBlockTypeRelation:
				relation = o
			case coresb.SmartBlockTypeRelationOption:
				option = o
			case coresb.SmartBlockTypeObjectType:
				typeObject = o
			case coresb.SmartBlockTypePage:
				page = o
			}
		}
		require.NotNil(t, relation, "custom Author relation must be emitted")
		assert.Equal(t, "Roman", mustString(t, page, domain.RelationKey(relation.Payload.Key)))
		assert.Equal(t, model.RelationFormat_shorttext, model.RelationFormat(relation.Payload.Details.GetInt64(bundle.RelationKeyRelationFormat)))

		require.NotNil(t, option, "Status option must be emitted")
		assert.Equal(t, "In Progress", option.Payload.Details.GetString(bundle.RelationKeyName))
		statusKey := domain.RelationKey(option.Payload.Details.GetString(bundle.RelationKeyRelationKey))
		assert.Equal(t, []string{option.SourceKey}, page.Payload.Details.GetStringList(statusKey),
			"page status detail must reference the option by source key")

		require.NotNil(t, typeObject)
		assert.Equal(t, "Zettel", typeObject.Payload.Details.GetString(bundle.RelationKeyName))
		assert.Equal(t, []string{typeObject.Payload.Key}, page.Payload.ObjectTypes)

		// H1 became the title and was stripped from blocks
		assert.Equal(t, "My Title", page.Payload.Details.GetString(bundle.RelationKeyName))
		for _, b := range page.Payload.Blocks {
			if text := b.GetText(); text != nil {
				assert.NotEqual(t, "My Title", text.Text)
			}
		}
	})

	t.Run("same source yields identical emissions (determinism)", func(t *testing.T) {
		// given
		files := map[string]string{
			"a.md": "---\nMood: [happy, calm]\n---\n# A\n",
			"b.md": "---\nMood: [calm]\n---\n# B\n",
		}

		// when — two independent runs
		first, _, _ := runConverter(t, files)
		second, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, first.keys(), second.keys())
		firstOption := first.objects[1]
		secondOption := second.objects[1]
		assert.Equal(t, firstOption.Payload.Key, secondOption.Payload.Key)
		assert.Equal(t,
			firstOption.Payload.Details.GetString(bundle.RelationKeyRelationOptionColor),
			secondOption.Payload.Details.GetString(bundle.RelationKeyRelationOptionColor))
	})
}

func TestLinks(t *testing.T) {
	files := map[string]string{
		"a.md":     "See [Other](other.md) inline.\n\n[Other](other.md)\n\n![pic](img.png)\n\n[Site](https://anytype.io)\n",
		"other.md": "# Other\n",
		"img.png":  "png-bytes",
	}

	t.Run("marks, page links, files and bookmarks rewrite to source keys", func(t *testing.T) {
		// given / when
		sink, _, _ := runConverter(t, files)
		page := sink.byKey("a.md")
		require.NotNil(t, page)

		var mention *model.BlockContentTextMark
		var pageLink *model.BlockContentLink
		var fileBlock *model.BlockContentFile
		var bookmark *model.BlockContentBookmark
		for _, b := range page.Payload.Blocks {
			if text := b.GetText(); text != nil {
				for _, mark := range text.GetMarks().GetMarks() {
					if mark.Type == model.BlockContentTextMark_Mention {
						mention = mark
					}
				}
			}
			if l := b.GetLink(); l != nil {
				pageLink = l
			}
			if f := b.GetFile(); f != nil {
				fileBlock = f
			}
			if bm := b.GetBookmark(); bm != nil {
				bookmark = bm
			}
		}
		require.NotNil(t, mention, "inline md link becomes a mention")
		assert.Equal(t, "other.md", mention.Param)
		require.NotNil(t, pageLink, "whole-line md link becomes a page link block")
		assert.Equal(t, "other.md", pageLink.TargetBlockId)
		require.NotNil(t, fileBlock, "image becomes a file block")
		assert.Equal(t, "img.png", fileBlock.TargetObjectId)
		require.NotNil(t, bookmark, "whole-line url becomes a bookmark")
		assert.Equal(t, "https://anytype.io", bookmark.Url)

		fileObject := sink.byKey("img.png")
		require.NotNil(t, fileObject, "referenced file is emitted as a file object")
		assert.Equal(t, coresb.SmartBlockTypeFileObject, fileObject.SbType)
		assert.NotEmpty(t, fileObject.File.Path)
	})

	t.Run("missing file reference degrades with a warning", func(t *testing.T) {
		// given / when
		sink, _, _ := runConverter(t, map[string]string{"a.md": "![gone](gone.png)\n"})

		// then
		require.NotEmpty(t, sink.issues)
		assert.Equal(t, importv2.IssueMissingTarget, sink.issues[0].Code)
		assert.Nil(t, sink.byKey("gone.png"))
	})
}

func TestCsvCollections(t *testing.T) {
	t.Run("csv wraps its sibling directory pages", func(t *testing.T) {
		// given / when
		sink, rootSpec, _ := runConverter(t, map[string]string{
			"Tasks.csv":    "Name,Status\n",
			"Tasks/a.md":   "# A\n",
			"Tasks/b.md":   "# B\n",
			"unrelated.md": "# U\n",
		})

		// then
		collection := sink.byKey("Tasks.csv")
		require.NotNil(t, collection)
		assert.True(t, collection.IsRootCandidate)
		assert.Equal(t, "Tasks", collection.Payload.Details.GetString(bundle.RelationKeyName))
		assert.Equal(t, model.BlockContentWidget_Link, rootSpec.WidgetLayout)
		assert.Equal(t, "Markdown Import", rootSpec.CollectionName)
	})
}

func mustString(t *testing.T, o *importv2.Object, key domain.RelationKey) string {
	t.Helper()
	require.NotNil(t, o)
	return o.Payload.Details.GetString(key)
}
