package domain

import "github.com/anyproto/anytype-heart/pkg/lib/pb/model"

var FileLayouts = []model.ObjectTypeLayout{
	model.ObjectType_file,
	model.ObjectType_image,
	model.ObjectType_video,
	model.ObjectType_audio,
	model.ObjectType_pdf,
}

var GCEligibleLayouts = []model.ObjectTypeLayout{
	// Files
	model.ObjectType_file,
	model.ObjectType_image,
	model.ObjectType_video,
	model.ObjectType_audio,
	model.ObjectType_pdf,
	// User content objects
	model.ObjectType_basic,
	model.ObjectType_profile,
	model.ObjectType_todo,
	model.ObjectType_set,
	model.ObjectType_note,
	model.ObjectType_bookmark,
	model.ObjectType_collection,
}
