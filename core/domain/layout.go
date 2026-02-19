package domain

import "github.com/anyproto/anytype-heart/pkg/lib/pb/model"

var FileLayouts = []model.ObjectTypeLayout{
	model.ObjectType_file,
	model.ObjectType_image,
	model.ObjectType_video,
	model.ObjectType_audio,
	model.ObjectType_pdf,
}
