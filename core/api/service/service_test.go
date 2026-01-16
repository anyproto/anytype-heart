package service

import (
	"testing"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

const (
	offset              = 0
	limit               = 100
	gatewayUrl          = "http://localhost:31006"
	techSpaceId         = "tech-space-id"
	mockedSpaceId       = "mocked-space-id"
	mockedObjectId      = "mocked-object-id"
	mockedNewObjectId   = "mocked-new-object-id"
	mockedObjectName    = "mocked-object-name"
	mockedObjectSnippet = "mocked-object-snippet"
	mockedObjectIcon    = "🔍"
	mockedParticipantId = "mocked-participant-id"
	mockedTypeKey       = "page"
	mockedTypeId        = "mocked-type-id"
	mockedTypeName      = "mocked-type-name"
	mockedTypeIcon      = "📝"
	mockedTemplateId    = "mocked-template-id"
	mockedTemplateName  = "mocked-template-name"
	mockedTemplateIcon  = "📃"
)

type fixture struct {
	service              *Service
	mwMock               *mock_apicore.MockClientCommands
	crossSpaceSubService *mock_apicore.MockCrossSpaceSubscriptionService
}

func newFixture(t *testing.T) *fixture {
	mwMock := mock_apicore.NewMockClientCommands(t)
	crossSpaceSubService := mock_apicore.NewMockCrossSpaceSubscriptionService(t)
	service := NewService(mwMock, gatewayUrl, techSpaceId, crossSpaceSubService)

	return &fixture{
		service:              service,
		mwMock:               mwMock,
		crossSpaceSubService: crossSpaceSubService,
	}
}

func (fx *fixture) populateCache(spaceId string) {
	for _, prop := range GetTestProperties() {
		fx.service.cache.cacheProperty(spaceId, prop)
	}
	for _, typ := range GetTestTypes() {
		fx.service.cache.cacheType(spaceId, typ)
	}
	for _, tag := range GetTestTags() {
		fx.service.cache.cacheTag(spaceId, tag)
	}
}

// Common test properties
func GetTestProperties() []*apimodel.Property {
	return []*apimodel.Property{
		// System properties
		{
			Id:          "prop1",
			Key:         "name",
			Name:        "Name",
			Format:      apimodel.PropertyFormatText,
			RelationKey: bundle.RelationKeyName.String(),
		},
		{
			Id:          "prop2",
			Key:         "description",
			Name:        "Description",
			Format:      apimodel.PropertyFormatText,
			RelationKey: bundle.RelationKeyDescription.String(),
		},
		{
			Id:          "prop3",
			Key:         "due_date",
			Name:        "Due Date",
			Format:      apimodel.PropertyFormatDate,
			RelationKey: bundle.RelationKeyDueDate.String(),
		},
		{
			Id:          "prop4",
			Key:         "tags",
			Name:        "Tags",
			Format:      apimodel.PropertyFormatMultiSelect,
			RelationKey: bundle.RelationKeyTag.String(),
		},
		{
			Id:          "prop5",
			Key:         "created_date",
			Name:        "Created Date",
			Format:      apimodel.PropertyFormatDate,
			RelationKey: bundle.RelationKeyCreatedDate.String(),
		},
		{
			Id:          "prop6",
			Key:         "last_modified_date",
			Name:        "Last Modified Date",
			Format:      apimodel.PropertyFormatDate,
			RelationKey: bundle.RelationKeyLastModifiedDate.String(),
		},
		{
			Id:          "prop7",
			Key:         "last_opened_date",
			Name:        "Last Opened Date",
			Format:      apimodel.PropertyFormatDate,
			RelationKey: bundle.RelationKeyLastOpenedDate.String(),
		},
		{
			Id:          "prop8",
			Key:         "creator",
			Name:        "Created by",
			Format:      apimodel.PropertyFormatObjects,
			RelationKey: bundle.RelationKeyCreator.String(),
		},
		{
			Id:          "prop9",
			Key:         "last_modified_by",
			Name:        "Last Modified By",
			Format:      apimodel.PropertyFormatObjects,
			RelationKey: bundle.RelationKeyLastModifiedBy.String(),
		},
		// Test properties for property_test.go
		{
			Id:          "text_prop_id",
			Key:         "text_prop",
			Name:        "Text Property",
			Format:      apimodel.PropertyFormatText,
			RelationKey: "text_prop",
		},
		{
			Id:          "number_prop_id",
			Key:         "number_prop",
			Name:        "Number Property",
			Format:      apimodel.PropertyFormatNumber,
			RelationKey: "number_prop",
		},
		{
			Id:          "select_prop_id",
			Key:         "select_prop",
			Name:        "Select Property",
			Format:      apimodel.PropertyFormatSelect,
			RelationKey: "select_prop",
		},
		{
			Id:          "multi_select_prop_id",
			Key:         "multi_select_prop",
			Name:        "Multi Select Property",
			Format:      apimodel.PropertyFormatMultiSelect,
			RelationKey: "multi_select_prop",
		},
		{
			Id:          "date_prop_id",
			Key:         "date_prop",
			Name:        "Date Property",
			Format:      apimodel.PropertyFormatDate,
			RelationKey: "date_prop",
		},
		{
			Id:          "files_prop_id",
			Key:         "files_prop",
			Name:        "Files Property",
			Format:      apimodel.PropertyFormatFiles,
			RelationKey: "files_prop",
		},
		{
			Id:          "checkbox_prop_id",
			Key:         "checkbox_prop",
			Name:        "Checkbox Property",
			Format:      apimodel.PropertyFormatCheckbox,
			RelationKey: "checkbox_prop",
		},
		{
			Id:          "url_prop_id",
			Key:         "url_prop",
			Name:        "URL Property",
			Format:      apimodel.PropertyFormatUrl,
			RelationKey: "url_prop",
		},
		{
			Id:          "email_prop_id",
			Key:         "email_prop",
			Name:        "Email Property",
			Format:      apimodel.PropertyFormatEmail,
			RelationKey: "email_prop",
		},
		{
			Id:          "phone_prop_id",
			Key:         "phone_prop",
			Name:        "Phone Property",
			Format:      apimodel.PropertyFormatPhone,
			RelationKey: "phone_prop",
		},
		{
			Id:          "objects_prop_id",
			Key:         "objects_prop",
			Name:        "Objects Property",
			Format:      apimodel.PropertyFormatObjects,
			RelationKey: "objects_prop",
		},
	}
}

// Common test types
func GetTestTypes() []*apimodel.Type {
	return []*apimodel.Type{
		{
			Id:   "mocked-type-id",
			Key:  "page",
			Name: "mocked-type-name",
			Icon: &apimodel.Icon{
				WrappedIcon: apimodel.EmojiIcon{
					Format: apimodel.IconFormatEmoji,
					Emoji:  "📝",
				},
			},
			UniqueKey:  "ot-page",
			Layout:     apimodel.ObjectLayoutBasic,
			Properties: []apimodel.Property{},
		},
		{
			Id:         "type2",
			Key:        "task",
			Icon:       nil,
			Name:       "Task",
			UniqueKey:  "ot-task",
			Layout:     apimodel.ObjectLayoutAction,
			Properties: []apimodel.Property{},
		},
	}
}

// Common test tags
func GetTestTags() []*apimodel.Tag {
	return []*apimodel.Tag{
		{
			Id:        "tag1",
			Key:       "important_tag",
			Name:      "Important",
			Color:     "red",
			UniqueKey: "unique_tag_1",
		},
		{
			Id:        "tag2",
			Key:       "urgent_tag",
			Name:      "Urgent",
			Color:     "orange",
			UniqueKey: "unique_tag_2",
		},
	}
}
