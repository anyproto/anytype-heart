/*
Code generated by pkg/lib/bundle/generator. DO NOT EDIT.
source: pkg/lib/bundle/types.json
*/
package bundle

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const TypeChecksum = "4887c4b1b59f6fa595cc67185d19aec9212680c2316aa452ec39b66d5eea1b83"
const (
	TypePrefix = "_ot"
)
const (
	TypeKeyRecipe         domain.TypeKey = "recipe"
	TypeKeyNote           domain.TypeKey = "note"
	TypeKeyContact        domain.TypeKey = "contact"
	TypeKeyBookmark       domain.TypeKey = "bookmark"
	TypeKeyDate           domain.TypeKey = "date"
	TypeKeyTask           domain.TypeKey = "task"
	TypeKeyRelation       domain.TypeKey = "relation"
	TypeKeyBook           domain.TypeKey = "book"
	TypeKeyVideo          domain.TypeKey = "video"
	TypeKeyDashboard      domain.TypeKey = "dashboard"
	TypeKeyMovie          domain.TypeKey = "movie"
	TypeKeyObjectType     domain.TypeKey = "objectType"
	TypeKeyRelationOption domain.TypeKey = "relationOption"
	TypeKeySpace          domain.TypeKey = "space"
	TypeKeySpaceView      domain.TypeKey = "spaceView"
	TypeKeyParticipant    domain.TypeKey = "participant"
	TypeKeyTemplate       domain.TypeKey = "template"
	TypeKeySet            domain.TypeKey = "set"
	TypeKeyCollection     domain.TypeKey = "collection"
	TypeKeyDiaryEntry     domain.TypeKey = "diaryEntry"
	TypeKeyPage           domain.TypeKey = "page"
	TypeKeyImage          domain.TypeKey = "image"
	TypeKeyProfile        domain.TypeKey = "profile"
	TypeKeyAudio          domain.TypeKey = "audio"
	TypeKeyGoal           domain.TypeKey = "goal"
	TypeKeyFile           domain.TypeKey = "file"
	TypeKeyProject        domain.TypeKey = "project"
	TypeKeyChat           domain.TypeKey = "chat"
	TypeKeyChatDerived    domain.TypeKey = "chatDerived"
)

