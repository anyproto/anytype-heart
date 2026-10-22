package markdown

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type createdIn struct {
	parent string
	ref    string
}

func createdInOf(t *testing.T, sink *recordingSink, sourceKey string) createdIn {
	t.Helper()
	object := sink.byKey(sourceKey)
	require.NotNil(t, object, "object %q must be emitted", sourceKey)
	details := object.Payload.Details
	return createdIn{
		parent: details.GetString(bundle.RelationKeyCreatedInContext),
		ref:    details.GetString(bundle.RelationKeyCreatedInContextRef),
	}
}

// linkBlockTo returns the id of the page's link block targeting target.
func linkBlockTo(t *testing.T, sink *recordingSink, pageKey, target string) string {
	t.Helper()
	page := sink.byKey(pageKey)
	require.NotNil(t, page)
	for _, b := range page.Payload.Blocks {
		if b.GetLink().GetTargetBlockId() == target {
			return b.Id
		}
	}
	require.Failf(t, "no link block", "%q has no link to %q", pageKey, target)
	return ""
}

// mentionBlockTo returns the id of the page's text block mentioning target.
func mentionBlockTo(t *testing.T, sink *recordingSink, pageKey, target string) string {
	t.Helper()
	page := sink.byKey(pageKey)
	require.NotNil(t, page)
	for _, b := range page.Payload.Blocks {
		for _, mark := range b.GetText().GetMarks().GetMarks() {
			if mark.Type == model.BlockContentTextMark_Mention && mark.Param == target {
				return b.Id
			}
		}
	}
	require.Failf(t, "no mention", "%q has no mention of %q", pageKey, target)
	return ""
}

