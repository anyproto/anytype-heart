package text

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestText_Diff(t *testing.T) {
	testBlock := func() *Text {
		return NewText(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfText{Text: &model.BlockContentText{}},
		}).(*Text)
	}
	t.Run("type error", func(t *testing.T) {
		b1 := testBlock()
		b2 := base.NewBase(&model.Block{})
		_, err := b1.Diff("", b2)
		assert.Error(t, err)
	})
	t.Run("no diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b1.SetText("same text", &model.BlockContentTextMarks{})
		b2.SetText("same text", &model.BlockContentTextMarks{})
		d, err := b1.Diff("", b2)
		require.NoError(t, err)
		assert.Len(t, d, 0)
	})
	t.Run("base diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.Restrictions.Read = true
		d, err := b1.Diff("", b2)
		require.NoError(t, err)
		assert.Len(t, d, 1)
	})
	t.Run("content diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.SetText("text", &model.BlockContentTextMarks{
			Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{1, 2},
					Type:  model.BlockContentTextMark_Italic,
				},
			},
		})
		b2.SetStyle(model.BlockContentText_Header2)
		b2.SetChecked(true)
		diff, err := b1.Diff("", b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		textChange := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetText).BlockSetText
		assert.NotNil(t, textChange.Style)
		assert.NotNil(t, textChange.Checked)
		assert.NotNil(t, textChange.Text)
		assert.NotNil(t, textChange.Marks)
	})
}

func TestText_Split(t *testing.T) {
	testBlock := func() *Text {
		return NewText(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text: "1234567890",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Type: model.BlockContentTextMark_Bold,
							Range: &model.Range{
								From: 0,
								To:   10,
							},
						},
						{
							Type: model.BlockContentTextMark_Italic,
							Range: &model.Range{
								From: 6,
								To:   10,
							},
						},
						{
							Type: model.BlockContentTextMark_BackgroundColor,
							Range: &model.Range{
								From: 3,
								To:   4,
							},
						},
					},
				},
			}},
		}).(*Text)
	}
	t.Run("should split block", func(t *testing.T) {
		b := testBlock()
		newBlock, err := b.Split(5)
		require.NoError(t, err)
		nb := newBlock.(*Text)
		assert.Equal(t, "12345", nb.content.Text)
		assert.Equal(t, "67890", b.content.Text)
		require.Len(t, b.content.Marks.Marks, 2)
		require.Len(t, nb.content.Marks.Marks, 2)
		assert.Equal(t, model.Range{0, 5}, *nb.content.Marks.Marks[0].Range)
		assert.Equal(t, model.Range{3, 4}, *nb.content.Marks.Marks[1].Range)
		assert.Equal(t, model.Range{0, 5}, *b.content.Marks.Marks[0].Range)
		assert.Equal(t, model.Range{1, 5}, *b.content.Marks.Marks[1].Range)
	})
	t.Run("out of range", func(t *testing.T) {
		b := testBlock()
		_, err := b.Split(11)
		require.Equal(t, ErrOutOfRange, err)
	})
	t.Run("start pos", func(t *testing.T) {
		b := testBlock()
		_, err := b.Split(0)
		require.NoError(t, err)
	})
	t.Run("end pos", func(t *testing.T) {
		b := testBlock()
		_, err := b.Split(10)
		require.NoError(t, err)
	})
}

