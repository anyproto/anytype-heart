package relation

import "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"

var (
	bundledRelations = map[string]*relation.Relation{
		"creationDate": {
			Format:       relation.RelationFormat_date,
			Name:         "Creation date",
			DefaultValue: nil,
			Key:          "creationDate",
			DataSource:   relation.Relation_local,
			Hidden:       false,
			ReadOnly:     true,
		},
		"modifiedDate": {
			Format:       relation.RelationFormat_date,
			Name:         "Last modified date",
			DefaultValue: nil,
			Key:          "modifiedDate",
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
			Hidden:       false,
			ReadOnly:     false,
		},
		"iconImage": {
			Format:       relation.RelationFormat_file,
			Name:         "Image icon",
			DefaultValue: nil,
			Key:          "iconImage",
			DataSource:   relation.Relation_details,
			Hidden:       false,
			ReadOnly:     false,
		},
		"coverImage": {
			Format:       relation.RelationFormat_file,
			Name:         "Image cover",
			DefaultValue: nil,
			Key:          "coverImage",
			DataSource:   relation.Relation_details,
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
			Key:          "coverX",
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
	}
)