func TestCreatedInContext(t *testing.T) {
	t.Run("a page linking documents in a directory below it is their parent", func(t *testing.T) {
		// given
		files := map[string]string{
			"a.md":     "# A\n\n[X](dir/x.md)\n",
			"b.md":     "# B\n\nSee [Y](dir/y.md) too.\n",
			"dir/x.md": "# X\n",
			"dir/y.md": "# Y\n",
			"dir/z.md": "# Z\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{parent: "a.md", ref: linkBlockTo(t, sink, "a.md", "dir/x.md")}, createdInOf(t, sink, "dir/x.md"))
		assert.Equal(t, createdIn{parent: "b.md", ref: mentionBlockTo(t, sink, "b.md", "dir/y.md")}, createdInOf(t, sink, "dir/y.md"),
			"an inline link is a mention; the block holding it is the ref")
		assert.Equal(t, createdIn{}, createdInOf(t, sink, "dir/z.md"), "no page links it")
		assert.Equal(t, createdIn{}, createdInOf(t, sink, "a.md"), "a top-level page has no context")
	})

	t.Run("a parent sorting after its child is still seen first", func(t *testing.T) {
		// given — lexicographically dir/x.md comes before z.md
		files := map[string]string{
			"z.md":     "# Z\n\n[X](dir/x.md)\n",
			"dir/x.md": "# X\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{parent: "z.md", ref: linkBlockTo(t, sink, "z.md", "dir/x.md")}, createdInOf(t, sink, "dir/x.md"))
	})

	t.Run("pages in one directory linking each other stay siblings", func(t *testing.T) {
		// given
		files := map[string]string{
			"a.md": "# A\n\n[B](b.md)\n",
			"b.md": "# B\n\n[A](a.md)\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{}, createdInOf(t, sink, "a.md"))
		assert.Equal(t, createdIn{}, createdInOf(t, sink, "b.md"))
	})

	t.Run("a link upwards does not make the deeper page a parent", func(t *testing.T) {
		// given
		files := map[string]string{
			"a.md":     "# A\n",
			"dir/x.md": "# X\n\n[A](../a.md)\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{}, createdInOf(t, sink, "a.md"))
	})

	t.Run("the closest linking page wins over one further up", func(t *testing.T) {
		// given
		files := map[string]string{
			"index.md":     "# Index\n\n[Deep](a/b/deep.md)\n",
			"a/section.md": "# Section\n\n[Deep](b/deep.md)\n",
			"a/b/deep.md":  "# Deep\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{
			parent: "a/section.md",
			ref:    linkBlockTo(t, sink, "a/section.md", "a/b/deep.md"),
		}, createdInOf(t, sink, "a/b/deep.md"))
	})

	t.Run("a hub linking into several directories is no parent", func(t *testing.T) {
		// given — Home links into two subtrees; Notes links into one
		files := map[string]string{
			"Home.md":       "# Home\n\n[A](Projects/a.md)\n\n[B](Areas/b.md)\n",
			"Notes.md":      "# Notes\n\n[B](Areas/b.md)\n",
			"Projects/a.md": "# A\n",
			"Areas/b.md":    "# B\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{}, createdInOf(t, sink, "Projects/a.md"))
		assert.Equal(t, createdIn{parent: "Notes.md", ref: linkBlockTo(t, sink, "Notes.md", "Areas/b.md")}, createdInOf(t, sink, "Areas/b.md"))
	})

	t.Run("a page two directories up linking a document is not its parent", func(t *testing.T) {
		// given
		files := map[string]string{
			"index.md":    "# Index\n\n[Deep](a/b/deep.md)\n",
			"a/b/deep.md": "# Deep\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{}, createdInOf(t, sink, "a/b/deep.md"))
	})

	t.Run("folder note beside the folder holds every document in it, linked or not", func(t *testing.T) {
		// given — dir.md wins over a.md linking from above
		files := map[string]string{
			"a.md":     "# A\n\n[X](dir/x.md)\n",
			"dir.md":   "# Dir\n\n[Y](dir/y.md)\n",
			"dir/x.md": "# X\n",
			"dir/y.md": "# Y\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{parent: "dir.md"}, createdInOf(t, sink, "dir/x.md"))
		assert.Equal(t, createdIn{parent: "dir.md", ref: linkBlockTo(t, sink, "dir.md", "dir/y.md")}, createdInOf(t, sink, "dir/y.md"))
	})

	for _, note := range []string{"Topic/Topic.md", "Topic/index.md", "Topic/_index.md", "Topic/README.md"} {
		t.Run("folder note inside the folder: "+note, func(t *testing.T) {
			// given
			files := map[string]string{
				"Area.md":             "# Area\n\n[Topic](Area/Topic/)\n",
				"Area/" + note:        "# Topic\n\n[Leaf](leaf.md)\n",
				"Area/Topic/leaf.md":  "# Leaf\n",
				"Area/Topic/other.md": "# Other\n",
			}

			// when
			sink, _, _ := runConverter(t, files)

			// then
			noteKey := "Area/" + note
			assert.Equal(t, createdIn{parent: noteKey, ref: linkBlockTo(t, sink, noteKey, "Area/Topic/leaf.md")},
				createdInOf(t, sink, "Area/Topic/leaf.md"), "the note is converted before its siblings, so the ref is known")
			assert.Equal(t, createdIn{parent: noteKey}, createdInOf(t, sink, "Area/Topic/other.md"))
			assert.Equal(t, "Area.md", createdInOf(t, sink, noteKey).parent, "the note sits where its folder does")
		})
	}

	t.Run("front matter parent or up wins over the folder", func(t *testing.T) {
		// given
		files := map[string]string{
			"dir.md":        "# Dir\n",
			"Hub.md":        "# Hub\n\n[Child](dir/child.md)\n",
			"dir/child.md":  "---\nup: \"[[Hub]]\"\n---\n# Child\n",
			"dir/other.md":  "---\nParent:\n  - ../Hub.md\n---\n# Other\n",
			"dir/broken.md": "---\nparent: \"[[Nowhere]]\"\n---\n# Broken\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{parent: "Hub.md", ref: linkBlockTo(t, sink, "Hub.md", "dir/child.md")}, createdInOf(t, sink, "dir/child.md"))
		assert.Equal(t, createdIn{parent: "Hub.md"}, createdInOf(t, sink, "dir/other.md"))
		assert.Equal(t, createdIn{parent: "dir.md"}, createdInOf(t, sink, "dir/broken.md"), "an unresolvable value falls through to the folder")
	})

	t.Run("without a closer parent the nearest ancestor folder note holds the document", func(t *testing.T) {
		// given
		files := map[string]string{
			"Projects.md":            "# Projects\n",
			"Projects/Alpha/spec.md": "# Spec\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{parent: "Projects.md"}, createdInOf(t, sink, "Projects/Alpha/spec.md"))
	})

	t.Run("an embedded page counts as a link, for the ref and for hub detection", func(t *testing.T) {
		// given — Home links one subtree and embeds another: a hub
		files := map[string]string{
			"Home.md":       "# Home\n\n[A](Projects/a.md)\n\n![B](Areas/b.md)\n",
			"Projects.md":   "# Projects\n\n![A](Projects/a.md)\n",
			"Projects/a.md": "# A\n",
			"Areas/b.md":    "# B\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{parent: "Projects.md", ref: linkBlockTo(t, sink, "Projects.md", "Projects/a.md")},
			createdInOf(t, sink, "Projects/a.md"))
		assert.Equal(t, createdIn{}, createdInOf(t, sink, "Areas/b.md"), "Home is a hub through its embed")
	})

	t.Run("a folder note directly under the root takes the page linking it", func(t *testing.T) {
		// given
		files := map[string]string{
			"Manual.md":      "# Manual\n\n[Guide](guide/index.md)\n",
			"guide/index.md": "# Guide\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{parent: "Manual.md", ref: linkBlockTo(t, sink, "Manual.md", "guide/index.md")},
			createdInOf(t, sink, "guide/index.md"))
	})

	t.Run("an edge closing a cycle falls through to the next rule", func(t *testing.T) {
		// given — Area names its own child as parent; the child's folder rule
		// names Area; front matter a<->b
		files := map[string]string{
			"Area.md":       "---\nparent: Area/child.md\n---\n# Area\n",
			"Area/child.md": "# Child\n",
			"a.md":          "---\nup: b.md\n---\n# A\n",
			"b.md":          "---\nup: a.md\n---\n# B\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{parent: "Area/child.md"}, createdInOf(t, sink, "Area.md"))
		assert.Equal(t, createdIn{}, createdInOf(t, sink, "Area/child.md"), "Area already hangs under the child")
		assert.Equal(t, createdIn{parent: "b.md"}, createdInOf(t, sink, "a.md"))
		assert.Equal(t, createdIn{}, createdInOf(t, sink, "b.md"))
	})

	t.Run("a root-relative front matter path resolves from the source root", func(t *testing.T) {
		// given — two Parent.md, so the basename fallback cannot pick one
		files := map[string]string{
			"parents/Parent.md": "# Parent\n",
			"other/Parent.md":   "# Other parent\n",
			"child/doc.md":      "---\nparent: /parents/Parent.md\n---\n# Doc\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{parent: "parents/Parent.md"}, createdInOf(t, sink, "child/doc.md"))
	})

	t.Run("a cyclic linker gives way to the next page linking the document", func(t *testing.T) {
		// given — a.md hangs under the child; b.md links it too
		files := map[string]string{
			"a.md":         "---\nparent: dir/child.md\n---\n# A\n\n[C](dir/child.md)\n",
			"b.md":         "# B\n\n[C](dir/child.md)\n",
			"dir/child.md": "# Child\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{parent: "b.md", ref: linkBlockTo(t, sink, "b.md", "dir/child.md")}, createdInOf(t, sink, "dir/child.md"))
	})

	t.Run("a front matter parent closing a cycle falls through to up", func(t *testing.T) {
		// given
		files := map[string]string{
			"a.md": "---\nparent: b.md\n---\n# A\n",
			"b.md": "---\nparent: a.md\nup: c.md\n---\n# B\n",
			"c.md": "# C\n",
			"d.md": "---\nparent: d.md\nup: c.md\n---\n# D\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{parent: "b.md"}, createdInOf(t, sink, "a.md"))
		assert.Equal(t, createdIn{parent: "c.md"}, createdInOf(t, sink, "b.md"))
		assert.Equal(t, createdIn{parent: "c.md"}, createdInOf(t, sink, "d.md"), "a self-reference is skipped too")
	})

	t.Run("an extensionless front matter reference finds an uppercase extension", func(t *testing.T) {
		// given — without it, the folder note would take the child
		files := map[string]string{
			"Parent.MD":       "# Parent\n",
			"Folder.md":       "# Folder\n",
			"Folder/child.md": "---\nparent: \"[[Parent]]\"\n---\n# Child\n",
			"Folder/other.md": "---\nup: /Parent\n---\n# Other\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{parent: "Parent.MD"}, createdInOf(t, sink, "Folder/child.md"))
		assert.Equal(t, createdIn{parent: "Parent.MD"}, createdInOf(t, sink, "Folder/other.md"))
	})

	t.Run("a folder note with an uppercase extension", func(t *testing.T) {
		// given
		files := map[string]string{
			"Topic/index.MD": "# Topic\n\n[Leaf](leaf.md)\n",
			"Topic/leaf.md":  "# Leaf\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{parent: "Topic/index.MD", ref: linkBlockTo(t, sink, "Topic/index.MD", "Topic/leaf.md")},
			createdInOf(t, sink, "Topic/leaf.md"))
	})

	t.Run("front matter parent wins over up, deterministically", func(t *testing.T) {
		// given
		files := map[string]string{
			"a.md":   "# A\n",
			"b.md":   "# B\n",
			"doc.md": "---\nup: b.md\nparent: a.md\n---\n# Doc\n",
		}

		for range 20 {
			// when
			sink, _, _ := runConverter(t, files)

			// then
			assert.Equal(t, createdIn{parent: "a.md"}, createdInOf(t, sink, "doc.md"))
		}
	})

	t.Run("rows of an uppercase .CSV collection", func(t *testing.T) {
		// given
		files := map[string]string{
			"Tasks.CSV":      "Name\nOne\n",
			"Tasks/one.md":   "# One\n",
			"Tasks/index.md": "# Index\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{parent: "Tasks.CSV"}, createdInOf(t, sink, "Tasks/one.md"))
	})

	t.Run("csv rows are created in their collection without a ref", func(t *testing.T) {
		// given
		files := map[string]string{
			"Home.md":           "# Home\n\n[Tasks](Home/Tasks.csv)\n",
			"Home/Tasks.csv":    "Name\nOne\n",
			"Home/Tasks/One.md": "# One\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{parent: "Home/Tasks.csv"}, createdInOf(t, sink, "Home/Tasks/One.md"))
		assert.Equal(t, createdIn{
			parent: "Home.md",
			ref:    linkBlockTo(t, sink, "Home.md", "Home/Tasks.csv"),
		}, createdInOf(t, sink, "Home/Tasks.csv"))
	})

	t.Run("directory pages: documents and subdirectories are created in their directory page", func(t *testing.T) {
		// given
		files := map[string]string{
			"top.md":   "# Top\n",
			"a/x.md":   "# X\n",
			"a/b/y.md": "# Y\n",
			"a/b/z.md": "# Z\n",
		}

		// when
		sink, _ := runConverterWithParams(t, files, Params{CreateDirectoryPages: true})

		// then
		for _, tc := range []struct {
			sourceKey string
			parent    string
		}{
			{"top.md", "dir:."},
			{"a/x.md", "dir:a"},
			{"a/b/y.md", "dir:a/b"},
			{"a/b/z.md", "dir:a/b"},
			{"dir:a/b", "dir:a"},
			{"dir:a", "dir:."},
		} {
			assert.Equal(t, createdIn{
				parent: tc.parent,
				ref:    linkBlockTo(t, sink, tc.parent, tc.sourceKey),
			}, createdInOf(t, sink, tc.sourceKey), tc.sourceKey)
		}
		assert.Equal(t, createdIn{}, createdInOf(t, sink, "dir:."), "the root directory page has no context")
	})

	t.Run("front matter cannot set the context", func(t *testing.T) {
		// given
		files := map[string]string{
			"doc.md": "---\ncreatedInContext: somewhere\ncreatedInContextRef: block\n---\n# Doc\n",
		}

		// when
		sink, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, createdIn{}, createdInOf(t, sink, "doc.md"))
	})
}

func TestStableBlockIds(t *testing.T) {
	files := map[string]string{
		"doc.md": "# Doc\n\nIntro\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\n- item\n  - nested\n",
	}

	t.Run("converting the same file twice yields the same block ids", func(t *testing.T) {
		// given / when
		first, _, _ := runConverter(t, files)
		second, _, _ := runConverter(t, files)

		// then
		assert.Equal(t, first.byKey("doc.md").Payload.Blocks, second.byKey("doc.md").Payload.Blocks)
	})

	t.Run("table cells keep their row-column ids and every child id exists", func(t *testing.T) {
		// given / when
		sink, _, _ := runConverter(t, files)
		blocks := sink.byKey("doc.md").Payload.Blocks

		// then
		ids := map[string]bool{}
		for _, b := range blocks {
			ids[b.Id] = true
		}
		cells := 0
		for _, b := range blocks {
			for _, child := range b.ChildrenIds {
				assert.True(t, ids[child], "child %q of %q must exist", child, b.Id)
			}
			if rowId, colId, isCell := strings.Cut(b.Id, "-"); isCell {
				cells++
				assert.True(t, ids[rowId], "cell %q must name an existing row", b.Id)
				assert.True(t, ids[colId], "cell %q must name an existing column", b.Id)
			}
		}
		assert.Positive(t, cells, "the table must have cells")
	})
}