func TestText_RangeSplit(t *testing.T) {
	testBlock := func() *Text {
		return NewText(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text: "1234567890",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Type: model.BlockContentTextMark_Bold,
							Range: &model.Range{
								From: 2,
								To:   8,
							},
						},
					},
				},
			}},
		}).(*Text)
	}
	t.Run("should split block", func(t *testing.T) {
		b := testBlock()
		newBlock, err := b.RangeSplit(1, 5, false)
		require.NoError(t, err)
		nb := newBlock.(*Text)
		assert.Equal(t, "67890", nb.content.Text)
		assert.Equal(t, "1", b.content.Text)
		require.Len(t, b.content.Marks.Marks, 0)
		require.Len(t, nb.content.Marks.Marks, 1)
		assert.Equal(t, model.Range{0, 3}, *nb.content.Marks.Marks[0].Range)
	})
	t.Run("split by range with marked and unmarked letters at the start of marked zone", func(t *testing.T) {
		b := testBlock()
		newBlock, err := b.RangeSplit(1, 3, false)
		require.NoError(t, err)
		nb := newBlock.(*Text)
		assert.Equal(t, "4567890", nb.content.Text)
		assert.Equal(t, "1", b.content.Text)
		require.Len(t, b.content.Marks.Marks, 0)
		require.Len(t, nb.content.Marks.Marks, 1)
		assert.Equal(t, model.Range{0, 5}, *nb.content.Marks.Marks[0].Range)
	})
	t.Run("split by range with marked letters at the start of marked zone", func(t *testing.T) {
		b := testBlock()
		newBlock, err := b.RangeSplit(2, 3, false)
		require.NoError(t, err)
		nb := newBlock.(*Text)
		assert.Equal(t, "4567890", nb.content.Text)
		assert.Equal(t, "12", b.content.Text)
		require.Len(t, b.content.Marks.Marks, 0)
		require.Len(t, nb.content.Marks.Marks, 1)
		assert.Equal(t, model.Range{0, 5}, *nb.content.Marks.Marks[0].Range)
	})
	t.Run("split by range with marked and unmarked letters at the end of marked zone", func(t *testing.T) {
		b := testBlock()
		newBlock, err := b.RangeSplit(7, 8, false)
		require.NoError(t, err)
		nb := newBlock.(*Text)
		assert.Equal(t, "90", nb.content.Text)
		assert.Equal(t, "1234567", b.content.Text)
		require.Len(t, b.content.Marks.Marks, 1)
		assert.Equal(t, model.Range{2, 7}, *b.content.Marks.Marks[0].Range)
	})
	t.Run("out of range", func(t *testing.T) {
		b := testBlock()
		_, err := b.RangeSplit(0, 11, false)
		require.Equal(t, ErrOutOfRange, err)
	})
	t.Run("checked checkbox split at position 0 bottom preserves checked state", func(t *testing.T) {
		b := NewText(&model.Block{
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text:    "checkbox text",
				Style:   model.BlockContentText_Checkbox,
				Checked: true,
			}},
		}).(*Text)
		newBlock, err := b.RangeSplit(0, 0, false)
		require.NoError(t, err)
		nb := newBlock.(*Text)
		assert.Equal(t, "checkbox text", nb.content.Text)
		assert.True(t, nb.content.Checked, "new block should preserve checked state")
		assert.Equal(t, "", b.content.Text)
		assert.True(t, b.content.Checked, "original block should preserve checked state")
	})
	t.Run("checked checkbox split at position 0 top preserves checked state", func(t *testing.T) {
		b := NewText(&model.Block{
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text:    "checkbox text",
				Style:   model.BlockContentText_Checkbox,
				Checked: true,
			}},
		}).(*Text)
		newBlock, err := b.RangeSplit(0, 0, true)
		require.NoError(t, err)
		nb := newBlock.(*Text)
		assert.Equal(t, "", nb.content.Text)
		assert.True(t, nb.content.Checked, "new top block should preserve checked state")
		assert.Equal(t, "checkbox text", b.content.Text)
		assert.True(t, b.content.Checked, "original block should preserve checked state")
	})
	t.Run("checked checkbox mid-text split preserves checked state", func(t *testing.T) {
		b := NewText(&model.Block{
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text:    "checkbox text",
				Style:   model.BlockContentText_Checkbox,
				Checked: true,
			}},
		}).(*Text)
		newBlock, err := b.RangeSplit(8, 8, false)
		require.NoError(t, err)
		nb := newBlock.(*Text)
		assert.Equal(t, " text", nb.content.Text)
		assert.True(t, nb.content.Checked, "new block should preserve checked state")
		assert.Equal(t, "checkbox", b.content.Text)
		assert.True(t, b.content.Checked, "original block should preserve checked state")
	})
	t.Run("unchecked checkbox split does not set checked", func(t *testing.T) {
		b := NewText(&model.Block{
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text:    "unchecked",
				Style:   model.BlockContentText_Checkbox,
				Checked: false,
			}},
		}).(*Text)
		newBlock, err := b.RangeSplit(0, 0, false)
		require.NoError(t, err)
		nb := newBlock.(*Text)
		assert.False(t, nb.content.Checked)
		assert.False(t, b.content.Checked)
	})
}

func TestText_Merge(t *testing.T) {
	testBlock := func() *Text {
		return NewText(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text: "1234567890",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Type: model.BlockContentTextMark_Bold,
							Range: &model.Range{
								From: 0,
								To:   5,
							},
						},
						{
							Type: model.BlockContentTextMark_Bold,
							Range: &model.Range{
								From: 5,
								To:   10,
							},
						},
						{
							Type: model.BlockContentTextMark_BackgroundColor,
							Range: &model.Range{
								From: 3,
								To:   4,
							},
						},
					},
				},
			}},
		}).(*Text)
	}

	t.Run("should merge two blocks", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		err := b1.Merge(b2)
		require.NoError(t, err)
		assert.Equal(t, "12345678901234567890", b1.content.Text)

		require.Len(t, b1.content.Marks.Marks, 3)
		assert.Equal(t, model.Range{From: 0, To: 20}, *b1.content.Marks.Marks[0].Range)
		assert.Equal(t, model.Range{From: 3, To: 4}, *b1.content.Marks.Marks[1].Range)
		assert.Equal(t, model.Range{From: 13, To: 14}, *b1.content.Marks.Marks[2].Range)
	})

	t.Run("merge styled blocks", func(t *testing.T) {
		// given
		mergeTo := NewText(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "",
				},
			},
		}).(*Text)

		mergeFrom := NewText(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "One",
					Style: model.BlockContentText_Header1,
				},
			},
		}).(*Text)
		mergeFrom.BackgroundColor = "grey"

		// when
		err := mergeTo.Merge(mergeFrom)

		// then
		require.NoError(t, err)
		assert.Equal(t, mergeTo.content.Style, model.BlockContentText_Header1)
		assert.Equal(t, mergeTo.BackgroundColor, "grey")
	})

	t.Run("preserve style of block with text", func(t *testing.T) {
		// given
		mergeTo := NewText(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "Two",
				},
			},
		}).(*Text)

		mergeFrom := NewText(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "One",
					Style: model.BlockContentText_Header1,
				},
			},
		}).(*Text)
		mergeFrom.BackgroundColor = "grey"

		// when
		err := mergeTo.Merge(mergeFrom)

		// then
		require.NoError(t, err)
		assert.Equal(t, model.BlockContentText_Paragraph, mergeTo.content.Style)
		assert.Empty(t, mergeTo.BackgroundColor)
		assert.Equal(t, "TwoOne", mergeTo.content.Text)
	})

	t.Run("preserve style != 0", func(t *testing.T) {
		// given
		mergeTo := NewText(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "",
					Style: model.BlockContentText_Numbered,
				},
			},
		}).(*Text)

		mergeFrom := NewText(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "One",
					Style: model.BlockContentText_Header1,
				},
			},
		}).(*Text)
		mergeFrom.BackgroundColor = "grey"

		// when
		err := mergeTo.Merge(mergeFrom)

		// then
		require.NoError(t, err)
		assert.Equal(t, model.BlockContentText_Numbered, mergeTo.content.Style)
		assert.Empty(t, mergeTo.BackgroundColor)
		assert.Equal(t, "One", mergeTo.content.Text)
	})
}

