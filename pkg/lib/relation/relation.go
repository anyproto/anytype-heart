package relation

import (
	"log"

	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
)

// ObjectTypeSelfType used in relations which target the type of the object which holds this relation
const ObjectTypeSelfType = "self"

// all required internal relations will be added to any new object type
var RequiredInternalRelations = []string{"id", "name", "type", "createdDate", "lastModifiedDate", "lastOpenedDate"}

var FormatFilePossibleTargetObjectTypes = []string{objects.BundledObjectTypeURLPrefix + "file", objects.BundledObjectTypeURLPrefix + "image", objects.BundledObjectTypeURLPrefix + "video", objects.BundledObjectTypeURLPrefix + "audio"}

// filled in init
var LocalOnlyRelationsKeys []string

var (
	BundledRelations = map[string]*relation.Relation{
		"id": {
			Format:       relation.RelationFormat_object,
			ObjectTypes:  []string{ObjectTypeSelfType}, // the actual objectType of the object which has this relation will be injected here
			Name:         "Anytype ID",
			DefaultValue: nil,
			Key:          "id",
			DataSource:   relation.Relation_local,
			Hidden:       true,
			ReadOnly:     true,
		},
		"type": {
			Format:       relation.RelationFormat_object,
			Multi:        true,
			ObjectTypes:  []string{bundledObjectTypeURLPrefix + "objectType"},
			Name:         "Object Type",
			DefaultValue: nil,
			Key:          "type",
			DataSource:   relation.Relation_local,
			Hidden:       true,
			ReadOnly:     true,
		},
		"createdDate": {
			Format:       relation.RelationFormat_date,
			Name:         "Creation date",
			DefaultValue: nil,
			Key:          "createdDate",
			DataSource:   relation.Relation_local,
			Hidden:       false,
			ReadOnly:     true,
		},
		"lastModifiedDate": {
			Format:       relation.RelationFormat_date,
			Name:         "Last modified date",
			DefaultValue: nil,
			Key:          "lastModifiedDate",
			DataSource:   relation.Relation_local,
			Hidden:       false,
			ReadOnly:     true,
		},
		"lastOpenedDate": {
			Format:       relation.RelationFormat_date,
			Name:         "Last opened date",
			DefaultValue: nil,
			Key:          "lastOpenedDate",
			DataSource:   relation.Relation_local,
			Hidden:       false,
			ReadOnly:     true,
		},
		"name": {
			Format:       relation.RelationFormat_title,
			Name:         "Name",
			DefaultValue: nil,
			Key:          "name",
			DataSource:   relation.Relation_details,
			Hidden:       false,
			ReadOnly:     false,
		},
		"iconEmoji": {
			Format:       relation.RelationFormat_emoji,
			Name:         "Emoji icon",
			DefaultValue: nil,
			Key:          "iconEmoji",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     false,
		},
		"iconImage": {
			Format:       relation.RelationFormat_file,
			Name:         "Image icon",
			DefaultValue: nil,
			Key:          "iconImage",
			DataSource:   relation.Relation_details,
			ObjectTypes:  []string{objects.BundledObjectTypeURLPrefix + "image"},
			Hidden:       true,
			ReadOnly:     false,
		},
		"coverImage": {
			Format:       relation.RelationFormat_file,
			Name:         "Image cover",
			DefaultValue: nil,
			Key:          "coverImage",
			DataSource:   relation.Relation_details,
			ObjectTypes:  []string{objects.BundledObjectTypeURLPrefix + "image"},
			Hidden:       true,
			ReadOnly:     false,
		},
		"coverX": {
			Format:       relation.RelationFormat_number,
			Name:         "Image cover X offset",
			DefaultValue: nil,
			Key:          "coverX",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     false,
		},
		"coverY": {
			Format:       relation.RelationFormat_number,
			Name:         "Image cover Y offset",
			DefaultValue: nil,
			Key:          "coverY",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     false,
		},
		"coverScale": {
			Format:       relation.RelationFormat_number,
			Name:         "Image cover scale",
			DefaultValue: nil,
			Key:          "coverScale",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     false,
		},

		// files
		"fileMimeType": {
			Format:       relation.RelationFormat_title,
			Name:         "File mime type",
			DefaultValue: nil,
			Key:          "fileMimeType",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     true,
		},
		"sizeInBytes": {
			Format:       relation.RelationFormat_title,
			Name:         "Size in bytes",
			DefaultValue: nil,
			Key:          "sizeInBytes",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     true,
		},
		"addedDate": {
			Format:       relation.RelationFormat_date,
			Name:         "Added date",
			DefaultValue: nil,
			Key:          "addedDate",
			DataSource:   relation.Relation_local,
			Hidden:       false,
			ReadOnly:     true,
		},

		// image
		"widthInPixels": {
			Format:       relation.RelationFormat_title,
			Name:         "Width",
			DefaultValue: nil,
			Key:          "widthInPixels",
			DataSource:   relation.Relation_details,
			Hidden:       false,
			ReadOnly:     true,
		},
		"heightInPixels": {
			Format:       relation.RelationFormat_number,
			Name:         "Height",
			DefaultValue: nil,
			Key:          "heightInPixels",
			DataSource:   relation.Relation_details,
			Hidden:       false,
			ReadOnly:     true,
		},
		"camera": {
			Format:       relation.RelationFormat_title,
			Name:         "Camera model",
			DefaultValue: nil,
			Key:          "camera",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     true,
		},
		"latitude": {
			Format:       relation.RelationFormat_number,
			Name:         "Latitude",
			DefaultValue: nil,
			Key:          "latitude",
			DataSource:   relation.Relation_details,
			Hidden:       false,
			ReadOnly:     false,
		},
		"longitude": {
			Format:       relation.RelationFormat_number,
			Name:         "Longitude",
			DefaultValue: nil,
			Key:          "longitude",
			DataSource:   relation.Relation_details,
			Hidden:       false,
			ReadOnly:     false,
		},
		"exposureTime": {
			Format:       relation.RelationFormat_title,
			Name:         "Exposure time",
			DefaultValue: nil,
			Key:          "exposureTime",
			DataSource:   relation.Relation_details,
			Hidden:       false,
			ReadOnly:     false,
		},
		"focalRatio": {
			Format:       relation.RelationFormat_number,
			Name:         "Focal ratio",
			DefaultValue: nil,
			Key:          "focalRatio",
			DataSource:   relation.Relation_details,
			Hidden:       false,
			ReadOnly:     false,
		},
		"iso": {
			Format:       relation.RelationFormat_number,
			Name:         "ISO",
			DefaultValue: nil,
			Key:          "iso",
			DataSource:   relation.Relation_details,
			Hidden:       false,
			ReadOnly:     false,
		},
		// video
		"thumbnailImage": {
			Format:       relation.RelationFormat_object,
			ObjectTypes:  []string{bundledObjectTypeURLPrefix + "image"},
			Name:         "Thumbnail image",
			DefaultValue: nil,
			Key:          "thumbnailImage",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     false,
		},

		// audio
		"audioAlbum": {
			Format:       relation.RelationFormat_title,
			Name:         "Album",
			DefaultValue: nil,
			Key:          "audioAlbum",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     false,
		},
		"releasedYear": {
			Format:       relation.RelationFormat_number,
			Name:         "Released year",
			DefaultValue: nil,
			Key:          "releasedYear",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     false,
		},
		"composer": {
			Format:       relation.RelationFormat_number,
			Name:         "Composer",
			DefaultValue: nil,
			Key:          "composer",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     false,
		},
		"audioGenre": {
			Format:       relation.RelationFormat_number,
			Name:         "Audio Genre",
			DefaultValue: nil,
			Key:          "audioGenre",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     false,
		},
		"trackNumber": {
			Format:       relation.RelationFormat_number,
			Name:         "Track number",
			DefaultValue: nil,
			Key:          "trackNumber",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     false,
		},
	}
)

func MustGetBundledRelationByKey(key string) *relation.Relation {
	if v, ok := BundledRelations[key]; !ok {
		log.Fatal("MustGetBundledRelationByName got not-existing key: ", key)
		return nil
	} else {
		return v
	}
}

func init() {
	for _, r := range BundledRelations {
		if r.DataSource == relation.Relation_local {
			LocalOnlyRelationsKeys = append(LocalOnlyRelationsKeys, r.Key)
		}
	}
}
