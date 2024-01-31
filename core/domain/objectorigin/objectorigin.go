package objectorigin

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type ObjectOrigin struct {
	Origin     model.ObjectOrigin
	ImportType model.ImportType
}

func (o ObjectOrigin) IsImported() bool {
	return o.Origin == model.ObjectOrigin_import
}

func (o ObjectOrigin) AddToDetails(details *types.Struct) {
	if o.Origin != model.ObjectOrigin_none {
		details.Fields[bundle.RelationKeyOrigin.String()] = pbtypes.Int64(int64(o.Origin))
		if o.Origin == model.ObjectOrigin_import || o.Origin == model.ObjectOrigin_usecase {
			details.Fields[bundle.RelationKeyImportType.String()] = pbtypes.Int64(int64(o.ImportType))
		}
	}
}

func ObjectOriginImport(importType model.ImportType) ObjectOrigin {
	return ObjectOrigin{
		Origin:     model.ObjectOrigin_import,
		ImportType: importType,
	}
}

func ObjectWebclipper() ObjectOrigin {
	return ObjectOrigin{
		Origin: model.ObjectOrigin_webclipper,
	}
}

func ObjectOriginUsecase() ObjectOrigin {
	return ObjectOrigin{
		Origin:     model.ObjectOrigin_usecase,
		ImportType: model.Import_Pb,
	}
}

func ObjectOriginNone() ObjectOrigin {
	return ObjectOrigin{
		Origin: model.ObjectOrigin_none,
	}
}

func ObjectOriginClipboard() ObjectOrigin {
	return ObjectOrigin{
		Origin: model.ObjectOrigin_clipboard,
	}
}

func ObjectOriginBookmark() ObjectOrigin {
	return ObjectOrigin{
		Origin: model.ObjectOrigin_bookmark,
	}
}

func ObjectOriginDragAndDrop() ObjectOrigin {
	return ObjectOrigin{
		Origin: model.ObjectOrigin_dragAndDrop,
	}
}
