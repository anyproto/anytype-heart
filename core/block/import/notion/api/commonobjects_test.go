package api

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
)

func Test_BuildMarkdownFromAnnotationsBold(t *testing.T) {
	rt := &RichText{
		Annotations: &Annotations{
			Bold:          true,
			Italic:        false,
			Strikethrough: false,
			Underline:     false,
			Code:          false,
			Color:         "",
		},
	}
	marks := rt.BuildMarkdownFromAnnotations(0, 5)	
	assert.Len(t, marks, 1)
	assert.Equal(t, marks[0].Type, model.BlockContentTextMark_Bold)
}

func Test_BuildMarkdownFromAnnotationsItalic(t *testing.T) {
	rt := &RichText{
		Annotations: &Annotations{
			Bold:          false,
			Italic:        true,
			Strikethrough: false,
			Underline:     false,
			Code:          false,
			Color:         "",
		},
	}
	marks := rt.BuildMarkdownFromAnnotations(0, 5)	
	assert.Len(t, marks, 1)
	assert.Equal(t, marks[0].Type, model.BlockContentTextMark_Italic)
}

func Test_BuildMarkdownFromAnnotationsStrikethrough(t *testing.T) {
	rt := &RichText{
		Annotations: &Annotations{
			Bold:          false,
			Italic:        false,
			Strikethrough: true,
			Underline:     false,
			Code:          false,
			Color:         "",
		},
	}
	marks := rt.BuildMarkdownFromAnnotations(0, 5)	
	assert.Len(t, marks, 1)
	assert.Equal(t, marks[0].Type, model.BlockContentTextMark_Strikethrough)
}

func Test_BuildMarkdownFromAnnotationsUnderline(t *testing.T) {
	rt := &RichText{
		Annotations: &Annotations{
			Bold:          false,
			Italic:        false,
			Strikethrough: false,
			Underline:     true,
			Code:          false,
			Color:         "",
		},
	}
	marks := rt.BuildMarkdownFromAnnotations(0, 5)	
	assert.Len(t, marks, 1)
	assert.Equal(t, marks[0].Type, model.BlockContentTextMark_Underscored)
}

func Test_BuildMarkdownFromAnnotationsTwoMarks(t *testing.T) {
	rt := &RichText{
		Annotations: &Annotations{
			Bold:          true,
			Italic:        true,
			Strikethrough: false,
			Underline:     false,
			Code:          false,
			Color:         "",
		},
	}
	marks := rt.BuildMarkdownFromAnnotations(0, 5)	
	assert.Len(t, marks, 2)
	assert.Equal(t, marks[0].Type, model.BlockContentTextMark_Bold)
	assert.Equal(t, marks[1].Type, model.BlockContentTextMark_Italic)
}

func Test_BuildMarkdownFromAnnotationsColor(t *testing.T) {
	rt := &RichText{
		Annotations: &Annotations{
			Bold:          false,
			Italic:        false,
			Strikethrough: false,
			Underline:     false,
			Code:          false,
			Color:         "red",
		},
	}
	marks := rt.BuildMarkdownFromAnnotations(0, 5)	
	assert.Len(t, marks, 1)
	assert.Equal(t, marks[0].Param, "red")
}