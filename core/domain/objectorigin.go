package domain

import "github.com/anyproto/anytype-heart/pkg/lib/pb/model"

type ObjectOrigin struct {
	Origin     model.ObjectOrigin
	ImportType model.ImportType
}

func ObjectOriginImport(importType model.ImportType) ObjectOrigin {
	return ObjectOrigin{
		Origin:     model.ObjectOrigin_import,
		ImportType: importType,
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
