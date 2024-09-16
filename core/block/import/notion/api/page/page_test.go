package page

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/client"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/property"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func Test_handlePagePropertiesSelect(t *testing.T) {
	details := make(map[string]*types.Value, 0)
	c := client.NewClient()
	selectProperty := property.SelectItem{
		Object: "",
		ID:     "id",
		Type:   string(property.PropertyConfigTypeSelect),
		Select: property.SelectOption{
			ID:    "id",
			Name:  "Name",
			Color: api.Blue,
		},
	}
	properties := property.Properties{"Select": &selectProperty}
	pageTask := Task{
		propertyService:        property.New(c),
		relationOptCreateMutex: &sync.Mutex{},
		relationCreateMutex:    &sync.Mutex{},
		p:                      Page{Properties: properties},
	}
	req := property.NewPropertiesStore()
	do := &DataObject{
		request:   &api.NotionImportContext{},
		relations: req,
	}
	snapshots, _ := pageTask.handlePageProperties(do, details)

	assert.Len(t, snapshots, 2) // 1 relation + 1 option
	assert.Len(t, req.RelationsIdsToOptions, 1)
	for _, options := range req.RelationsIdsToOptions {
		assert.Len(t, options, 1)
		assert.NotNil(t, options[0].Details)
		assert.Equal(t, options[0].Details.Fields[bundle.RelationKeyName.String()], pbtypes.String("Name"))
		assert.Equal(t, options[0].Details.Fields[bundle.RelationKeyRelationOptionColor.String()], pbtypes.String("blue"))
	}

	// Relation already exist
	selectProperty = property.SelectItem{
		Object: "",
		ID:     "id",
		Type:   string(property.PropertyConfigTypeSelect),
		Select: property.SelectOption{
			ID:    "id",
			Name:  "Name 2",
			Color: api.Pink,
		},
	}
	snapshots, _ = pageTask.handlePageProperties(do, details)

	assert.NotEmpty(t, req)
	assert.Len(t, snapshots, 1) // 1 option
	assert.Len(t, req.RelationsIdsToOptions, 1)
	for _, options := range req.RelationsIdsToOptions {
		assert.Len(t, options, 2)
		assert.NotNil(t, options[0].Details)
		assert.Equal(t, options[0].Details.Fields[bundle.RelationKeyName.String()], pbtypes.String("Name"))
		assert.Equal(t, options[0].Details.Fields[bundle.RelationKeyRelationOptionColor.String()], pbtypes.String("blue"))

		assert.NotNil(t, options[1].Details)
		assert.Equal(t, options[1].Details.Fields[bundle.RelationKeyName.String()], pbtypes.String("Name 2"))
		assert.Equal(t, options[1].Details.Fields[bundle.RelationKeyRelationOptionColor.String()], pbtypes.String("pink"))
	}
}

func Test_handlePagePropertiesLastEditedTime(t *testing.T) {
	c := client.NewClient()
	details := make(map[string]*types.Value, 0)

	lastEditedTimeProperty := property.LastEditedTimeItem{
		ID:             "id",
		Type:           string(property.PropertyConfigLastEditedTime),
		LastEditedTime: "2022-10-24T22:56:00.000Z",
	}
	properties := property.Properties{"LastEditedTime": &lastEditedTimeProperty}
	pageTask := Task{
		propertyService:        property.New(c),
		relationOptCreateMutex: &sync.Mutex{},
		relationCreateMutex:    &sync.Mutex{},
		p:                      Page{Properties: properties},
	}
	req := property.NewPropertiesStore()
	do := &DataObject{
		request:   &api.NotionImportContext{},
		relations: req,
	}
	snapshots, _ := pageTask.handlePageProperties(do, details)
	assert.Len(t, snapshots, 1) // 1 relation
	assert.Len(t, req.PropertyIdsToSnapshots, 1)
	assert.NotEmpty(t, req.PropertyIdsToSnapshots["id"])
	key := pbtypes.GetString(req.PropertyIdsToSnapshots["id"].Details, bundle.RelationKeyRelationKey.String())
	assert.NotEmpty(t, details[key])
}

