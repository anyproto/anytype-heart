package page

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/property"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func Test_handlePagePropertiesSelect(t *testing.T) {
	details := make(map[string]*types.Value, 0)
	c := client.NewClient()
	ps := New(c)

	p := property.SelectItem{
		Object: "",
		ID:     "id",
		Type:   string(property.PropertyConfigTypeSelect),
		Select: property.SelectOption{
			ID:    "id",
			Name:  "Name",
			Color: api.Blue,
		},
	}
	pr := property.Properties{"Select": &p}
	_ = ps.handlePageProperties(context.TODO(), "key", "id", pr, details, &block.MapRequest{})

	assert.NotEmpty(t, details["Select"])
}

func Test_handlePagePropertiesLastEditedTime(t *testing.T) {
	c := client.NewClient()
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.LastEditedTimeItem{
		ID:             "id",
		Type:           string(property.PropertyConfigLastEditedTime),
		LastEditedTime: "2022-10-24T22:56:00.000Z",
	}
	pr := property.Properties{"LastEditedTime": &p}
	_ = ps.handlePageProperties(context.TODO(), "key", "id", pr, details, &block.MapRequest{})

	assert.NotEmpty(t, details["LastEditedTime"])
}

func Test_handlePagePropertiesRichText(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"list","results":[{"object":"property_item","type":"rich_text","id":"RPBv","rich_text":{"type":"text","text":{"content":"sdfsdfsdfsdfsdfsdf","link":null},"annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},"plain_text":"example text","href":null}}],"next_cursor":null,"has_more":false,"type":"property_item","property_item":{"id":"RPBv","next_url":null,"type":"rich_text","rich_text":{}}}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.RichTextItem{ID: "id", Type: string(property.PropertyConfigLastEditedTime)}
	pr := property.Properties{"RichText": &p}
	_ = ps.handlePageProperties(context.TODO(), "key", "id", pr, details, &block.MapRequest{})

	assert.NotEmpty(t, details["RichText"])
	assert.Equal(t, details["RichText"].GetStringValue(), "example text")
}

func Test_handlePagePropertiesStatus(t *testing.T) {
	c := client.NewClient()
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.StatusItem{
		ID:   "id",
		Type: property.PropertyConfigStatus,
		Status: &property.Status{
			Name:  "Done",
			ID:    "id",
			Color: api.Pink,
		},
	}
	pr := property.Properties{"Status": &p}
	_ = ps.handlePageProperties(context.TODO(), "key", "id", pr, details, &block.MapRequest{})

	assert.NotEmpty(t, details["Status"])
}

func Test_handlePagePropertiesNumber(t *testing.T) {
	c := client.NewClient()
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	num := float64(12)
	p := property.NumberItem{
		ID:     "id",
		Type:   string(property.PropertyConfigTypeNumber),
		Number: &num,
	}
	pr := property.Properties{"Number": &p}
	_ = ps.handlePageProperties(context.TODO(), "key", "id", pr, details, &block.MapRequest{})

	assert.NotEmpty(t, details["Number"])
}

func Test_handlePagePropertiesMultiSelect(t *testing.T) {
	c := client.NewClient()
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.MultiSelectItem{
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
	pr := property.Properties{"MultiSelect": &p}
	_ = ps.handlePageProperties(context.TODO(), "key", "id", pr, details, &block.MapRequest{})

	assert.NotEmpty(t, details["MultiSelect"])
}

func Test_handlePagePropertiesCheckbox(t *testing.T) {
	c := client.NewClient()
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.CheckboxItem{
		ID:       "id",
		Type:     string(property.PropertyConfigTypeCheckbox),
		Checkbox: true,
	}
	pr := property.Properties{"Checkbox": &p}
	_ = ps.handlePageProperties(context.TODO(), "key", "id", pr, details, &block.MapRequest{})

	assert.NotEmpty(t, details["Checkbox"])
}

func Test_handlePagePropertiesEmail(t *testing.T) {
	c := client.NewClient()
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	email := "a@mail.com"
	p := property.EmailItem{
		ID:    "id",
		Type:  string(property.PropertyConfigTypeEmail),
		Email: &email,
	}
	pr := property.Properties{"Email": &p}
	_ = ps.handlePageProperties(context.TODO(), "key", "id", pr, details, &block.MapRequest{})

	assert.NotEmpty(t, details["Email"])
}

func Test_handlePagePropertiesRelation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"list","results":[{"object":"property_item","type":"relation","id":"cm~~","relation":{"id":"id"}}],"next_cursor":null,"has_more":false,"type":"property_item","property_item":{"id":"cm~~","next_url":null,"type":"relation","relation":{}}}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.RelationItem{ID: "id", Type: string(property.PropertyConfigTypeRelation), HasMore: true, Relation: []*property.Relation{{ID: "id"}}}
	pr := property.Properties{"Relation": &p}
	notionPageIdsToAnytype := map[string]string{"id": "anytypeID"}
	notionDatabaseIdsToAnytype := map[string]string{"id": "anytypeID"}
	req := &block.MapRequest{NotionPageIdsToAnytype: notionPageIdsToAnytype, NotionDatabaseIdsToAnytype: notionDatabaseIdsToAnytype}
	_ = ps.handlePageProperties(context.TODO(), "key", "id", pr, details, req)

	assert.NotNil(t, details["Relation"].GetListValue())
	assert.Len(t, details["Relation"].GetListValue().Values, 1)
	assert.Equal(t, pbtypes.GetStringListValue(details["Relation"])[0], "anytypeID")
}

func Test_handlePagePropertiesPeople(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"list","results":[{"object":"property_item","type":"people","id":"id","people":{"object":"user","id":"1","name":"Example","avatar_url":"https://example1.com","type":"person","person":{"email":"email1@.com"}}},{"object":"property_item","type":"people","id":"id","people":{"object":"user","id":"2","name":"Example 2","avatar_url":"https://example2.com","type":"person","person":{"email":"email2@.com"}}}],"next_cursor":null,"has_more":false,"type":"property_item","property_item":{"id":"id","next_url":null,"type":"people","people":{}}}`))
	}))
	c := client.NewClient()
	c.BasePath = s.URL
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.PeopleItem{
		Object: "",
		ID:     "id",
		Type:   string(property.PropertyConfigTypePeople),
	}
	pr := property.Properties{"People": &p}
	_ = ps.handlePageProperties(context.TODO(), "key", "id", pr, details, &block.MapRequest{})

	assert.NotEmpty(t, details["People"])
	assert.Len(t, pbtypes.GetStringListValue(details["People"]), 2)
	people := pbtypes.GetStringListValue(details["People"])
	assert.Equal(t, people[0], "Example")
	assert.Equal(t, people[1], "Example 2")
}

func Test_handlePagePropertiesFormula(t *testing.T) {
	c := client.NewClient()
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.FormulaItem{
		ID:      "id",
		Type:    string(property.PropertyConfigTypeFormula),
		Formula: map[string]interface{}{"type": property.NumberFormula, "number": float64(1)},
	}
	pr := property.Properties{"Formula": &p}
	_ = ps.handlePageProperties(context.TODO(), "key", "id", pr, details, &block.MapRequest{})

	assert.NotEmpty(t, details["Formula"])
}

func Test_handlePagePropertiesTitle(t *testing.T) {
	c := client.NewClient()
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.TitleItem{
		ID:    "id",
		Type:  string(property.PropertyConfigTypeTitle),
		Title: []*api.RichText{{PlainText: "Title"}},
	}
	pr := property.Properties{"Title": &p}
	_ = ps.handlePageProperties(context.TODO(), "key", "id", pr, details, &block.MapRequest{})

	assert.NotEmpty(t, details["name"])
	assert.NotEmpty(t, details["name"].GetStringValue(), "Title")
}

func Test_handleRollupProperties(t *testing.T) {
	c := client.NewClient()
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p1 := property.RollupItem{
		ID:   "id1",
		Type: string(property.PropertyConfigTypeRollup),
		Rollup: property.RollupObject{
			Type:   "number",
			Number: 2,
		},
	}

	p2 := property.RollupItem{
		ID:   "id2",
		Type: string(property.PropertyConfigTypeRollup),
		Rollup: property.RollupObject{
			Type: "date",
			Date: &api.DateObject{
				Start: "12-12-2022",
			},
		},
	}

	p3 := property.RollupItem{
		ID:   "id3",
		Type: string(property.PropertyConfigTypeRollup),
		Rollup: property.RollupObject{
			Type: "array",
			Array: []interface{}{
				map[string]interface{}{"type": "title", "title": []map[string]string{{"plain_text": "Title"}}},
			},
		},
	}

	pr := property.Properties{"Rollup1": &p1, "Rollup2": &p2, "Rollup3": &p3}
	_ = ps.handlePageProperties(context.TODO(), "key", "id", pr, details, &block.MapRequest{})

	assert.NotEmpty(t, details["Rollup1"])
	assert.NotEmpty(t, details["Rollup1"].GetNumberValue(), float64(2))

	assert.NotEmpty(t, details["Rollup2"])
	assert.NotEmpty(t, details["Rollup2"].GetStringValue(), "12-12-2022")

	assert.NotEmpty(t, details["Rollup3"])
	assert.Len(t, pbtypes.GetStringListValue(details["Rollup3"]), 1)
	rollup := pbtypes.GetStringListValue(details["Rollup3"])
	assert.Equal(t, rollup[0], "Title")
}
