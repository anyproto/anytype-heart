package resolve

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type fakeRefs struct {
	ids     map[string]string
	failed  map[string]error
	blocked map[string]chan string // file futures: wait until a value arrives
}

func (f *fakeRefs) ResolveRef(ctx context.Context, sourceKey string) (string, bool, error) {
	if err, ok := f.failed[sourceKey]; ok {
		return "", true, err
	}
	if ch, ok := f.blocked[sourceKey]; ok {
		select {
		case id := <-ch:
			return id, true, nil
		case <-ctx.Done():
			return "", true, ctx.Err()
		}
	}
	id, ok := f.ids[sourceKey]
	return id, ok, nil
}

type fakeKeys map[string]string

func (f fakeKeys) FinalKey(sourceKey string) (string, bool) {
	finalKey, ok := f[sourceKey]
	return finalKey, ok
}

type issueCollector struct {
	issues []importv2.Issue
}

func (c *issueCollector) report(i importv2.Issue) {
	c.issues = append(c.issues, i)
}

func (c *issueCollector) codes() []importv2.IssueCode {
	codes := make([]importv2.IssueCode, 0, len(c.issues))
	for _, i := range c.issues {
		codes = append(codes, i.Code)
	}
	return codes
}

func newResolver(refs *fakeRefs, keys fakeKeys) *Resolver {
	if refs.ids == nil {
		refs.ids = map[string]string{}
	}
	return New(refs, keys, NewFormats())
}

func docWithBlocks(blocks ...*model.Block) *state.State {
	blockMap := map[string]simple.Block{}
	childrenIds := make([]string, 0, len(blocks))
	for _, b := range blocks {
		blockMap[b.Id] = simple.New(b)
		childrenIds = append(childrenIds, b.Id)
	}
	blockMap["root"] = simple.New(&model.Block{Id: "root", ChildrenIds: childrenIds})
	return state.NewDoc("root", blockMap).(*state.State)
}