func Test_handlePagePropertiesRichText(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"list","results":[{"object":"property_item","type":"rich_text","id":"RPBv","rich_text":{"type":"text","text":{"content":"sdfsdfsdfsdfsdfsdf","link":null},"annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},"plain_text":"example text","href":null}}],"next_cursor":null,"has_more":false,"type":"property_item","property_item":{"id":"RPBv","next_url":null,"type":"rich_text","rich_text":{}}}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL
	details := make(map[string]*types.Value, 0)

	richTextProperty := property.RichTextItem{ID: "id", Type: string(property.PropertyConfigTypeRichText)}
	properties := property.Properties{"RichText": &richTextProperty}
	pageTask := Task{
		propertyService:        property.New(c),
		relationOptCreateMutex: &sync.Mutex{},
		relationCreateMutex:    &sync.Mutex{},
		p:                      Page{Properties: properties},
	}
	req := property.NewPropertiesStore()
	do := &DataObject{
		ctx:       context.Background(),
		request:   &api.NotionImportContext{},
		relations: req,
	}
	snapshots, _ := pageTask.handlePageProperties(do, details)

	assert.Len(t, snapshots, 1) // 1 relation
	assert.Len(t, req.PropertyIdsToSnapshots, 1)
	assert.NotEmpty(t, req.PropertyIdsToSnapshots["id"])
	key := pbtypes.GetString(req.PropertyIdsToSnapshots["id"].Details, bundle.RelationKeyRelationKey.String())
	assert.NotEmpty(t, details[key])
}

func Test_handlePagePropertiesDate(t *testing.T) {
	t.Run("parse Date property: date and time", func(t *testing.T) {
		// given
		c := client.NewClient()
		details := make(map[string]*types.Value, 0)

		dateProperty := property.DateItem{
			ID:   "id",
			Type: string(property.PropertyConfigTypeDate),
			Date: &api.DateObject{
				Start: "2023-11-08T20:27:00.000Z",
			},
		}
		properties := property.Properties{"Date": &dateProperty}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      Page{Properties: properties},
		}
		req := property.NewPropertiesStore()
		do := &DataObject{
			ctx:       context.Background(),
			request:   &api.NotionImportContext{},
			relations: req,
		}

		// when
		snapshots, _ := pageTask.handlePageProperties(do, details)

		// then
		assert.Len(t, snapshots, 1) // 1 relation
		assert.Len(t, req.PropertyIdsToSnapshots, 1)
		assert.NotEmpty(t, req.PropertyIdsToSnapshots["id"])
		key := pbtypes.GetString(req.PropertyIdsToSnapshots["id"].Details, bundle.RelationKeyRelationKey.String())
		assert.Equal(t, int(details[key].GetNumberValue()), 1699475220)
	})
	t.Run("parse Date property: only date", func(t *testing.T) {
		// given
		c := client.NewClient()
		details := make(map[string]*types.Value, 0)

		richTextProperty := property.DateItem{
			ID:   "id",
			Type: string(property.PropertyConfigTypeDate),
			Date: &api.DateObject{
				Start: "2023-11-08",
			},
		}
		properties := property.Properties{"Date": &richTextProperty}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      Page{Properties: properties},
		}
		req := property.NewPropertiesStore()
		do := &DataObject{
			ctx:       context.Background(),
			request:   &api.NotionImportContext{},
			relations: req,
		}

		// when
		snapshots, _ := pageTask.handlePageProperties(do, details)

		// then
		assert.Len(t, snapshots, 1) // 1 relation
		assert.Len(t, req.PropertyIdsToSnapshots, 1)
		assert.NotEmpty(t, req.PropertyIdsToSnapshots["id"])
		key := pbtypes.GetString(req.PropertyIdsToSnapshots["id"].Details, bundle.RelationKeyRelationKey.String())
		assert.Equal(t, int(details[key].GetNumberValue()), 1699401600)
	})
}

func Test_handlePagePropertiesStatus(t *testing.T) {
	c := client.NewClient()
	details := make(map[string]*types.Value, 0)

	statusProperty := property.StatusItem{
		ID:   "id",
		Type: property.PropertyConfigStatus,
		Status: &property.Status{
			Name:  "Done",
			ID:    "id",
			Color: api.Pink,
		},
	}
	properties := property.Properties{"Status": &statusProperty}
	pageTask := Task{
		propertyService:        property.New(c),
		relationOptCreateMutex: &sync.Mutex{},
		relationCreateMutex:    &sync.Mutex{},
		p:                      Page{Properties: properties},
	}
	req := property.NewPropertiesStore()
	do := &DataObject{
		request:   &api.NotionImportContext{},
		relations: req,
	}
	snapshots, _ := pageTask.handlePageProperties(do, details)

	assert.Len(t, snapshots, 2) // 1 relation + 1 option
	assert.Len(t, req.PropertyIdsToSnapshots, 1)
	assert.NotEmpty(t, req.PropertyIdsToSnapshots["id"])
	key := pbtypes.GetString(req.PropertyIdsToSnapshots["id"].Details, bundle.RelationKeyRelationKey.String())
	assert.NotEmpty(t, details[key])

	assert.Len(t, req.RelationsIdsToOptions, 1)
	for _, options := range req.RelationsIdsToOptions {
		assert.Len(t, options, 1)
		assert.NotNil(t, options[0].Details)
		assert.Equal(t, options[0].Details.Fields[bundle.RelationKeyName.String()], pbtypes.String("Done"))
		assert.Equal(t, options[0].Details.Fields[bundle.RelationKeyRelationOptionColor.String()], pbtypes.String("pink"))
	}

	// Relation already exist
	statusProperty = property.StatusItem{
		ID:   "id",
		Type: property.PropertyConfigStatus,
		Status: &property.Status{
			Name:  "In progress",
			ID:    "id",
			Color: api.Gray,
		},
	}
	snapshots, _ = pageTask.handlePageProperties(do, details)

	assert.NotEmpty(t, req)
	assert.Len(t, snapshots, 1) // 1 option
	assert.Len(t, req.RelationsIdsToOptions, 1)
	for _, options := range req.RelationsIdsToOptions {
		assert.Len(t, options, 2)
		assert.NotNil(t, options[0].Details)
		assert.Equal(t, options[0].Details.Fields[bundle.RelationKeyName.String()], pbtypes.String("Done"))
		assert.Equal(t, options[0].Details.Fields[bundle.RelationKeyRelationOptionColor.String()], pbtypes.String("pink"))

		assert.NotNil(t, options[1].Details)
		assert.Equal(t, options[1].Details.Fields[bundle.RelationKeyName.String()], pbtypes.String("In progress"))
		assert.Equal(t, options[1].Details.Fields[bundle.RelationKeyRelationOptionColor.String()], pbtypes.String("grey"))
	}
}

func Test_handlePageProperties(t *testing.T) {
	t.Run("empty status property", func(t *testing.T) {
		c := client.NewClient()
		details := make(map[string]*types.Value, 0)

		statusProperty := property.StatusItem{
			ID:   "id",
			Type: property.PropertyConfigStatus,
		}
		properties := property.Properties{"Status": &statusProperty}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      Page{Properties: properties},
		}
		req := property.NewPropertiesStore()
		do := &DataObject{
			request:   &api.NotionImportContext{},
			relations: req,
		}
		snapshots, _ := pageTask.handlePageProperties(do, details)

		assert.Len(t, snapshots, 1) // 1 relation without option
		assert.Len(t, req.PropertyIdsToSnapshots, 1)
		assert.NotEmpty(t, req.PropertyIdsToSnapshots["id"])
		key := pbtypes.GetString(req.PropertyIdsToSnapshots["id"].Details, bundle.RelationKeyRelationKey.String())
		assert.NotEmpty(t, details[key])
	})
}

