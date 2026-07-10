package report

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func blockById(t *testing.T, blocks []*model.Block, id string) *model.Block {
	t.Helper()
	for _, b := range blocks {
		if b.Id == id {
			return b
		}
	}
	t.Fatalf("block %q not found", id)
	return nil
}

func textOf(t *testing.T, blocks []*model.Block, id string) string {
	t.Helper()
	return blockById(t, blocks, id).GetText().GetText()
}

func TestBuild(t *testing.T) {
	resolveAll := func(key string) (string, bool) { return "id-" + key, true }
	resolveNone := func(key string) (string, bool) { return "", false }

	t.Run("groups sort by severity then code, issues keep stream order", func(t *testing.T) {
		// given
		issues := []importv2.Issue{
			importv2.Warning(importv2.IssueMissingTarget, "b.md", "second warning group"),
			importv2.ObjectError(importv2.IssueObjectFailed, "x.md", errors.New("boom")),
			importv2.Warning(importv2.IssueDataLoss, "a.md", "first warning group"),
			importv2.Warning(importv2.IssueDataLoss, "c.md", "same group, later"),
		}

		// when
		object := Build("Import report — test", issues, 0, resolveNone)

		// then
		require.Equal(t, SourceKey, object.SourceKey)
		blocks := object.Payload.Blocks
		// objectError group first, then warnings by code: dataLoss < missingTarget
		assert.Contains(t, textOf(t, blocks, "group0"), "objectFailed (1)")
		assert.Contains(t, textOf(t, blocks, "group1"), "dataLoss (2)")
		assert.Contains(t, textOf(t, blocks, "group2"), "missingTarget (1)")
		toggle := blockById(t, blocks, "group1")
		require.Equal(t, []string{"group1-0", "group1-1"}, toggle.ChildrenIds)
		assert.Equal(t, "a.md — first warning group", textOf(t, blocks, "group1-0"))
		assert.Equal(t, "c.md — same group, later", textOf(t, blocks, "group1-1"))
	})

	t.Run("summary table rows mirror the groups", func(t *testing.T) {
		// given
		issues := []importv2.Issue{
			importv2.Warning(importv2.IssueDataLoss, "a.md", "lost"),
			importv2.Warning(importv2.IssueDataLoss, "b.md", "lost too"),
		}

		// when
		object := Build("t", issues, 0, resolveNone)

		// then
		blocks := object.Payload.Blocks
		table := blockById(t, blocks, "summary")
		require.Equal(t, []string{"summary-columns", "summary-rows"}, table.ChildrenIds)
		rows := blockById(t, blocks, "summary-rows")
		require.Equal(t, []string{"sumr0", "sumr1"}, rows.ChildrenIds)
		assert.True(t, blockById(t, blocks, "sumr0").GetTableRow().GetIsHeader())
		assert.Equal(t, "warning", textOf(t, blocks, "sumr1-sumc0"))
		assert.Equal(t, "dataLoss", textOf(t, blocks, "sumr1-sumc1"))
		assert.Equal(t, "2", textOf(t, blocks, "sumr1-sumc2"))
	})

	t.Run("resolvable source key gets a mention mark with the key as param", func(t *testing.T) {
		// given
		issues := []importv2.Issue{importv2.Warning(importv2.IssueDataLoss, "страница.md", "lost")}

		// when
		object := Build("t", issues, 0, resolveAll)

		// then
		line := blockById(t, object.Payload.Blocks, "group0-0")
		marks := line.GetText().GetMarks().GetMarks()
		require.Len(t, marks, 1)
		assert.Equal(t, model.BlockContentTextMark_Mention, marks[0].Type)
		assert.Equal(t, "страница.md", marks[0].Param, "resolver rewrites source keys, not final ids")
		assert.Equal(t, int32(11), marks[0].Range.To, "UTF-16 length of the source key")
	})

	t.Run("unresolvable source key stays plain text", func(t *testing.T) {
		// when
		object := Build("t", []importv2.Issue{importv2.Warning(importv2.IssueDataLoss, "gone.md", "lost")}, 0, resolveNone)

		// then
		line := blockById(t, object.Payload.Blocks, "group0-0")
		assert.Empty(t, line.GetText().GetMarks().GetMarks())
	})

	t.Run("overflow renders a table row and a trailing line", func(t *testing.T) {
		// when
		object := Build("t", []importv2.Issue{importv2.Warning(importv2.IssueDataLoss, "a.md", "x")}, 7, resolveNone)

		// then
		blocks := object.Payload.Blocks
		assert.Contains(t, textOf(t, blocks, "overflow"), "7 further issues")
		rows := blockById(t, blocks, "summary-rows")
		assert.Equal(t, []string{"sumr0", "sumr1", "sumr2"}, rows.ChildrenIds)
		assert.Equal(t, "7", textOf(t, blocks, "sumr2-sumc2"))
	})

	t.Run("details carry name and icon, type is page", func(t *testing.T) {
		// when
		object := Build("Import report — Notion Import", []importv2.Issue{importv2.Warning(importv2.IssueDataLoss, "a", "x")}, 0, resolveNone)

		// then
		assert.Equal(t, "Import report — Notion Import", object.Payload.Details.GetString(bundle.RelationKeyName))
		assert.NotEmpty(t, object.Payload.Details.GetString(bundle.RelationKeyIconEmoji))
		assert.Equal(t, []string{bundle.TypeKeyPage.String()}, object.Payload.ObjectTypes)
	})
}