var (
	types = map[domain.TypeKey]*model.ObjectType{
		TypeKeyAudio: {

			Description:            "",
			IconColor:              5,
			IconName:               "musical-notes",
			Layout:                 model.ObjectType_file,
			Name:                   "Audio",
			PluralName:             "Audio",
			Readonly:               true,
			RelationLinks:          []*model.RelationLink{MustGetRelationLink(RelationKeyAddedDate), MustGetRelationLink(RelationKeyOrigin), MustGetRelationLink(RelationKeyFileExt), MustGetRelationLink(RelationKeySizeInBytes), MustGetRelationLink(RelationKeyFileMimeType), MustGetRelationLink(RelationKeyArtist), MustGetRelationLink(RelationKeyAudioAlbum), MustGetRelationLink(RelationKeyAudioGenre), MustGetRelationLink(RelationKeyReleasedYear), MustGetRelationLink(RelationKeyAudioAlbumTrackNumber), MustGetRelationLink(RelationKeyAudioLyrics)},
			RestrictObjectCreation: true,
			Revision:               5,
			Types:                  []model.SmartBlockType{model.SmartBlockType_File},
			Url:                    TypePrefix + "audio",
		},
		TypeKeyBook: {

			Description:   "",
			IconColor:     3,
			IconName:      "book",
			Layout:        model.ObjectType_basic,
			Name:          "Book",
			PluralName:    "Books",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag), MustGetRelationLink(RelationKeyAuthor), MustGetRelationLink(RelationKeyStarred), MustGetRelationLink(RelationKeyStatus), MustGetRelationLink(RelationKeyUrl)},
			Revision:      3,
			Types:         []model.SmartBlockType{model.SmartBlockType_Page},
			Url:           TypePrefix + "book",
		},
		TypeKeyBookmark: {

			Description:   "",
			IconColor:     4,
			IconName:      "bookmark",
			Layout:        model.ObjectType_bookmark,
			Name:          "Bookmark",
			PluralName:    "Bookmarks",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag), MustGetRelationLink(RelationKeyPicture), MustGetRelationLink(RelationKeySource)},
			Revision:      3,
			Types:         []model.SmartBlockType{model.SmartBlockType_Page},
			Url:           TypePrefix + "bookmark",
		},
		TypeKeyChat: {

			Description:   "",
			Hidden:        true,
			IconColor:     7,
			IconName:      "chatbubble",
			Layout:        model.ObjectType_chat,
			Name:          "Chat [deprecated]",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag)},
			Revision:      2,
			Types:         []model.SmartBlockType{model.SmartBlockType_ChatObject},
			Url:           TypePrefix + "chat",
		},
		TypeKeyChatDerived: {

			Description:   "",
			Hidden:        true,
			IconColor:     7,
			IconName:      "chatbubble",
			Layout:        model.ObjectType_chatDerived,
			Name:          "Chat Derived Object",
			PluralName:    "Chat Derived Objects",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag)},
			Revision:      4,
			Types:         []model.SmartBlockType{model.SmartBlockType_ChatDerivedObject},
			Url:           TypePrefix + "chatDerived",
		},
		TypeKeyCollection: {

			Description:   "",
			IconColor:     7,
			IconName:      "layers",
			Layout:        model.ObjectType_collection,
			Name:          "Collection",
			PluralName:    "Collections",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag)},
			Revision:      3,
			Types:         []model.SmartBlockType{model.SmartBlockType_Page},
			Url:           TypePrefix + "collection",
		},
		TypeKeyContact: {

			Description:   "",
			IconColor:     8,
			IconName:      "id-card",
			Layout:        model.ObjectType_profile,
			Name:          "Contact",
			PluralName:    "Contacts",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag), MustGetRelationLink(RelationKeyCompany), MustGetRelationLink(RelationKeyEmail), MustGetRelationLink(RelationKeyPhone)},
			Revision:      3,
			Types:         []model.SmartBlockType{model.SmartBlockType_Page},
			Url:           TypePrefix + "contact",
		},
		TypeKeyDashboard: {

			Description:            "",
			Hidden:                 true,
			IconColor:              0,
			Layout:                 model.ObjectType_dashboard,
			Name:                   "Dashboard",
			Readonly:               true,
			RestrictObjectCreation: true,
			Types:                  []model.SmartBlockType{model.SmartBlockType_Home},
			Url:                    TypePrefix + "dashboard",
		},
		TypeKeyDate: {

			Description:   "",
			Hidden:        true,
			IconColor:     9,
			IconName:      "calendar",
			Layout:        model.ObjectType_date,
			Name:          "Date",
			PluralName:    "Dates",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag)},
			Revision:      1,
			Types:         []model.SmartBlockType{model.SmartBlockType_Date},
			Url:           TypePrefix + "date",
		},
		TypeKeyDiaryEntry: {

			Description:   "",
			IconColor:     2,
			IconName:      "reader",
			Layout:        model.ObjectType_basic,
			Name:          "Diary Entry",
			PluralName:    "Diary Entries",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag), MustGetRelationLink(RelationKeyMood)},
			Revision:      3,
			Types:         []model.SmartBlockType{model.SmartBlockType_Page},
			Url:           TypePrefix + "diaryEntry",
		},
		TypeKeyFile: {

			Description:            "",
			IconColor:              7,
			IconName:               "attach",
			Layout:                 model.ObjectType_file,
			Name:                   "File",
			PluralName:             "Files",
			Readonly:               true,
			RelationLinks:          []*model.RelationLink{MustGetRelationLink(RelationKeyAddedDate), MustGetRelationLink(RelationKeyOrigin), MustGetRelationLink(RelationKeyFileExt), MustGetRelationLink(RelationKeySizeInBytes), MustGetRelationLink(RelationKeyFileMimeType)},
			RestrictObjectCreation: true,
			Revision:               5,
			Types:                  []model.SmartBlockType{model.SmartBlockType_File},
			Url:                    TypePrefix + "file",
		},
		TypeKeyGoal: {

			Description:   "",
			IconColor:     4,
			IconName:      "flag",
			Layout:        model.ObjectType_todo,
			Name:          "Goal",
			PluralName:    "Goals",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag), MustGetRelationLink(RelationKeyDueDate), MustGetRelationLink(RelationKeyProgress), MustGetRelationLink(RelationKeyStatus)},
			Revision:      3,
			Types:         []model.SmartBlockType{model.SmartBlockType_Page},
			Url:           TypePrefix + "goal",
		},
		TypeKeyImage: {

			Description:            "",
			IconColor:              10,
			IconName:               "image",
			Layout:                 model.ObjectType_image,
			Name:                   "Image",
			PluralName:             "Images",
			Readonly:               true,
			RelationLinks:          []*model.RelationLink{MustGetRelationLink(RelationKeyAddedDate), MustGetRelationLink(RelationKeyOrigin), MustGetRelationLink(RelationKeyFileExt), MustGetRelationLink(RelationKeySizeInBytes), MustGetRelationLink(RelationKeyHeightInPixels), MustGetRelationLink(RelationKeyWidthInPixels), MustGetRelationLink(RelationKeyFileMimeType), MustGetRelationLink(RelationKeyCamera), MustGetRelationLink(RelationKeyCameraIso), MustGetRelationLink(RelationKeyAperture), MustGetRelationLink(RelationKeyExposure), MustGetRelationLink(RelationKeyFocalRatio)},
			RestrictObjectCreation: true,
			Revision:               5,
			Types:                  []model.SmartBlockType{model.SmartBlockType_File},
			Url:                    TypePrefix + "image",
		},
		TypeKeyMovie: {

			Description:   "",
			IconColor:     5,
			IconName:      "film",
			Layout:        model.ObjectType_basic,
			Name:          "Movie",
			PluralName:    "Movies",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag), MustGetRelationLink(RelationKeyGenre), MustGetRelationLink(RelationKeyStatus)},
			Revision:      3,
			Types:         []model.SmartBlockType{model.SmartBlockType_Page},
			Url:           TypePrefix + "movie",
		},
		TypeKeyNote: {

			Description:   "",
			IconColor:     2,
			IconName:      "create",
			Layout:        model.ObjectType_note,
			Name:          "Note",
			PluralName:    "Notes",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag)},
			Revision:      3,
			Types:         []model.SmartBlockType{model.SmartBlockType_Page},
			Url:           TypePrefix + "note",
		},
		TypeKeyObjectType: {

			Description:   "",
			Hidden:        true,
			IconColor:     9,
			IconName:      "extension-puzzle",
			Layout:        model.ObjectType_objectType,
			Name:          "Type",
			PluralName:    "Types",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyRecommendedRelations), MustGetRelationLink(RelationKeyRecommendedLayout)},
			Revision:      3,
			Types:         []model.SmartBlockType{model.SmartBlockType_SubObject, model.SmartBlockType_BundledObjectType},
			Url:           TypePrefix + "objectType",
		},
		TypeKeyPage: {

			Description:   "",
			IconColor:     8,
			IconName:      "document",
			Layout:        model.ObjectType_basic,
			Name:          "Page",
			PluralName:    "Pages",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag)},
			Revision:      3,
			Types:         []model.SmartBlockType{model.SmartBlockType_Page},
			Url:           TypePrefix + "page",
		},
		TypeKeyParticipant: {

			Description:            "",
			IconColor:              3,
			IconName:               "person",
			Layout:                 model.ObjectType_participant,
			Name:                   "Space member",
			PluralName:             "Space members",
			Readonly:               true,
			RelationLinks:          []*model.RelationLink{MustGetRelationLink(RelationKeyTag)},
			RestrictObjectCreation: true,
			Revision:               4,
			Types:                  []model.SmartBlockType{model.SmartBlockType_Participant},
			Url:                    TypePrefix + "participant",
		},
		TypeKeyProfile: {

			Description:   "",
			Hidden:        true,
			IconColor:     3,
			IconName:      "man",
			Layout:        model.ObjectType_profile,
			Name:          "Human",
			PluralName:    "Humans",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag)},
			Revision:      3,
			Types:         []model.SmartBlockType{model.SmartBlockType_Page, model.SmartBlockType_ProfilePage},
			Url:           TypePrefix + "profile",
		},
		TypeKeyProject: {

			Description:   "",
			IconColor:     3,
			IconName:      "hammer",
			Layout:        model.ObjectType_basic,
			Name:          "Project",
			PluralName:    "Projects",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag)},
			Revision:      3,
			Types:         []model.SmartBlockType{model.SmartBlockType_Page},
			Url:           TypePrefix + "project",
		},
		TypeKeyRecipe: {

			Description:   "",
			IconColor:     4,
			IconName:      "pizza",
			Layout:        model.ObjectType_basic,
			Name:          "Recipe",
			PluralName:    "Recipes",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag), MustGetRelationLink(RelationKeyIngredients), MustGetRelationLink(RelationKeyTime)},
			Revision:      3,
			Types:         []model.SmartBlockType{model.SmartBlockType_Page},
			Url:           TypePrefix + "recipe",
		},
		TypeKeyRelation: {

			Description:   "",
			Hidden:        true,
			IconColor:     7,
			IconName:      "share-social",
			Layout:        model.ObjectType_relation,
			Name:          "Relation",
			PluralName:    "Relation",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyRelationFormat), MustGetRelationLink(RelationKeyRelationMaxCount), MustGetRelationLink(RelationKeyRelationDefaultValue), MustGetRelationLink(RelationKeyRelationFormatObjectTypes)},
			Revision:      3,
			Types:         []model.SmartBlockType{model.SmartBlockType_SubObject, model.SmartBlockType_BundledRelation},
			Url:           TypePrefix + "relation",
		},
		TypeKeyRelationOption: {

			Description: "",
			Hidden:      true,
			IconColor:   0,
			Layout:      model.ObjectType_relationOption,
			Name:        "Relation option",
			Readonly:    true,
			Types:       []model.SmartBlockType{model.SmartBlockType_SubObject},
			Url:         TypePrefix + "relationOption",
		},
		TypeKeySet: {

			Description:   "",
			IconColor:     6,
			IconName:      "search",
			Layout:        model.ObjectType_set,
			Name:          "Query",
			PluralName:    "Queries",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag), MustGetRelationLink(RelationKeySetOf)},
			Revision:      4,
			Types:         []model.SmartBlockType{model.SmartBlockType_Page},
			Url:           TypePrefix + "set",
		},
		TypeKeySpace: {

			Description:            "",
			Hidden:                 true,
			IconColor:              10,
			IconName:               "folder",
			Layout:                 model.ObjectType_space,
			Name:                   "Space",
			PluralName:             "Spaces",
			Readonly:               true,
			RelationLinks:          []*model.RelationLink{MustGetRelationLink(RelationKeyTag)},
			RestrictObjectCreation: true,
			Revision:               3,
			Types:                  []model.SmartBlockType{model.SmartBlockType_Workspace},
			Url:                    TypePrefix + "space",
		},
		TypeKeySpaceView: {

			Description:            "",
			Hidden:                 true,
			IconColor:              10,
			IconName:               "folder",
			Layout:                 model.ObjectType_spaceView,
			Name:                   "Space",
			PluralName:             "Spaces",
			Readonly:               true,
			RelationLinks:          []*model.RelationLink{MustGetRelationLink(RelationKeyTag)},
			RestrictObjectCreation: true,
			Revision:               2,
			Types:                  []model.SmartBlockType{model.SmartBlockType_SpaceView},
			Url:                    TypePrefix + "spaceView",
		},
		TypeKeyTask: {

			Description:   "",
			IconColor:     10,
			IconName:      "checkbox",
			Layout:        model.ObjectType_todo,
			Name:          "Task",
			PluralName:    "Tasks",
			Readonly:      true,
			RelationLinks: []*model.RelationLink{MustGetRelationLink(RelationKeyTag), MustGetRelationLink(RelationKeyAssignee), MustGetRelationLink(RelationKeyDone), MustGetRelationLink(RelationKeyDueDate), MustGetRelationLink(RelationKeyLinkedProjects), MustGetRelationLink(RelationKeyStatus)},
			Revision:      3,
			Types:         []model.SmartBlockType{model.SmartBlockType_Page},
			Url:           TypePrefix + "task",
		},
		TypeKeyTemplate: {

			Description:            "",
			IconColor:              8,
			IconName:               "copy",
			Layout:                 model.ObjectType_basic,
			Name:                   "Template",
			PluralName:             "Templates",
			Readonly:               true,
			RelationLinks:          []*model.RelationLink{MustGetRelationLink(RelationKeyTargetObjectType), MustGetRelationLink(RelationKeyTemplateIsBundled)},
			RestrictObjectCreation: true,
			Revision:               4,
			Types:                  []model.SmartBlockType{model.SmartBlockType_Template},
			Url:                    TypePrefix + "template",
		},
		TypeKeyVideo: {

			Description:            "",
			IconColor:              6,
			IconName:               "videocam",
			Layout:                 model.ObjectType_file,
			Name:                   "Video",
			PluralName:             "Video",
			Readonly:               true,
			RelationLinks:          []*model.RelationLink{MustGetRelationLink(RelationKeyAddedDate), MustGetRelationLink(RelationKeyOrigin), MustGetRelationLink(RelationKeyFileExt), MustGetRelationLink(RelationKeySizeInBytes), MustGetRelationLink(RelationKeyHeightInPixels), MustGetRelationLink(RelationKeyWidthInPixels), MustGetRelationLink(RelationKeyFileMimeType), MustGetRelationLink(RelationKeyCamera), MustGetRelationLink(RelationKeyCameraIso), MustGetRelationLink(RelationKeyAperture), MustGetRelationLink(RelationKeyExposure)},
			RestrictObjectCreation: true,
			Revision:               5,
			Types:                  []model.SmartBlockType{model.SmartBlockType_File},
			Url:                    TypePrefix + "video",
		},
	}
)