func Test_handlePagePropertiesNumber(t *testing.T) {
	c := client.NewClient()
	details := make(map[string]*types.Value, 0)

	num := float64(12)
	numberProperty := property.NumberItem{
		ID:     "id",
		Type:   string(property.PropertyConfigTypeNumber),
		Number: &num,
	}
	properties := property.Properties{"Number": &numberProperty}
	pageTask := Task{
		propertyService:        property.New(c),
		relationOptCreateMutex: &sync.Mutex{},
		relationCreateMutex:    &sync.Mutex{},
		p:                      Page{Properties: properties},
	}
	req := property.NewPropertiesStore()
	do := &DataObject{
		request:   &api.NotionImportContext{},
		relations: req,
	}
	snapshots, _ := pageTask.handlePageProperties(do, details)

	assert.Len(t, snapshots, 1) // 1 relation
	assert.Len(t, req.PropertyIdsToSnapshots, 1)
	assert.NotEmpty(t, req.PropertyIdsToSnapshots["id"])
	key := pbtypes.GetString(req.PropertyIdsToSnapshots["id"].Details, bundle.RelationKeyRelationKey.String())
	assert.NotEmpty(t, details[key])
}

func Test_handlePagePropertiesMultiSelect(t *testing.T) {
	c := client.NewClient()
	details := make(map[string]*types.Value, 0)

	multiSelectProperty := property.MultiSelectItem{
		ID:   "id",
		Type: string(property.PropertyConfigTypeMultiSelect),
		MultiSelect: []*property.SelectOption{
			{
				ID:    "id",
				Name:  "Name",
				Color: api.Blue,
			},
		},
	}
	properties := property.Properties{"MultiSelect": &multiSelectProperty}
	pageTask := Task{
		propertyService:        property.New(c),
		relationOptCreateMutex: &sync.Mutex{},
		relationCreateMutex:    &sync.Mutex{},
		p:                      Page{Properties: properties},
	}
	req := property.NewPropertiesStore()
	do := &DataObject{
		request:   &api.NotionImportContext{},
		relations: req,
	}
	snapshots, _ := pageTask.handlePageProperties(do, details)

	assert.Len(t, snapshots, 2) // 1 relation + 1 option
	assert.Len(t, req.PropertyIdsToSnapshots, 1)
	assert.NotEmpty(t, req.PropertyIdsToSnapshots["id"])
	key := pbtypes.GetString(req.PropertyIdsToSnapshots["id"].Details, bundle.RelationKeyRelationKey.String())
	assert.NotEmpty(t, details[key])

	assert.Len(t, req.RelationsIdsToOptions, 1)
	for _, options := range req.RelationsIdsToOptions {
		assert.Len(t, options, 1)
		assert.NotNil(t, options[0].Details)
		assert.Equal(t, options[0].Details.Fields[bundle.RelationKeyName.String()], pbtypes.String("Name"))
		assert.Equal(t, options[0].Details.Fields[bundle.RelationKeyRelationOptionColor.String()], pbtypes.String("blue"))
	}

	// Relation already exist
	multiSelectProperty = property.MultiSelectItem{
		ID:   "id",
		Type: string(property.PropertyConfigTypeMultiSelect),
		MultiSelect: []*property.SelectOption{
			{
				ID:    "id",
				Name:  "Name 2",
				Color: api.Purple,
			},
		},
	}
	snapshots, _ = pageTask.handlePageProperties(do, details)

	assert.NotEmpty(t, req)
	assert.Len(t, snapshots, 1) // 1 option
	assert.Len(t, req.RelationsIdsToOptions, 1)
	for _, options := range req.RelationsIdsToOptions {
		assert.Len(t, options, 2)
		assert.NotNil(t, options[0].Details)
		assert.Equal(t, options[0].Details.Fields[bundle.RelationKeyName.String()], pbtypes.String("Name"))
		assert.Equal(t, options[0].Details.Fields[bundle.RelationKeyRelationOptionColor.String()], pbtypes.String("blue"))

		assert.NotNil(t, options[1].Details)
		assert.Equal(t, options[1].Details.Fields[bundle.RelationKeyName.String()], pbtypes.String("Name 2"))
		assert.Equal(t, options[1].Details.Fields[bundle.RelationKeyRelationOptionColor.String()], pbtypes.String("purple"))
	}
}