func TestText_SetMarkForAllText(t *testing.T) {
	b := NewText(&model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "1234567890",
			},
		},
	})
	tb := b.(Block)
	tb.SetMarkForAllText(&model.BlockContentTextMark{
		Type: model.BlockContentTextMark_Bold,
	})
	require.Len(t, tb.Model().GetText().Marks.Marks, 1)
	assert.Equal(t, &model.BlockContentTextMark{
		Type:  model.BlockContentTextMark_Bold,
		Range: &model.Range{From: 0, To: 10},
	}, tb.Model().GetText().Marks.Marks[0])
	tb.SetMarkForAllText(&model.BlockContentTextMark{
		Type: model.BlockContentTextMark_Italic,
	})
	require.Len(t, tb.Model().GetText().Marks.Marks, 2)
	assert.Equal(t, &model.BlockContentTextMark{
		Type:  model.BlockContentTextMark_Italic,
		Range: &model.Range{From: 0, To: 10},
	}, tb.Model().GetText().Marks.Marks[1])
	tb.SetMarkForAllText(&model.BlockContentTextMark{
		Type: model.BlockContentTextMark_Bold,
	})
	assert.Len(t, tb.Model().GetText().Marks.Marks, 2)
}

func TestText_IncompatibleTypes(t *testing.T) {
	b := NewText(&model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "1234567890",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						&model.BlockContentTextMark{
							Range: &model.Range{
								From: 0,
								To:   10,
							},
							Type:  model.BlockContentTextMark_Link,
							Param: "https://www.youtube.com/",
						},
					},
				},
			},
		},
	})
	tb := b.(Block)
	tb.SetMarkForAllText(&model.BlockContentTextMark{
		Type: model.BlockContentTextMark_Object,
		Range: &model.Range{
			From: 0,
			To:   10,
		},
		Param: "bafyba2frjwd6jnmisz7ejfld2yg3qe7z6g2f5r5dkwkfknkvs2xyuatc",
	})
	assert.Len(t, tb.Model().GetText().Marks.Marks, 1)

	b = NewText(&model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "1234567890",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						&model.BlockContentTextMark{
							Range: &model.Range{
								From: 0,
								To:   10,
							},
							Type:  model.BlockContentTextMark_Object,
							Param: "bafyba2frjwd6jnmisz7ejfld2yg3qe7z6g2f5r5dkwkfknkvs2xyuatc",
						},
					},
				},
			},
		},
	})
	tb = b.(Block)
	tb.SetMarkForAllText(&model.BlockContentTextMark{
		Type: model.BlockContentTextMark_Object,
		Range: &model.Range{
			From: 0,
			To:   10,
		},
		Param: "https://www.youtube.com/",
	})
	assert.Len(t, tb.Model().GetText().Marks.Marks, 1)
}

func TestText_RemoveMarkType(t *testing.T) {
	b := NewText(&model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "1234567890",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{Type: model.BlockContentTextMark_Bold, Range: &model.Range{To: 10}},
						{Type: model.BlockContentTextMark_Italic, Range: &model.Range{To: 5}},
					},
				},
			},
		},
	}).(Block)
	b.RemoveMarkType(model.BlockContentTextMark_Bold)
	assert.Len(t, b.Model().GetText().Marks.Marks, 1)
	assert.Equal(t, model.BlockContentTextMark_Italic, b.Model().GetText().Marks.Marks[0].Type)
}

func TestText_HasMarkForAllText(t *testing.T) {
	b := NewText(&model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "1234567890",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{Type: model.BlockContentTextMark_Bold, Range: &model.Range{To: 10}},
						{Type: model.BlockContentTextMark_Italic, Range: &model.Range{To: 5}},
					},
				},
			},
		},
	}).(Block)
	assert.False(t, b.HasMarkForAllText(&model.BlockContentTextMark{Type: model.BlockContentTextMark_Italic}))
	assert.True(t, b.HasMarkForAllText(&model.BlockContentTextMark{Type: model.BlockContentTextMark_Bold}))
}

