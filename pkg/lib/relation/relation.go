package relation

import "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"

var (
	bundledRelations = map[string]*relation.Relation{
		"creationDate": {
			Format:       relation.RelationFormat_date,
			DefaultName:  "Creation date",
			DefaultValue: nil,
			DataKey:      "creationDate",
			DataSource:   relation.Relation_local,
			Hidden:       false,
			ReadOnly:     true,
		},
		"modifiedDate": {
			Format:       relation.RelationFormat_date,
			DefaultName:  "Last modified date",
			DefaultValue: nil,
			DataKey:      "modifiedDate",
			DataSource:   relation.Relation_local,
			Hidden:       false,
			ReadOnly:     true,
		},
		"name": {
			Format:       relation.RelationFormat_shortText,
			DefaultName:  "Name",
			DefaultValue: nil,
			DataKey:      "name",
			DataSource:   relation.Relation_details,
			Hidden:       false,
			ReadOnly:     false,
		},
		"iconEmoji": {
			Format:       relation.RelationFormat_emoji,
			DefaultName:  "Emoji icon",
			DefaultValue: nil,
			DataKey:      "iconEmoji",
			DataSource:   relation.Relation_details,
			Hidden:       false,
			ReadOnly:     false,
		},
		"iconImage": {
			Format:       relation.RelationFormat_file,
			DefaultName:  "Image icon",
			DefaultValue: nil,
			DataKey:      "iconImage",
			DataSource:   relation.Relation_details,
			Hidden:       false,
			ReadOnly:     false,
		},
		"coverImage": {
			Format:       relation.RelationFormat_file,
			DefaultName:  "Image cover",
			DefaultValue: nil,
			DataKey:      "coverImage",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     false,
		},
		"coverX": {
			Format:       relation.RelationFormat_number,
			DefaultName:  "Image cover X offset",
			DefaultValue: nil,
			DataKey:      "coverX",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     false,
		},
		"coverY": {
			Format:       relation.RelationFormat_number,
			DefaultName:  "Image cover Y offset",
			DefaultValue: nil,
			DataKey:      "coverX",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     false,
		},
		"coverScale": {
			Format:       relation.RelationFormat_number,
			DefaultName:  "Image cover scale",
			DefaultValue: nil,
			DataKey:      "coverScale",
			DataSource:   relation.Relation_details,
			Hidden:       true,
			ReadOnly:     false,
		},
	}
)
