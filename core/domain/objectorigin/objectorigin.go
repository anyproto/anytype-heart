package objectorigin

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ObjectOrigin struct {
	Origin     model.ObjectOrigin
	ImportType model.ImportType
}

func FromDetails(details *domain.Details) ObjectOrigin {
	origin := details.GetInt64(bundle.RelationKeyOrigin)
	importType := details.GetInt64(bundle.RelationKeyImportType)

	return ObjectOrigin{
		Origin:     model.ObjectOrigin(origin),
		ImportType: model.ImportType(importType),
	}
}

func (o ObjectOrigin) IsImported() bool {
	return o.Origin == model.ObjectOrigin_import
}

func (o ObjectOrigin) AddToDetails(details *domain.Details) {
	if o.Origin != model.ObjectOrigin_none {
		details.SetInt64(bundle.RelationKeyOrigin, int64(o.Origin))
		if o.Origin == model.ObjectOrigin_import || o.Origin == model.ObjectOrigin_usecase {
			details.SetInt64(bundle.RelationKeyImportType, int64(o.ImportType))
		}
	}
}

func Import(importType model.ImportType) ObjectOrigin {
	return ObjectOrigin{
		Origin:     model.ObjectOrigin_import,
		ImportType: importType,
	}
}

func Usecase() ObjectOrigin {
	return ObjectOrigin{
		Origin:     model.ObjectOrigin_usecase,
		ImportType: model.Import_Pb,
	}
}

func None() ObjectOrigin {
	return ObjectOrigin{
		Origin: model.ObjectOrigin_none,
	}
}

func Clipboard() ObjectOrigin {
	return ObjectOrigin{
		Origin: model.ObjectOrigin_clipboard,
	}
}

func Bookmark() ObjectOrigin {
	return ObjectOrigin{
		Origin: model.ObjectOrigin_bookmark,
	}
}

func DragAndDrop() ObjectOrigin {
	return ObjectOrigin{
		Origin: model.ObjectOrigin_dragAndDrop,
	}
}

func Webclipper() ObjectOrigin {
	return ObjectOrigin{
		Origin: model.ObjectOrigin_webclipper,
	}
}
