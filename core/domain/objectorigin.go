package domain

import "github.com/anyproto/anytype-heart/pkg/lib/pb/model"

type ObjectOrigin struct {
	Origin     model.ObjectOrigin
	ImportType model.ImportType
}

func ObjectOriginImport(origin model.ObjectOrigin, importType model.ImportType) *ObjectOrigin {
	return &ObjectOrigin{
		Origin:     origin,
		ImportType: importType,
	}
}

func ObjectOriginNone() *ObjectOrigin {
	return &ObjectOrigin{
		Origin: model.ObjectOrigin_none,
	}
}

func ObjectOriginClipboard() *ObjectOrigin {
	return &ObjectOrigin{
		Origin: model.ObjectOrigin_clipboard,
	}
}

func ObjectOriginBookmark() *ObjectOrigin {
	return &ObjectOrigin{
		Origin: model.ObjectOrigin_bookmark,
	}
}