func TestRewriteBlocks(t *testing.T) {
	t.Run("link block resolves, unknown target becomes missing marker with issue", func(t *testing.T) {
		// given
		st := docWithBlocks(
			&model.Block{Id: "l1", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "docs/a.md"}}},
			&model.Block{Id: "l2", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "docs/gone.md"}}},
		)
		r := newResolver(&fakeRefs{ids: map[string]string{"docs/a.md": "idA"}}, fakeKeys{})
		c := &issueCollector{}

		// when
		err := r.RewriteState(context.Background(), st, c.report)

		// then
		require.NoError(t, err)
		assert.Equal(t, "idA", st.Pick("l1").Model().GetLink().TargetBlockId)
		assert.Equal(t, addr.MissingObject, st.Pick("l2").Model().GetLink().TargetBlockId)
		assert.Equal(t, []importv2.IssueCode{importv2.IssueMissingTarget}, c.codes())
	})

	t.Run("bundled and date targets pass through untouched", func(t *testing.T) {
		// given
		bundledType := bundle.TypeKeyPage.BundledURL()
		dateId := addr.DatePrefix + "2026-07-02"
		st := docWithBlocks(
			&model.Block{Id: "l1", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: bundledType}}},
			&model.Block{Id: "l2", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: dateId}}},
		)
		r := newResolver(&fakeRefs{}, fakeKeys{})
		c := &issueCollector{}

		// when
		err := r.RewriteState(context.Background(), st, c.report)

		// then
		require.NoError(t, err)
		assert.Equal(t, bundledType, st.Pick("l1").Model().GetLink().TargetBlockId)
		assert.Equal(t, dateId, st.Pick("l2").Model().GetLink().TargetBlockId)
		assert.Empty(t, c.issues)
	})

	t.Run("marks after a bundled mention are still resolved (v1 return-bug regression)", func(t *testing.T) {
		// given
		st := docWithBlocks(&model.Block{Id: "t1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text: "a b c",
			Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{Type: model.BlockContentTextMark_Mention, Param: bundle.TypeKeyTask.BundledURL(), Range: &model.Range{From: 0, To: 1}},
				{Type: model.BlockContentTextMark_Mention, Param: "docs/b.md", Range: &model.Range{From: 2, To: 3}},
				{Type: model.BlockContentTextMark_Bold, Range: &model.Range{From: 4, To: 5}},
			}},
		}}})
		r := newResolver(&fakeRefs{ids: map[string]string{"docs/b.md": "idB"}}, fakeKeys{})
		c := &issueCollector{}

		// when
		err := r.RewriteState(context.Background(), st, c.report)

		// then
		require.NoError(t, err)
		marks := st.Pick("t1").Model().GetText().GetMarks().GetMarks()
		assert.Equal(t, bundle.TypeKeyTask.BundledURL(), marks[0].Param)
		assert.Equal(t, "idB", marks[1].Param)
		assert.Empty(t, c.issues)
	})

	t.Run("file block waits for the file future", func(t *testing.T) {
		// given
		blocked := make(chan string, 1)
		st := docWithBlocks(&model.Block{Id: "f1", Content: &model.BlockContentOfFile{File: &model.BlockContentFile{TargetObjectId: "docs/img.png"}}})
		r := newResolver(&fakeRefs{blocked: map[string]chan string{"docs/img.png": blocked}}, fakeKeys{})
		done := make(chan error, 1)
		go func() {
			done <- r.RewriteState(context.Background(), st, (&issueCollector{}).report)
		}()

		// when
		blocked <- "fileObjId"

		// then
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("rewrite did not finish after future resolution")
		}
		assert.Equal(t, "fileObjId", st.Pick("f1").Model().GetFile().TargetObjectId)
	})

	t.Run("failed file reference degrades to missing marker with issue", func(t *testing.T) {
		// given
		st := docWithBlocks(&model.Block{Id: "f1", Content: &model.BlockContentOfFile{File: &model.BlockContentFile{TargetObjectId: "docs/img.png"}}})
		r := newResolver(&fakeRefs{failed: map[string]error{"docs/img.png": assert.AnError}}, fakeKeys{})
		c := &issueCollector{}

		// when
		err := r.RewriteState(context.Background(), st, c.report)

		// then
		require.NoError(t, err)
		assert.Equal(t, addr.MissingObject, st.Pick("f1").Model().GetFile().TargetObjectId)
		assert.Equal(t, []importv2.IssueCode{importv2.IssueMissingTarget}, c.codes())
	})

	t.Run("cancellation aborts the rewrite with an error", func(t *testing.T) {
		// given
		ctx, cancel := context.WithCancel(context.Background())
		st := docWithBlocks(&model.Block{Id: "f1", Content: &model.BlockContentOfFile{File: &model.BlockContentFile{TargetObjectId: "docs/img.png"}}})
		r := newResolver(&fakeRefs{blocked: map[string]chan string{"docs/img.png": make(chan string)}}, fakeKeys{})

		// when
		cancel()
		err := r.RewriteState(ctx, st, (&issueCollector{}).report)

		// then
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestRewriteDetails(t *testing.T) {
	t.Run("object-format detail values remap; scalar leniency preserved", func(t *testing.T) {
		// given
		st := docWithBlocks()
		st.SetDetail(bundle.RelationKeyCoverId, domain.String("yellow"))
		st.SetDetail("linkedPages", domain.StringList([]string{"docs/a.md", "unknown-value"}))
		st.AddRelationLinks(&model.RelationLink{Key: "linkedPages", Format: model.RelationFormat_object})
		r := newResolver(&fakeRefs{ids: map[string]string{"docs/a.md": "idA"}}, fakeKeys{})

		// when
		err := r.RewriteState(context.Background(), st, (&issueCollector{}).report)

		// then
		require.NoError(t, err)
		assert.Equal(t, "yellow", st.Details().GetString(bundle.RelationKeyCoverId))
		assert.Equal(t, []string{"idA", "unknown-value"}, st.Details().GetStringList("linkedPages"))
	})

	t.Run("tag values resolve to option ids", func(t *testing.T) {
		// given
		st := docWithBlocks()
		st.SetDetail(bundle.RelationKeyTag, domain.StringList([]string{"opt:tag:urgent"}))
		r := newResolver(&fakeRefs{ids: map[string]string{"opt:tag:urgent": "optionId1"}}, fakeKeys{})

		// when
		err := r.RewriteState(context.Background(), st, (&issueCollector{}).report)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"optionId1"}, st.Details().GetStringList(bundle.RelationKeyTag))
	})
}

func TestAdoptKeys(t *testing.T) {
	t.Run("adopted relation key moves detail, link and type key", func(t *testing.T) {
		// given
		st := docWithBlocks()
		st.SetDetail("myrel", domain.String("v"))
		st.AddRelationLinks(&model.RelationLink{Key: "myrel", Format: model.RelationFormat_longtext})
		st.SetObjectTypeKey("mytype")
		r := newResolver(&fakeRefs{}, fakeKeys{"myrel": "existingkey", "mytype": "existingtype"})

		// when
		err := r.RewriteState(context.Background(), st, (&issueCollector{}).report)

		// then
		require.NoError(t, err)
		assert.Equal(t, "v", st.Details().GetString("existingkey"))
		_, hasOld := st.Details().TryGet("myrel")
		assert.False(t, hasOld)
		assert.True(t, st.PickRelationLinks().Has("existingkey"))
		assert.False(t, st.PickRelationLinks().Has("myrel"))
		assert.Equal(t, domain.TypeKey("existingtype"), st.ObjectTypeKey())
	})
}

func TestRewriteCollectionStore(t *testing.T) {
	t.Run("membership remaps, unresolved members dropped with issue", func(t *testing.T) {
		// given
		st := docWithBlocks()
		st.UpdateStoreSlice(template.CollectionStoreKey, []string{"docs/a.md", "docs/gone.md"})
		r := newResolver(&fakeRefs{ids: map[string]string{"docs/a.md": "idA"}}, fakeKeys{})
		c := &issueCollector{}

		// when
		err := r.RewriteState(context.Background(), st, c.report)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"idA"}, st.GetStoreSlice(template.CollectionStoreKey))
		assert.Equal(t, []importv2.IssueCode{importv2.IssueMissingTarget}, c.codes())
	})
}