func Test_handlePagePropertiesCheckbox(t *testing.T) {
	c := client.NewClient()
	details := make(map[string]*types.Value, 0)

	checkboxProperty := property.CheckboxItem{
		ID:       "id",
		Type:     string(property.PropertyConfigTypeCheckbox),
		Checkbox: true,
	}
	properties := property.Properties{"Checkbox": &checkboxProperty}
	pageTask := Task{
		propertyService:        property.New(c),
		relationOptCreateMutex: &sync.Mutex{},
		relationCreateMutex:    &sync.Mutex{},
		p:                      Page{Properties: properties},
	}
	req := property.NewPropertiesStore()
	do := &DataObject{
		request:   &api.NotionImportContext{},
		relations: req,
	}
	snapshots, _ := pageTask.handlePageProperties(do, details)

	assert.Len(t, snapshots, 1) // 1 relation
	assert.Len(t, req.PropertyIdsToSnapshots, 1)
	assert.NotEmpty(t, req.PropertyIdsToSnapshots["id"])
	key := pbtypes.GetString(req.PropertyIdsToSnapshots["id"].Details, bundle.RelationKeyRelationKey.String())
	assert.NotEmpty(t, details[key])
}

func Test_handlePagePropertiesEmail(t *testing.T) {
	c := client.NewClient()
	details := make(map[string]*types.Value, 0)

	email := "a@mail.com"
	emailProperty := property.EmailItem{
		ID:    "id",
		Type:  string(property.PropertyConfigTypeEmail),
		Email: &email,
	}
	properties := property.Properties{"Email": &emailProperty}
	pageTask := Task{
		propertyService:        property.New(c),
		relationOptCreateMutex: &sync.Mutex{},
		relationCreateMutex:    &sync.Mutex{},
		p:                      Page{Properties: properties},
	}
	req := property.NewPropertiesStore()
	do := &DataObject{
		request:   &api.NotionImportContext{},
		relations: req,
	}
	snapshots, _ := pageTask.handlePageProperties(do, details)

	assert.Len(t, snapshots, 1) // 1 relation
	assert.Len(t, req.PropertyIdsToSnapshots, 1)
	assert.NotEmpty(t, req.PropertyIdsToSnapshots["id"])
	key := pbtypes.GetString(req.PropertyIdsToSnapshots["id"].Details, bundle.RelationKeyRelationKey.String())
	assert.NotEmpty(t, details[key])
}

func Test_handlePagePropertiesRelation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"list","results":[{"object":"property_item","type":"relation","id":"cm~~","relation":{"id":"id"}}],"next_cursor":null,"has_more":false,"type":"property_item","property_item":{"id":"cm~~","next_url":null,"type":"relation","relation":{}}}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL

	details := make(map[string]*types.Value, 0)

	relationProperty := property.RelationItem{ID: "id", Type: string(property.PropertyConfigTypeRelation), HasMore: true, Relation: []*property.Relation{{ID: "id"}}}
	properties := property.Properties{"Relation": &relationProperty}
	notionPageIdsToAnytype := map[string]string{"id": "anytypeID"}
	notionDatabaseIdsToAnytype := map[string]string{"id": "anytypeID"}
	req := &api.NotionImportContext{
		NotionPageIdsToAnytype:     notionPageIdsToAnytype,
		NotionDatabaseIdsToAnytype: notionDatabaseIdsToAnytype,
	}
	pageTask := Task{
		propertyService:        property.New(c),
		relationOptCreateMutex: &sync.Mutex{},
		relationCreateMutex:    &sync.Mutex{},
		p:                      Page{Properties: properties},
	}
	store := property.NewPropertiesStore()
	do := &DataObject{
		ctx:       context.Background(),
		request:   req,
		relations: store,
	}
	snapshots, _ := pageTask.handlePageProperties(do, details)

	assert.Len(t, snapshots, 1) // 1 relation
	assert.Len(t, store.PropertyIdsToSnapshots, 1)
	assert.NotEmpty(t, store.PropertyIdsToSnapshots["id"])
	key := pbtypes.GetString(store.PropertyIdsToSnapshots["id"].Details, bundle.RelationKeyRelationKey.String())
	assert.NotEmpty(t, details[key].GetListValue())
	assert.Len(t, details[key].GetListValue().Values, 1)
	assert.Equal(t, pbtypes.GetStringListValue(details[key])[0], "anytypeID")
}