func TestText_MigrateFile(t *testing.T) {
	fileId := "bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku"
	fileIdMigrated := "bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku-migrated"
	migrator := func(id string) string {
		if domain.IsFileId(id) {
			return fileId + "-migrated"
		}
		return id
	}

	got := NewText(&model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:      "1234567890",
				IconImage: fileId,
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{Type: model.BlockContentTextMark_Mention, Param: fileId},
						{Type: model.BlockContentTextMark_Mention, Param: "object1"},
						{Type: model.BlockContentTextMark_Object, Param: fileId},
						{Type: model.BlockContentTextMark_Object, Param: "object2"},
					},
				},
			},
		},
	}).(Block)

	got.MigrateFile(migrator)

	want := NewText(&model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:      "1234567890",
				IconImage: fileIdMigrated,
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{Type: model.BlockContentTextMark_Mention, Param: fileIdMigrated},
						{Type: model.BlockContentTextMark_Mention, Param: "object1"},
						{Type: model.BlockContentTextMark_Object, Param: fileIdMigrated},
						{Type: model.BlockContentTextMark_Object, Param: "object2"},
					},
				},
			},
		},
	}).(Block)

	assert.Equal(t, want, got)
}

// RangeTextPaste adopts the pasted block's style when the paste replaces all of the target's
// text (or fills an empty paragraph) and the caller asked for the style to be copied. Checked
// rides along with the checkbox style, but only into an empty paragraph: there it has no state
// of its own to destroy. Replacing the text of an existing block never moves its checked state,
// so retitling a finished task cannot quietly reopen it.
func TestText_RangeTextPasteChecked(t *testing.T) {
	target := func(text string, style model.BlockContentTextStyle, checked bool) *Text {
		return NewText(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text: text, Style: style, Checked: checked,
				Marks: &model.BlockContentTextMarks{},
			}},
		}).(*Text)
	}
	pasted := func(text string, style model.BlockContentTextStyle, checked bool) *model.Block {
		return &model.Block{Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text: text, Style: style, Checked: checked,
			Marks: &model.BlockContentTextMarks{},
		}}}
	}

	for _, tc := range []struct {
		name        string
		target      *Text
		from, to    int32
		copied      *model.Block
		copyStyle   bool
		wantChecked bool
		wantText    string // the label must survive; asserting Checked alone lets it vanish
	}{
		{
			name:   "empty paragraph adopts a checked checkbox",
			target: target("", model.BlockContentText_Paragraph, false),
			from:   0, to: 0,
			copied:      pasted("new", model.BlockContentText_Checkbox, true),
			copyStyle:   true,
			wantChecked: true,
			wantText:    "new",
		},
		{
			// Deliberate, and a change from the old behaviour: the leftover state is
			// invisible (the block renders as a plain paragraph) while the pasted "- [ ]"
			// is explicit, so honouring the paste is right. Preserving the residue here
			// means pasting an unchecked task produces a checked one, which is what the
			// old code did. Do not "restore" that.
			name:   "empty paragraph adopts an unchecked checkbox over leftover state",
			target: target("", model.BlockContentText_Paragraph, true),
			from:   0, to: 0,
			copied:      pasted("new", model.BlockContentText_Checkbox, false),
			copyStyle:   true,
			wantChecked: false,
			wantText:    "new",
		},
		{
			name:   "full replace does not set checked on the target",
			target: target("old", model.BlockContentText_Checkbox, false),
			from:   0, to: 3,
			copied:      pasted("new", model.BlockContentText_Checkbox, true),
			copyStyle:   true,
			wantChecked: false,
			wantText:    "new",
		},
		{
			name:   "full replace does not clear checked on the target",
			target: target("old", model.BlockContentText_Checkbox, true),
			from:   0, to: 3,
			copied:      pasted("new", model.BlockContentText_Checkbox, false),
			copyStyle:   true,
			wantChecked: true,
			wantText:    "new",
		},
		{
			name:   "copyStyle false does not clear checked",
			target: target("", model.BlockContentText_Paragraph, true),
			from:   0, to: 0,
			copied:      pasted("new", model.BlockContentText_Checkbox, false),
			copyStyle:   false,
			wantChecked: true,
			wantText:    "new",
		},
		{
			name:   "copyStyle false does not set checked either",
			target: target("", model.BlockContentText_Paragraph, false),
			from:   0, to: 0,
			copied:      pasted("new", model.BlockContentText_Checkbox, true),
			copyStyle:   false,
			wantChecked: false,
			wantText:    "new",
		},
		{
			// an empty checkbox has a style of its own, so it is not adopting one and
			// must keep its own state whatever arrives
			name:   "empty checkbox target does not take the pasted checked state",
			target: target("", model.BlockContentText_Checkbox, false),
			from:   0, to: 0,
			copied:      pasted("new", model.BlockContentText_Checkbox, true),
			copyStyle:   true,
			wantChecked: false,
			wantText:    "new",
		},
		{
			name:   "empty checkbox target does not lose its checked state either",
			target: target("", model.BlockContentText_Checkbox, true),
			from:   0, to: 0,
			copied:      pasted("new", model.BlockContentText_Checkbox, false),
			copyStyle:   true,
			wantChecked: true,
			wantText:    "new",
		},
		{
			name:   "partial replacement does not clear checked",
			target: target("old", model.BlockContentText_Checkbox, true),
			from:   0, to: 1,
			copied:      pasted("n", model.BlockContentText_Checkbox, false),
			copyStyle:   true,
			wantChecked: true,
			wantText:    "nld",
		},
		{
			name:   "partial replacement does not set checked either",
			target: target("old", model.BlockContentText_Checkbox, false),
			from:   1, to: 2,
			copied:      pasted("n", model.BlockContentText_Checkbox, true),
			copyStyle:   true,
			wantChecked: false,
			wantText:    "ond",
		},
		{
			name:   "filling an empty paragraph with a non-checkbox does not clear checked",
			target: target("", model.BlockContentText_Paragraph, true),
			from:   0, to: 0,
			copied:      pasted("new", model.BlockContentText_Paragraph, false),
			copyStyle:   true,
			wantChecked: true,
			wantText:    "new",
		},
		{
			// a paragraph carrying checked residue is not a checkbox: pasting it in must
			// not turn the target into one that is silently complete
			name:   "filling an empty paragraph with a non-checkbox does not set checked",
			target: target("", model.BlockContentText_Paragraph, false),
			from:   0, to: 0,
			copied:      pasted("new", model.BlockContentText_Paragraph, true),
			copyStyle:   true,
			wantChecked: false,
			wantText:    "new",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// when
			_, err := tc.target.RangeTextPaste(tc.from, tc.to, tc.copied, tc.copyStyle)

			// then
			require.NoError(t, err)
			assert.Equal(t, tc.wantChecked, tc.target.content.Checked)
			assert.Equal(t, tc.wantText, tc.target.content.Text, "the label must survive")
		})
	}
}

