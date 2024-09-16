package api

import (
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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

func Test_BuildMarkdownFromAnnotationsColorGrey(t *testing.T) {
	rt := &RichText{
		Annotations: &Annotations{
			Bold:          false,
			Italic:        false,
			Strikethrough: false,
			Underline:     false,
			Code:          false,
			Color:         "gray",
		},
	}
	marks := rt.BuildMarkdownFromAnnotations(0, 5)
	assert.Len(t, marks, 1)
	assert.Equal(t, marks[0].Param, "grey")
}

func Test_GetFileBlockImage(t *testing.T) {
	f := &FileObject{
		Name: "file",
		File: FileProperty{
			URL:        "",
			ExpiryTime: &time.Time{},
		},
		External: FileProperty{
			URL:        "https:/example.ru/",
			ExpiryTime: &time.Time{},
		},
	}
	imageBlock, _ := f.GetFileBlock(model.BlockContentFile_Image)
	assert.NotNil(t, imageBlock.GetFile())
	assert.Equal(t, imageBlock.GetFile().Name, "https:/example.ru/")
	assert.Equal(t, imageBlock.GetFile().Type, model.BlockContentFile_Image)

	f = &FileObject{
		Name: "file",
		File: FileProperty{
			URL:        "https:/example.ru/",
			ExpiryTime: &time.Time{},
		},
		External: FileProperty{
			URL:        "",
			ExpiryTime: &time.Time{},
		},
	}
	imageBlock, _ = f.GetFileBlock(model.BlockContentFile_Image)
	assert.NotNil(t, imageBlock.GetFile())
	assert.Equal(t, imageBlock.GetFile().Name, "https:/example.ru/")
	assert.Equal(t, imageBlock.GetFile().Type, model.BlockContentFile_Image)
}

func Test_GetFileBlockPdf(t *testing.T) {
	f := &FileObject{
		Name: "file",
		File: FileProperty{
			URL:        "",
			ExpiryTime: &time.Time{},
		},
		External: FileProperty{
			URL:        "https:/example.ru/",
			ExpiryTime: &time.Time{},
		},
	}
	imageBlock, _ := f.GetFileBlock(model.BlockContentFile_PDF)
	assert.NotNil(t, imageBlock.GetFile())
	assert.Equal(t, imageBlock.GetFile().Name, "https:/example.ru/")
	assert.Equal(t, imageBlock.GetFile().Type, model.BlockContentFile_PDF)

	f = &FileObject{
		Name: "file",
		File: FileProperty{
			URL:        "https:/example.ru/",
			ExpiryTime: &time.Time{},
		},
		External: FileProperty{
			URL:        "",
			ExpiryTime: &time.Time{},
		},
	}
	imageBlock, _ = f.GetFileBlock(model.BlockContentFile_PDF)
	assert.NotNil(t, imageBlock.GetFile())
	assert.Equal(t, imageBlock.GetFile().Name, "https:/example.ru/")
	assert.Equal(t, imageBlock.GetFile().Type, model.BlockContentFile_PDF)
}

func Test_GetFileBlockFile(t *testing.T) {
	f := &FileObject{
		Name: "file",
		File: FileProperty{
			URL:        "",
			ExpiryTime: &time.Time{},
		},
		External: FileProperty{
			URL:        "https:/example.ru/",
			ExpiryTime: &time.Time{},
		},
	}
	imageBlock, _ := f.GetFileBlock(model.BlockContentFile_File)
	assert.NotNil(t, imageBlock.GetFile())
	assert.Equal(t, imageBlock.GetFile().Name, "https:/example.ru/")
	assert.Equal(t, imageBlock.GetFile().Type, model.BlockContentFile_File)

	f = &FileObject{
		Name: "file",
		File: FileProperty{
			URL:        "https:/example.ru/",
			ExpiryTime: &time.Time{},
		},
		External: FileProperty{
			URL:        "",
			ExpiryTime: &time.Time{},
		},
	}
	imageBlock, _ = f.GetFileBlock(model.BlockContentFile_File)
	assert.NotNil(t, imageBlock.GetFile())
	assert.Equal(t, imageBlock.GetFile().Name, "https:/example.ru/")
	assert.Equal(t, imageBlock.GetFile().Type, model.BlockContentFile_File)
}

func Test_GetFileBlockVideo(t *testing.T) {
	f := &FileObject{
		Name: "file",
		File: FileProperty{
			URL:        "",
			ExpiryTime: &time.Time{},
		},
		External: FileProperty{
			URL:        "https:/example.ru/",
			ExpiryTime: &time.Time{},
		},
	}
	imageBlock, _ := f.GetFileBlock(model.BlockContentFile_Video)
	assert.NotNil(t, imageBlock.GetFile())
	assert.Equal(t, imageBlock.GetFile().Name, "https:/example.ru/")
	assert.Equal(t, imageBlock.GetFile().Type, model.BlockContentFile_Video)

	f = &FileObject{
		Name: "file",
		File: FileProperty{
			URL:        "https:/example.ru/",
			ExpiryTime: &time.Time{},
		},
		External: FileProperty{
			URL:        "",
			ExpiryTime: &time.Time{},
		},
	}
	imageBlock, _ = f.GetFileBlock(model.BlockContentFile_Video)
	assert.NotNil(t, imageBlock.GetFile())
	assert.Equal(t, imageBlock.GetFile().Name, "https:/example.ru/")
	assert.Equal(t, imageBlock.GetFile().Type, model.BlockContentFile_Video)
}

func TestSetCover(t *testing.T) {
	t.Run("cover is nil", func(t *testing.T) {
		// given
		details := make(map[string]*types.Value, 0)

		// when
		SetCover(details, nil)

		// then
		assert.Empty(t, details)
	})
	t.Run("details are nil", func(t *testing.T) {
		// given
		var details map[string]*types.Value

		// when
		SetCover(details, &FileObject{
			Name: "filename",
			Type: External,
			External: FileProperty{
				URL: "url",
			},
		})

		// then
		assert.Empty(t, details)
	})
	t.Run("external type", func(t *testing.T) {
		// given
		details := make(map[string]*types.Value, 0)

		// when
		SetCover(details, &FileObject{
			Name: "filename",
			Type: External,
			External: FileProperty{
				URL: "url",
			},
		})

		// then
		assert.Equal(t, "url", details[bundle.RelationKeyCoverId.String()].GetStringValue())
		assert.Equal(t, float64(1), details[bundle.RelationKeyCoverType.String()].GetNumberValue())
	})
	t.Run("file type", func(t *testing.T) {
		// given
		details := make(map[string]*types.Value, 0)

		// when
		SetCover(details, &FileObject{
			Name: "filename",
			Type: File,
			File: FileProperty{
				URL: "url",
			},
		})

		// then
		assert.Equal(t, "url", details[bundle.RelationKeyCoverId.String()].GetStringValue())
		assert.Equal(t, float64(1), details[bundle.RelationKeyCoverType.String()].GetNumberValue())
	})
}