func Test_handlePagePropertiesPeople(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"list","results":[{"object":"property_item","type":"people","id":"id","people":{"object":"user","id":"1","name":"Example","avatar_url":"https://example1.com","type":"person","person":{"email":"email1@.com"}}},{"object":"property_item","type":"people","id":"id","people":{"object":"user","id":"2","name":"Example 2","avatar_url":"https://example2.com","type":"person","person":{"email":"email2@.com"}}}],"next_cursor":null,"has_more":false,"type":"property_item","property_item":{"id":"id","next_url":null,"type":"people","people":{}}}`))
	}))
	c := client.NewClient()
	c.BasePath = s.URL
	details := make(map[string]*types.Value, 0)

	peopleProperty := property.PeopleItem{
		Object: "",
		ID:     "id",
		Type:   string(property.PropertyConfigTypePeople),
	}
	properties := property.Properties{"People": &peopleProperty}
	pageTask := Task{
		propertyService:        property.New(c),
		relationOptCreateMutex: &sync.Mutex{},
		relationCreateMutex:    &sync.Mutex{},
		p:                      Page{Properties: properties},
	}
	store := property.NewPropertiesStore()
	do := &DataObject{
		request:   &api.NotionImportContext{},
		relations: store,
		ctx:       context.Background(),
	}
	snapshots, _ := pageTask.handlePageProperties(do, details)

	assert.Len(t, snapshots, 3) // 1 relation + 1 option
	assert.Len(t, store.PropertyIdsToSnapshots, 1)
	assert.NotEmpty(t, store.PropertyIdsToSnapshots["id"])
	key := pbtypes.GetString(store.PropertyIdsToSnapshots["id"].Details, bundle.RelationKeyRelationKey.String())
	assert.NotEmpty(t, details[key])

	for _, options := range store.RelationsIdsToOptions {
		assert.Len(t, options, 2)
		assert.NotNil(t, options[0].Details)
		assert.Equal(t, options[0].Details.Fields[bundle.RelationKeyName.String()], pbtypes.String("Example"))

		assert.NotNil(t, options[1].Details)
		assert.Equal(t, options[1].Details.Fields[bundle.RelationKeyName.String()], pbtypes.String("Example 2"))
	}
}

func Test_handlePagePropertiesFormula(t *testing.T) {
	c := client.NewClient()
	details := make(map[string]*types.Value, 0)

	formulaProperty := property.FormulaItem{
		ID:      "id",
		Type:    string(property.PropertyConfigTypeFormula),
		Formula: map[string]interface{}{"type": property.NumberFormula, "number": float64(1)},
	}
	properties := property.Properties{"Formula": &formulaProperty}
	pageTask := Task{
		propertyService:        property.New(c),
		relationOptCreateMutex: &sync.Mutex{},
		relationCreateMutex:    &sync.Mutex{},
		p:                      Page{Properties: properties},
	}
	store := property.NewPropertiesStore()
	do := &DataObject{
		request:   &api.NotionImportContext{},
		relations: store,
	}
	snapshots, _ := pageTask.handlePageProperties(do, details)

	assert.Len(t, snapshots, 1) // 1 relation
	assert.Len(t, store.PropertyIdsToSnapshots, 1)
	assert.NotEmpty(t, store.PropertyIdsToSnapshots["id"])
	key := pbtypes.GetString(store.PropertyIdsToSnapshots["id"].Details, bundle.RelationKeyRelationKey.String())
	assert.NotEmpty(t, details[key])
}

func Test_handlePagePropertiesTitle(t *testing.T) {
	c := client.NewClient()
	details := make(map[string]*types.Value, 0)

	titleProperty := property.TitleItem{
		ID:    "id",
		Type:  string(property.PropertyConfigTypeTitle),
		Title: []*api.RichText{{PlainText: "Title"}},
	}
	properties := property.Properties{"Title": &titleProperty}
	pageTask := Task{
		propertyService:        property.New(c),
		relationOptCreateMutex: &sync.Mutex{},
		relationCreateMutex:    &sync.Mutex{},
		p:                      Page{Properties: properties},
	}
	store := property.NewPropertiesStore()
	do := &DataObject{
		request:   &api.NotionImportContext{},
		relations: store,
	}
	snapshots, _ := pageTask.handlePageProperties(do, details)
	assert.Len(t, snapshots, 0) // not create snapshot for existing anytype relation name
}

func Test_handleRollupProperties(t *testing.T) {
	c := client.NewClient()
	details := make(map[string]*types.Value, 0)

	rollupPropertyNumber := property.RollupItem{
		ID:   "id1",
		Type: string(property.PropertyConfigTypeRollup),
		Rollup: property.RollupObject{
			Type:   "number",
			Number: 2,
		},
	}

	rollupPropertyDate := property.RollupItem{
		ID:   "id2",
		Type: string(property.PropertyConfigTypeRollup),
		Rollup: property.RollupObject{
			Type: "date",
			Date: &api.DateObject{
				Start: "2023-02-07",
			},
		},
	}

	rollupPropertyArray := property.RollupItem{
		ID:   "id3",
		Type: string(property.PropertyConfigTypeRollup),
		Rollup: property.RollupObject{
			Type: "array",
			Array: []interface{}{
				map[string]interface{}{"type": "title", "title": []map[string]string{{"plain_text": "Title"}}},
			},
		},
	}

	properties := property.Properties{"Rollup1": &rollupPropertyNumber, "Rollup2": &rollupPropertyDate, "Rollup3": &rollupPropertyArray}

	pageTask := Task{
		propertyService:        property.New(c),
		relationOptCreateMutex: &sync.Mutex{},
		relationCreateMutex:    &sync.Mutex{},
		p:                      Page{Properties: properties},
	}
	store := property.NewPropertiesStore()
	do := &DataObject{
		request:   &api.NotionImportContext{},
		relations: store,
	}
	snapshots, _ := pageTask.handlePageProperties(do, details)

	assert.Len(t, snapshots, 3) // 3 relations
	assert.Len(t, store.PropertyIdsToSnapshots, 3)
	assert.NotEmpty(t, store.PropertyIdsToSnapshots["id1"])
	key := pbtypes.GetString(store.PropertyIdsToSnapshots["id1"].Details, bundle.RelationKeyRelationKey.String())
	assert.Equal(t, details[key].GetNumberValue(), float64(2))

	assert.NotEmpty(t, store.PropertyIdsToSnapshots["id2"])
	key = pbtypes.GetString(store.PropertyIdsToSnapshots["id2"].Details, bundle.RelationKeyRelationKey.String())
	assert.Equal(t, int(details[key].GetNumberValue()), 1675728000)

	assert.NotEmpty(t, store.PropertyIdsToSnapshots["id3"])
	key = pbtypes.GetString(store.PropertyIdsToSnapshots["id3"].Details, bundle.RelationKeyRelationKey.String())
	assert.Len(t, pbtypes.GetStringListValue(details[key]), 1)
	rollup := pbtypes.GetStringListValue(details[key])
	assert.Equal(t, rollup[0], "Title")
}

func Test_handlePagePropertiesUniqueID(t *testing.T) {
	t.Run("create relation from unique property - empty prefix", func(t *testing.T) {
		// given
		c := client.NewClient()
		details := make(map[string]*types.Value, 0)

		uniqueIDProperty := property.UniqueIDItem{
			ID:   "id",
			Type: "unique_id",
			UniqueID: property.UniqueID{
				Number: 1,
			},
		}
		properties := property.Properties{"ID": &uniqueIDProperty}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      Page{Properties: properties},
		}
		store := property.NewPropertiesStore()
		do := &DataObject{
			request:   &api.NotionImportContext{},
			relations: store,
		}

		// when
		snapshots, _ := pageTask.handlePageProperties(do, details)

		// then
		assert.Len(t, snapshots, 1)

		assert.Len(t, store.PropertyIdsToSnapshots, 1)
		assert.NotEmpty(t, store.PropertyIdsToSnapshots["id"])
		key := pbtypes.GetString(store.PropertyIdsToSnapshots["id"].Details, bundle.RelationKeyRelationKey.String())
		assert.Equal(t, details[key].GetStringValue(), "1")
	})

	t.Run("create relation from unique property - not empty prefix", func(t *testing.T) {
		// given
		c := client.NewClient()
		details := make(map[string]*types.Value, 0)

		uniqueIDProperty := property.UniqueIDItem{
			ID:   "id",
			Type: "unique_id",
			UniqueID: property.UniqueID{
				Number: 1,
				Prefix: "PR",
			},
		}
		properties := property.Properties{"ID": &uniqueIDProperty}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      Page{Properties: properties},
		}
		store := property.NewPropertiesStore()
		do := &DataObject{
			request:   &api.NotionImportContext{},
			relations: store,
		}

		// when
		snapshots, _ := pageTask.handlePageProperties(do, details)

		// then
		assert.Len(t, snapshots, 1)

		assert.Len(t, store.PropertyIdsToSnapshots, 1)
		assert.NotEmpty(t, store.PropertyIdsToSnapshots["id"])
		key := pbtypes.GetString(store.PropertyIdsToSnapshots["id"].Details, bundle.RelationKeyRelationKey.String())
		assert.Equal(t, details[key].GetStringValue(), "PR-1")
	})
}

func Test_handlePagePropertiesSelectWithTagName(t *testing.T) {
	t.Run("Page has Select property with Tag name", func(t *testing.T) {
		// given
		details := make(map[string]*types.Value, 0)
		c := client.NewClient()
		selectProperty := property.SelectItem{
			Object: "",
			ID:     "id",
			Type:   string(property.PropertyConfigTypeSelect),
			Select: property.SelectOption{
				ID:    "id",
				Name:  "Name",
				Color: api.Blue,
			},
		}
		properties := property.Properties{"Tag": &selectProperty}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      Page{Properties: properties},
		}
		req := property.NewPropertiesStore()
		do := &DataObject{
			request:   &api.NotionImportContext{},
			relations: req,
		}

		// when
		snapshots, _ := pageTask.handlePageProperties(do, details)

		// then
		assert.Len(t, snapshots, 2) // 1 relation + 1 option
		assert.Len(t, req.PropertyIdsToSnapshots, 1)
		assert.Equal(t, bundle.RelationKeyTag.String(), pbtypes.GetString(req.PropertyIdsToSnapshots[selectProperty.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
	})

	t.Run("Page has Select property with Tags name", func(t *testing.T) {
		// given
		details := make(map[string]*types.Value, 0)
		c := client.NewClient()
		selectProperty := property.SelectItem{
			Object: "",
			ID:     "id",
			Type:   string(property.PropertyConfigTypeSelect),
			Select: property.SelectOption{
				ID:    "id",
				Name:  "Name",
				Color: api.Blue,
			},
		}
		properties := property.Properties{"Tags": &selectProperty}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      Page{Properties: properties},
		}
		req := property.NewPropertiesStore()
		do := &DataObject{
			request:   &api.NotionImportContext{},
			relations: req,
		}

		// when
		snapshots, _ := pageTask.handlePageProperties(do, details)

		// then
		assert.Len(t, snapshots, 2) // 1 relation + 1 option
		assert.Len(t, req.PropertyIdsToSnapshots, 1)
		assert.Equal(t, bundle.RelationKeyTag.String(), pbtypes.GetString(req.PropertyIdsToSnapshots[selectProperty.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
	})

	t.Run("Page has MultiSelect property with Tags name", func(t *testing.T) {
		// given
		details := make(map[string]*types.Value, 0)
		c := client.NewClient()
		multiSelectProperty := property.MultiSelectItem{
			Object: "",
			ID:     "id",
			Type:   string(property.PropertyConfigTypeSelect),
			MultiSelect: []*property.SelectOption{{
				ID:    "id",
				Name:  "Name",
				Color: api.Blue,
			},
			},
		}
		properties := property.Properties{"Tags": &multiSelectProperty}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      Page{Properties: properties},
		}
		req := property.NewPropertiesStore()
		do := &DataObject{
			request:   &api.NotionImportContext{},
			relations: req,
		}

		// when
		snapshots, _ := pageTask.handlePageProperties(do, details)

		// then
		assert.Len(t, snapshots, 2) // 1 relation + 1 option
		assert.Len(t, req.PropertyIdsToSnapshots, 1)
		assert.Equal(t, bundle.RelationKeyTag.String(), pbtypes.GetString(req.PropertyIdsToSnapshots[multiSelectProperty.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
	})

	t.Run("Page has MultiSelect property with Tag name", func(t *testing.T) {
		// given
		details := make(map[string]*types.Value, 0)
		c := client.NewClient()
		multiSelectProperty := property.MultiSelectItem{
			Object: "",
			ID:     "id",
			Type:   string(property.PropertyConfigTypeSelect),
			MultiSelect: []*property.SelectOption{{
				ID:    "id",
				Name:  "Name",
				Color: api.Blue,
			},
			},
		}
		properties := property.Properties{"Tags": &multiSelectProperty}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      Page{Properties: properties},
		}
		req := property.NewPropertiesStore()
		do := &DataObject{
			request:   &api.NotionImportContext{},
			relations: req,
		}

		// when
		snapshots, _ := pageTask.handlePageProperties(do, details)

		// then
		assert.Len(t, snapshots, 2) // 1 relation + 1 option
		assert.Len(t, req.PropertyIdsToSnapshots, 1)
		assert.Equal(t, bundle.RelationKeyTag.String(), pbtypes.GetString(req.PropertyIdsToSnapshots[multiSelectProperty.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
	})

	t.Run("Page has MultiSelect property with Tag name and Select property with Tags name - MultiSelect is mapped to Tag relation", func(t *testing.T) {
		// given
		details := make(map[string]*types.Value, 0)
		c := client.NewClient()
		multiSelectProperty := property.MultiSelectItem{
			Object: "",
			ID:     "id",
			Type:   string(property.PropertyConfigTypeSelect),
			MultiSelect: []*property.SelectOption{{
				ID:    "id",
				Name:  "Name",
				Color: api.Blue,
			},
			},
		}
		selectProperty := property.SelectItem{
			Object: "",
			ID:     "id1",
			Type:   string(property.PropertyConfigTypeSelect),
			Select: property.SelectOption{
				ID:    "id1",
				Name:  "Name",
				Color: api.Blue,
			},
		}
		properties := property.Properties{"Tag": &multiSelectProperty, "Tags": &selectProperty}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      Page{Properties: properties},
		}
		req := property.NewPropertiesStore()
		do := &DataObject{
			request:   &api.NotionImportContext{},
			relations: req,
		}

		// when
		snapshots, _ := pageTask.handlePageProperties(do, details)

		// then
		assert.Len(t, snapshots, 4) // 2 relation + 2 option
		assert.Len(t, req.PropertyIdsToSnapshots, 2)
		assert.Equal(t, bundle.RelationKeyTag.String(), pbtypes.GetString(req.PropertyIdsToSnapshots[multiSelectProperty.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
		assert.NotEqual(t, bundle.RelationKeyTag.String(), pbtypes.GetString(req.PropertyIdsToSnapshots[selectProperty.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
	})

	t.Run("Page has MultiSelect property with tags name and Select property with Tag name - Tag property is mapped to Tag relation, tags is a new relation", func(t *testing.T) {
		// given
		details := make(map[string]*types.Value, 0)
		c := client.NewClient()
		multiSelectProperty := property.MultiSelectItem{
			Object: "",
			ID:     "id",
			Type:   string(property.PropertyConfigTypeSelect),
			MultiSelect: []*property.SelectOption{{
				ID:    "id",
				Name:  "Name",
				Color: api.Blue,
			},
			},
		}
		selectProperty := property.SelectItem{
			Object: "",
			ID:     "id1",
			Type:   string(property.PropertyConfigTypeSelect),
			Select: property.SelectOption{
				ID:    "id1",
				Name:  "Name",
				Color: api.Blue,
			},
		}
		properties := property.Properties{"tags": &multiSelectProperty, "Tag": &selectProperty}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      Page{Properties: properties},
		}
		req := property.NewPropertiesStore()
		do := &DataObject{
			request:   &api.NotionImportContext{},
			relations: req,
		}

		// when
		snapshots, _ := pageTask.handlePageProperties(do, details)

		// then
		assert.Len(t, snapshots, 4) // 2 relation + 2 option
		assert.Len(t, req.PropertyIdsToSnapshots, 2)
		assert.NotEqual(t, bundle.RelationKeyTag.String(), pbtypes.GetString(req.PropertyIdsToSnapshots[multiSelectProperty.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
		assert.Equal(t, bundle.RelationKeyTag.String(), pbtypes.GetString(req.PropertyIdsToSnapshots[selectProperty.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
	})

	t.Run("Page has property with empty name - return relation with name Untitled", func(t *testing.T) {
		// given
		details := make(map[string]*types.Value, 0)
		c := client.NewClient()
		selectProperty := property.SelectItem{
			Object: "",
			ID:     "id1",
			Type:   string(property.PropertyConfigTypeSelect),
			Select: property.SelectOption{},
		}
		properties := property.Properties{"": &selectProperty}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      Page{Properties: properties},
		}
		req := property.NewPropertiesStore()
		do := &DataObject{
			request:   &api.NotionImportContext{},
			relations: req,
		}

		// when
		snapshots, _ := pageTask.handlePageProperties(do, details)

		// then
		assert.Len(t, snapshots, 1) // 1 relation
		assert.Len(t, req.PropertyIdsToSnapshots, 1)
		assert.Equal(t, property.UntitledProperty, pbtypes.GetString(req.PropertyIdsToSnapshots[selectProperty.ID].GetDetails(), bundle.RelationKeyName.String()))
	})
	t.Run("Page has property which already exist - don't create new relation", func(t *testing.T) {
		// given
		details := make(map[string]*types.Value, 0)
		c := client.NewClient()
		selectProperty := property.SelectItem{
			Object: "",
			ID:     "id1",
			Type:   string(property.PropertyConfigTypeSelect),
			Select: property.SelectOption{},
		}
		properties := property.Properties{"Name": &selectProperty}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      Page{Properties: properties},
		}
		req := property.NewPropertiesStore()
		req.AddSnapshotByNameAndFormat("Name", int64(selectProperty.GetFormat()), &model.SmartBlockSnapshotBase{})
		do := &DataObject{
			request:   &api.NotionImportContext{},
			relations: req,
		}

		// when
		snapshots, _ := pageTask.handlePageProperties(do, details)

		// then
		assert.Len(t, snapshots, 0)
	})
}

func TestTask_provideDetails(t *testing.T) {
	t.Run("Page has icon emoji - details have relation iconEmoji", func(t *testing.T) {
		c := client.NewClient()
		emoji := "😘"
		page := Page{
			Icon: &api.Icon{Emoji: &emoji},
		}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      page,
		}

		// when
		details, _ := pageTask.prepareDetails()

		// then
		assert.Contains(t, details, bundle.RelationKeyIconEmoji.String())
		assert.Equal(t, emoji, details[bundle.RelationKeyIconEmoji.String()].GetStringValue())
	})
	t.Run("Page has custom external icon - details have relation iconImage", func(t *testing.T) {
		c := client.NewClient()
		page := Page{
			Icon: &api.Icon{
				Type: api.External,
				External: &api.FileProperty{
					URL: "url",
				}},
		}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      page,
		}

		// when
		details, _ := pageTask.prepareDetails()

		// then
		assert.Contains(t, details, bundle.RelationKeyIconImage.String())
		assert.Equal(t, "url", details[bundle.RelationKeyIconImage.String()].GetStringValue())
	})
	t.Run("Database has custom file icon - details have relation iconImage", func(t *testing.T) {
		c := client.NewClient()
		page := Page{
			Icon: &api.Icon{
				Type: api.File,
				File: &api.FileProperty{
					URL: "url",
				}},
		}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      page,
		}

		// when
		details, _ := pageTask.prepareDetails()

		// then
		assert.Contains(t, details, bundle.RelationKeyIconImage.String())
		assert.Equal(t, "url", details[bundle.RelationKeyIconImage.String()].GetStringValue())
	})
	t.Run("Database doesn't have icon - details don't have neither iconImage nor iconEmoji", func(t *testing.T) {
		c := client.NewClient()
		page := Page{}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      page,
		}

		// when
		details, _ := pageTask.prepareDetails()

		// then
		assert.NotContains(t, details, bundle.RelationKeyIconImage.String())
		assert.NotContains(t, details, bundle.RelationKeyIconEmoji.String())
	})
	t.Run("Page has cover - details have relation coverId and coverType", func(t *testing.T) {
		c := client.NewClient()
		page := Page{
			Cover: &api.FileObject{
				Name: "file",
				Type: api.File,
				File: api.FileProperty{
					URL: "file",
				},
			},
		}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      page,
		}

		// when
		details, _ := pageTask.prepareDetails()

		// then
		assert.Contains(t, details, bundle.RelationKeyCoverType.String())
		assert.Contains(t, details, bundle.RelationKeyCoverId.String())
		assert.Equal(t, "file", details[bundle.RelationKeyCoverId.String()].GetStringValue())
	})
	t.Run("Page doesn't have cover - details doesn't have relations coverId and coverType", func(t *testing.T) {
		c := client.NewClient()
		page := Page{}
		pageTask := Task{
			propertyService:        property.New(c),
			relationOptCreateMutex: &sync.Mutex{},
			relationCreateMutex:    &sync.Mutex{},
			p:                      page,
		}

		// when
		details, _ := pageTask.prepareDetails()

		// then
		assert.Empty(t, details[bundle.RelationKeyCoverType.String()])
		assert.Empty(t, details[bundle.RelationKeyCoverId.String()])
	})
}