// RangeTextPaste adopts the pasted block's presentation along with its style. GO-7513 covered
// the two colors and the checked state; the icon and the code block's language were still left
// behind, so a callout or a fenced block pasted on its own — a body of one block takes this
// route, a longer one does not — arrived stripped of what a pasted pair keeps.
//
// Fields is shared storage: textDetails keeps a block's detail binding there, so only the key
// owned by the style being adopted is ever written, never the struct as a whole. Every field
// is exercised in both directions, because adopting a value and adopting its absence are two
// different code paths and only one of them is obvious.
func TestText_RangeTextPasteStyleFields(t *testing.T) {
	type want struct {
		style     model.BlockContentTextStyle
		iconEmoji string
		iconImage string
		lang      string
		text      string // the text must land too: asserting a field alone lets the paste vanish
	}
	withLang := func(b *model.Block, lang string) *model.Block {
		b.Fields = &types.Struct{Fields: map[string]*types.Value{CodeLangFieldName: pbtypes.String(lang)}}
		return b
	}
	target := func(text string, style model.BlockContentTextStyle) *model.Block {
		return &model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text: text, Style: style, Marks: &model.BlockContentTextMarks{},
			}},
		}
	}
	pasted := func(text string, style model.BlockContentTextStyle) *model.Block {
		return &model.Block{Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text: text, Style: style, Marks: &model.BlockContentTextMarks{},
		}}}
	}

	for _, tc := range []struct {
		name      string
		target    *model.Block
		from, to  int32
		copied    *model.Block
		copyStyle bool
		want      want
	}{
		{
			name:   "empty paragraph adopts the callout icon",
			target: target("", model.BlockContentText_Paragraph),
			from:   0, to: 0,
			copied: func() *model.Block {
				b := pasted("note", model.BlockContentText_Callout)
				b.GetText().IconEmoji = "\U0001f4a1"
				return b
			}(),
			copyStyle: true,
			want:      want{style: model.BlockContentText_Callout, iconEmoji: "\U0001f4a1", text: "note"},
		},
		{
			name:   "empty paragraph adopts the callout icon image",
			target: target("", model.BlockContentText_Paragraph),
			from:   0, to: 0,
			copied: func() *model.Block {
				b := pasted("note", model.BlockContentText_Callout)
				b.GetText().IconImage = "imagehash"
				return b
			}(),
			copyStyle: true,
			want:      want{style: model.BlockContentText_Callout, iconImage: "imagehash", text: "note"},
		},
		{
			// A plain paragraph states nothing whatsoever about icons, so its empty icon
			// is not a value to adopt — taking it would destroy state the paste never
			// mentioned. This is where the analogy to the checked state below runs out:
			// that guard requires the pasted style to be Checkbox, the style that owns
			// the field, so a pasted "- [ ]" really is an explicit statement.
			name: "a paste that says nothing about icons leaves the block's own icon alone",
			target: func() *model.Block {
				b := target("", model.BlockContentText_Paragraph)
				b.GetText().IconEmoji = "\U0001f525"
				b.GetText().IconImage = "oldhash"
				return b
			}(),
			from: 0, to: 0,
			copied:    pasted("plain", model.BlockContentText_Paragraph),
			copyStyle: true,
			want: want{
				style: model.BlockContentText_Paragraph, iconEmoji: "\U0001f525",
				iconImage: "oldhash", text: "plain",
			},
		},
		{
			// the residue does go when the style that owned it is replaced, which is the
			// moment it actually becomes stale
			name: "a style change clears the icon residue",
			target: func() *model.Block {
				b := target("", model.BlockContentText_Paragraph)
				b.GetText().IconEmoji = "\U0001f525"
				b.GetText().IconImage = "oldhash"
				return b
			}(),
			from: 0, to: 0,
			copied:    pasted("note", model.BlockContentText_Callout),
			copyStyle: true,
			want:      want{style: model.BlockContentText_Callout, text: "note"},
		},
		{
			name:   "replacing all the text adopts the callout icon",
			target: target("old", model.BlockContentText_Paragraph),
			from:   0, to: 3,
			copied: func() *model.Block {
				b := pasted("note", model.BlockContentText_Callout)
				b.GetText().IconEmoji = "\U0001f4a1"
				return b
			}(),
			copyStyle: true,
			want:      want{style: model.BlockContentText_Callout, iconEmoji: "\U0001f4a1", text: "note"},
		},
		{
			// the icon goes with the style it belonged to, exactly like the two colors
			// beside it, rather than lingering on a block that no longer renders one
			name: "replacing all the text of a callout clears the icon with the style",
			target: func() *model.Block {
				b := target("old", model.BlockContentText_Callout)
				b.GetText().IconEmoji = "\U0001f4a1"
				return b
			}(),
			from: 0, to: 3,
			copied:    pasted("plain", model.BlockContentText_Paragraph),
			copyStyle: true,
			want:      want{style: model.BlockContentText_Paragraph, text: "plain"},
		},
		{
			// The regression guard. pasteHtml hands an incoming plain paragraph the
			// focused block's style and nothing else (GO-250), so retitling a callout
			// through the HTML slot presents an icon-less Callout. Adopting that
			// emptiness wiped an icon the paste never mentioned.
			name: "replacing all the text of a callout keeps the block's own icon",
			target: func() *model.Block {
				b := target("old", model.BlockContentText_Callout)
				b.GetText().IconEmoji = "\U0001f4a1"
				b.GetText().IconImage = "imagehash"
				return b
			}(),
			from: 0, to: 3,
			copied:    pasted("New note", model.BlockContentText_Callout),
			copyStyle: true,
			want: want{
				style: model.BlockContentText_Callout, iconEmoji: "\U0001f4a1",
				iconImage: "imagehash", text: "New note",
			},
		},
		{
			// the same rule seen from the other side: where the style does not change,
			// the icon on the block is the block's own and the paste does not touch it
			name: "replacing all the text of a callout does not take another callout's icon",
			target: func() *model.Block {
				b := target("old", model.BlockContentText_Callout)
				b.GetText().IconEmoji = "\U0001f4a1"
				return b
			}(),
			from: 0, to: 3,
			copied: func() *model.Block {
				b := pasted("note", model.BlockContentText_Callout)
				b.GetText().IconEmoji = "\U0001f525"
				return b
			}(),
			copyStyle: true,
			want:      want{style: model.BlockContentText_Callout, iconEmoji: "\U0001f4a1", text: "note"},
		},
		{
			// the image twin of the case above: an icon guard that special-cases the
			// image field passes every emoji-only fixture
			name: "replacing all the text of a callout does not take another callout's image",
			target: func() *model.Block {
				b := target("old", model.BlockContentText_Callout)
				b.GetText().IconImage = "imageA"
				return b
			}(),
			from: 0, to: 3,
			copied: func() *model.Block {
				b := pasted("note", model.BlockContentText_Callout)
				b.GetText().IconImage = "imageB"
				return b
			}(),
			copyStyle: true,
			want:      want{style: model.BlockContentText_Callout, iconImage: "imageA", text: "note"},
		},
		{
			// adoption of the image is not restricted to an empty paragraph: the style
			// changes here over text that was already there
			name:   "replacing all the text adopts the callout image",
			target: target("old", model.BlockContentText_Paragraph),
			from:   0, to: 3,
			copied: func() *model.Block {
				b := pasted("note", model.BlockContentText_Callout)
				b.GetText().IconImage = "imageB"
				return b
			}(),
			copyStyle: true,
			want:      want{style: model.BlockContentText_Callout, iconImage: "imageB", text: "note"},
		},
		{
			// an empty callout is not an empty paragraph: it has a style of its own, so
			// the guard does not fire and its icon is left alone
			name: "filling an empty callout keeps its icon",
			target: func() *model.Block {
				b := target("", model.BlockContentText_Callout)
				b.GetText().IconEmoji = "\U0001f4a1"
				return b
			}(),
			from: 0, to: 0,
			copied:    pasted("New note", model.BlockContentText_Callout),
			copyStyle: true,
			want:      want{style: model.BlockContentText_Callout, iconEmoji: "\U0001f4a1", text: "New note"},
		},
		{
			name:   "empty paragraph adopts the code language",
			target: target("", model.BlockContentText_Paragraph),
			from:   0, to: 0,
			copied:    withLang(pasted("fmt.Println(1)", model.BlockContentText_Code), "go"),
			copyStyle: true,
			want:      want{style: model.BlockContentText_Code, lang: "go", text: "fmt.Println(1)"},
		},
		{
			name:   "replacing all the text adopts the code language",
			target: target("old", model.BlockContentText_Paragraph),
			from:   0, to: 3,
			copied:    withLang(pasted("fmt.Println(1)", model.BlockContentText_Code), "go"),
			copyStyle: true,
			want:      want{style: model.BlockContentText_Code, lang: "go", text: "fmt.Println(1)"},
		},
		{
			// a fence with no language after one that had it must not keep the old one
			name: "adopting the code style with no language clears the language residue",
			target: func() *model.Block {
				return withLang(target("", model.BlockContentText_Paragraph), "go")
			}(),
			from: 0, to: 0,
			copied:    pasted("plain code", model.BlockContentText_Code),
			copyStyle: true,
			want:      want{style: model.BlockContentText_Code, text: "plain code"},
		},
		{
			// Deliberate asymmetry with the icon above. The language is invisible on a
			// block that is not code and clearing it buys nothing, while it would destroy
			// a real one on the intoCodeBlock path, where a select-all paste already
			// rewrites the style of the code block it lands in.
			name: "adopting a style that owns no fields leaves the language alone",
			target: func() *model.Block {
				return withLang(target("", model.BlockContentText_Paragraph), "go")
			}(),
			from: 0, to: 0,
			copied:    pasted("plain", model.BlockContentText_Paragraph),
			copyStyle: true,
			want:      want{style: model.BlockContentText_Paragraph, lang: "go", text: "plain"},
		},
		{
			// only the key owned by the style being adopted is written, so a language
			// riding along on a paragraph is not applied to anything
			name:   "adopting a non-Code style does not apply the source's language",
			target: target("", model.BlockContentText_Paragraph),
			from:   0, to: 0,
			copied:    withLang(pasted("plain", model.BlockContentText_Paragraph), "rust"),
			copyStyle: true,
			want:      want{style: model.BlockContentText_Paragraph, text: "plain"},
		},
		{
			name: "adopting a non-Code style does not overwrite the language residue either",
			target: func() *model.Block {
				return withLang(target("", model.BlockContentText_Paragraph), "go")
			}(),
			from: 0, to: 0,
			copied:    withLang(pasted("plain", model.BlockContentText_Paragraph), "rust"),
			copyStyle: true,
			want:      want{style: model.BlockContentText_Paragraph, lang: "go", text: "plain"},
		},
		{
			name:   "copyStyle false does not adopt the icon",
			target: target("", model.BlockContentText_Paragraph),
			from:   0, to: 0,
			copied: func() *model.Block {
				b := pasted("note", model.BlockContentText_Callout)
				b.GetText().IconEmoji = "\U0001f4a1"
				return b
			}(),
			copyStyle: false,
			want:      want{style: model.BlockContentText_Paragraph, text: "note"},
		},
		{
			name: "copyStyle false does not clear the icon either",
			target: func() *model.Block {
				b := target("", model.BlockContentText_Paragraph)
				b.GetText().IconEmoji = "\U0001f525"
				return b
			}(),
			from: 0, to: 0,
			copied:    pasted("plain", model.BlockContentText_Paragraph),
			copyStyle: false,
			want:      want{style: model.BlockContentText_Paragraph, iconEmoji: "\U0001f525", text: "plain"},
		},
		{
			name:   "copyStyle false does not adopt the language",
			target: target("", model.BlockContentText_Paragraph),
			from:   0, to: 0,
			copied:    withLang(pasted("code", model.BlockContentText_Code), "go"),
			copyStyle: false,
			want:      want{style: model.BlockContentText_Paragraph, text: "code"},
		},
		{
			name: "copyStyle false does not clear the language either",
			target: func() *model.Block {
				return withLang(target("", model.BlockContentText_Paragraph), "go")
			}(),
			from: 0, to: 0,
			copied:    pasted("plain code", model.BlockContentText_Code),
			copyStyle: false,
			want:      want{style: model.BlockContentText_Paragraph, lang: "go", text: "plain code"},
		},
		{
			// nothing is adopted when the paste only replaces part of the text: the block
			// keeps its own presentation, so neither the icon nor the language moves
			name: "a partial replacement adopts neither the icon nor the language",
			target: func() *model.Block {
				b := withLang(target("old", model.BlockContentText_Paragraph), "go")
				b.GetText().IconEmoji = "\U0001f525"
				return b
			}(),
			from: 0, to: 1,
			copied: func() *model.Block {
				b := withLang(pasted("n", model.BlockContentText_Code), "rust")
				b.GetText().IconEmoji = "\U0001f4a1"
				return b
			}(),
			copyStyle: true,
			want: want{
				style: model.BlockContentText_Paragraph, iconEmoji: "\U0001f525",
				lang: "go", text: "nld",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			b := NewText(tc.target).(*Text)

			// when
			_, err := b.RangeTextPaste(tc.from, tc.to, tc.copied, tc.copyStyle)

			// then
			require.NoError(t, err)
			got := want{
				style:     b.content.Style,
				iconEmoji: b.content.IconEmoji,
				iconImage: b.content.IconImage,
				lang:      pbtypes.GetString(b.Model().Fields, CodeLangFieldName),
				text:      b.content.Text,
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

// textDetails binds a block to a relation through Fields, under DetailsKeyFieldName. Adopting
// the pasted block's Fields wholesale would drop that key and silently unbind the block, so
// only the key belonging to the adopted style may be written — never the struct as a whole,
// and never a key the source happens to carry.
//
// The whole resulting field map is asserted, not just that the binding is still present: a
// binding that survived alongside a key the paste had no business writing, or one quietly
// replaced by the source's own binding, both read as "still bound" to a narrower assertion.
func TestText_RangeTextPasteKeepsTheDetailsBinding(t *testing.T) {
	boundTo := func(rel string) *types.Value { return pbtypes.StringList([]string{rel}) }
	source := func(txt string, style model.BlockContentTextStyle, fields map[string]*types.Value) *model.Block {
		b := &model.Block{Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text: txt, Style: style, Marks: &model.BlockContentTextMarks{},
		}}}
		if fields != nil {
			b.Fields = &types.Struct{Fields: fields}
		}
		return b
	}

	for _, tc := range []struct {
		name         string
		targetFields map[string]*types.Value
		targetText   string
		from, to     int32
		copied       *model.Block
		want         *types.Struct
	}{
		{
			name:         "a Code source with a language adds only that language",
			targetFields: map[string]*types.Value{DetailsKeyFieldName: boundTo("name")},
			copied: source("fmt.Println(1)", model.BlockContentText_Code,
				map[string]*types.Value{CodeLangFieldName: pbtypes.String("go")}),
			want: &types.Struct{Fields: map[string]*types.Value{
				DetailsKeyFieldName: boundTo("name"),
				CodeLangFieldName:   pbtypes.String("go"),
			}},
		},
		{
			// the clearing branch runs on a bound block: it must remove one key, not the
			// map the binding lives in
			name:         "a Code source with no language leaves the binding standing",
			targetFields: map[string]*types.Value{DetailsKeyFieldName: boundTo("name")},
			copied:       source("plain code", model.BlockContentText_Code, nil),
			want: &types.Struct{Fields: map[string]*types.Value{
				DetailsKeyFieldName: boundTo("name"),
			}},
		},
		{
			name: "a Code source with no language clears the language and keeps the binding",
			targetFields: map[string]*types.Value{
				DetailsKeyFieldName: boundTo("name"),
				CodeLangFieldName:   pbtypes.String("go"),
			},
			copied: source("plain code", model.BlockContentText_Code, nil),
			want: &types.Struct{Fields: map[string]*types.Value{
				DetailsKeyFieldName: boundTo("name"),
			}},
		},
		{
			// a source carrying a binding of its own must not rebind the target
			name:         "a Code source carrying its own binding does not rebind the target",
			targetFields: map[string]*types.Value{DetailsKeyFieldName: boundTo("name")},
			copied: source("fmt.Println(1)", model.BlockContentText_Code, map[string]*types.Value{
				DetailsKeyFieldName: boundTo("description"),
				CodeLangFieldName:   pbtypes.String("go"),
			}),
			want: &types.Struct{Fields: map[string]*types.Value{
				DetailsKeyFieldName: boundTo("name"),
				CodeLangFieldName:   pbtypes.String("go"),
			}},
		},
		{
			name:         "a paragraph source carrying a binding and a language changes nothing",
			targetFields: map[string]*types.Value{DetailsKeyFieldName: boundTo("name")},
			copied: source("plain", model.BlockContentText_Paragraph, map[string]*types.Value{
				DetailsKeyFieldName: boundTo("description"),
				CodeLangFieldName:   pbtypes.String("rust"),
			}),
			want: &types.Struct{Fields: map[string]*types.Value{
				DetailsKeyFieldName: boundTo("name"),
			}},
		},
		{
			name:         "replacing all of a bound block's text keeps its binding",
			targetFields: map[string]*types.Value{DetailsKeyFieldName: boundTo("name")},
			targetText:   "old",
			from:         0, to: 3,
			copied: source("fmt.Println(1)", model.BlockContentText_Code, map[string]*types.Value{
				DetailsKeyFieldName: boundTo("description"),
			}),
			want: &types.Struct{Fields: map[string]*types.Value{
				DetailsKeyFieldName: boundTo("name"),
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			b := NewDetails(&model.Block{
				Restrictions: &model.BlockRestrictions{},
				Fields:       &types.Struct{Fields: tc.targetFields},
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text: tc.targetText, Style: model.BlockContentText_Paragraph,
					Marks: &model.BlockContentTextMarks{},
				}},
			}, DetailsKeys{Text: "name"}).(*textDetails)

			// when
			_, err := b.RangeTextPaste(tc.from, tc.to, tc.copied, true)

			// then
			require.NoError(t, err)
			assert.Equal(t, tc.want, b.Model().Fields)
		})
	}
}

// The list lives here now, next to the styles it is about, and basic.canHaveChildren delegates
// to it. Pinning it here means moving it cannot quietly change which styles may own children —
// paste consults the same answer when it decides where a pasted subtree lands.
func TestCanHaveChildren(t *testing.T) {
	for _, tc := range []struct {
		style model.BlockContentTextStyle
		want  bool
	}{
		{model.BlockContentText_Paragraph, true},
		{model.BlockContentText_Header1, false},
		{model.BlockContentText_Header2, false},
		{model.BlockContentText_Header3, false},
		{model.BlockContentText_Header4, false},
		{model.BlockContentText_Quote, true},
		{model.BlockContentText_Code, false},
		{model.BlockContentText_Title, false},
		{model.BlockContentText_Description, false},
		{model.BlockContentText_Checkbox, true},
		{model.BlockContentText_Marked, true},
		{model.BlockContentText_Numbered, true},
		{model.BlockContentText_Toggle, true},
		{model.BlockContentText_Callout, true},
		{model.BlockContentText_ToggleHeader1, true},
		{model.BlockContentText_ToggleHeader2, true},
		{model.BlockContentText_ToggleHeader3, true},
	} {
		t.Run(tc.style.String(), func(t *testing.T) {
			assert.Equal(t, tc.want, CanHaveChildren(tc.style))
		})
	}
}
