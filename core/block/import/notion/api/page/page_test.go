package page

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/property"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
)

func Test_handlePagePropertiesSelect(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"property_item","type":"select","id":"C%5E%7DO","select":{"id":"f56c757b-eb58-4a15-b528-055a0e3e85b4","name":"dddd","color":"red"}}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.SelectProperty{ID:"id", Type: string(property.PropertyConfigTypeSelect)}
	pr := property.Properties{"Select": &p}
	_, ce := ps.handlePageProperties("key", "id", pr, details, pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	assert.Nil(t, ce)
	assert.NotEmpty(t, details["Select"])
}

func Test_handlePagePropertiesLastEditedTime(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"property_item","type":"last_edited_time","id":"MeQJ","last_edited_time":"2022-11-04T13:02:00.000Z"}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.LastEditedTime{ID: "id", Type: string(property.PropertyConfigLastEditedTime)}
	pr := property.Properties{"LastEditedTime": &p}
	_, ce := ps.handlePageProperties("key", "id", pr, details, pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	assert.Nil(t, ce)
	assert.NotEmpty(t, details["LastEditedTime"])
}

func Test_handlePagePropertiesRichText(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"list","results":[{"object":"property_item","type":"rich_text","id":"RPBv","rich_text":{"type":"text","text":{"content":"sdfsdfsdfsdfsdfsdf","link":null},"annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},"plain_text":"sdfsdfsdfsdfsdfsdf","href":null}}],"next_cursor":null,"has_more":false,"type":"property_item","property_item":{"id":"RPBv","next_url":null,"type":"rich_text","rich_text":{}}}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.RichText{ID: "id", Type: string(property.PropertyConfigLastEditedTime)}
	pr := property.Properties{"RichText": &p}
	_, ce := ps.handlePageProperties("key", "id", pr, details, pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	assert.Nil(t, ce)
	assert.NotEmpty(t, details["RichText"])
}

func Test_handlePagePropertiesStatus(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"property_item","type":"status","id":"VwSP","status":{"id":"e4927bd2-4580-4e37-9095-eb0af45923bc","name":"In progress","color":"blue"}}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.StatusProperty{ID: "id", Type: property.PropertyConfigStatus}
	pr := property.Properties{"Status": &p}
	_, ce := ps.handlePageProperties("key", "id", pr, details, pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	assert.Nil(t, ce)
	assert.NotEmpty(t, details["Status"])
}

func Test_handlePagePropertiesNumber(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"property_item","type":"number","id":"WxBc","number":3434}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.NumberProperty{ID: "id", Type: string(property.PropertyConfigTypeNumber)}
	pr := property.Properties{"Number": &p}
	_, ce := ps.handlePageProperties("key", "id", pr, details, pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	assert.Nil(t, ce)
	assert.NotEmpty(t, details["Number"])
}

func Test_handlePagePropertiesMultiSelect(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"property_item","type":"multi_select","id":"%5D%60%3FX","multi_select":[{"id":"EgfK","name":"ddd","color":"default"},{"id":"QO[c","name":"AAA","color":"purple"},{"id":"UsL>","name":"Option","color":"orange"}]}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.NumberProperty{ID: "id", Type: string(property.PropertyConfigTypeMultiSelect)}
	pr := property.Properties{"MultiSelect": &p}
	_, ce := ps.handlePageProperties("key", "id", pr, details, pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	assert.Nil(t, ce)
	assert.NotEmpty(t, details["MultiSelect"])
}

func Test_handlePagePropertiesCheckbox(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"property_item","type":"checkbox","id":"%60%3C%3DZ","checkbox":true}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.Checkbox{ID: "id", Type: string(property.PropertyConfigTypeCheckbox)}
	pr := property.Properties{"Checkbox": &p}
	_, ce := ps.handlePageProperties("key", "id", pr, details, pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	assert.Nil(t, ce)
	assert.NotEmpty(t, details["Checkbox"])
}

func Test_handlePagePropertiesEmail(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"property_item","type":"email","id":"bQRa","email":null}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.Email{ID: "id", Type: string(property.PropertyConfigTypeEmail)}
	pr := property.Properties{"Email": &p}
	_, ce := ps.handlePageProperties("key", "id", pr, details, pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	assert.Nil(t, ce)
	assert.NotEmpty(t, details["Email"])
}

func Test_handlePagePropertiesRelation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"list","results":[{"object":"property_item","type":"relation","id":"cm~~","relation":{"id":"18e660df-d7f4-4d4b-b30c-eeb88ffee645"}}],"next_cursor":null,"has_more":false,"type":"property_item","property_item":{"id":"cm~~","next_url":null,"type":"relation","relation":{}}}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.RelationProperty{ID: "id", Type: string(property.PropertyConfigTypeRelation)}
	pr := property.Properties{"Relation": &p}
	_, ce := ps.handlePageProperties("key", "id", pr, details, pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	assert.Nil(t, ce)
	assert.NotEmpty(t, details["Relation"])
}

func Test_handlePagePropertiesPeople(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"list","results":[{"object":"property_item","type":"people","id":"nWZg","people":{"object":"user","id":"60faafc6-0c5c-4479-a3f7-67d77cd8a56d"}}],"next_cursor":null,"has_more":false,"type":"property_item","property_item":{"id":"nWZg","next_url":null,"type":"people","people":{}}}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.People{ID: "id", Type: string(property.PropertyConfigTypePeople)}
	pr := property.Properties{"People": &p}
	_, ce := ps.handlePageProperties("key", "id", pr, details, pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	assert.Nil(t, ce)
	assert.NotEmpty(t, details["People"])
}

func Test_handlePagePropertiesFormula(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"property_item","type":"formula","id":"%7Do%40%7B","formula":{"type":"number","number":11.745674324002}}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.Formula{ID: "id", Type: string(property.PropertyConfigTypeFormula)}
	pr := property.Properties{"Formula": &p}
	_, ce := ps.handlePageProperties("key", "id", pr, details, pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	assert.Nil(t, ce)
	assert.NotEmpty(t, details["Formula"])
}

func Test_handlePagePropertiesTitle(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"list","results":[{"object":"property_item","type":"title","id":"title","title":{"type":"text","text":{"content":"Daily Entry","link":null},"annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},"plain_text":"Daily Entry","href":null}}],"next_cursor":null,"has_more":false,"type":"property_item","property_item":{"id":"title","next_url":null,"type":"title","title":{}}}`))
	}))

	c := client.NewClient()
	c.BasePath = s.URL
	ps := New(c)

	details := make(map[string]*types.Value, 0)

	p := property.Title{ID: "id", Type: string(property.PropertyConfigTypeTitle)}
	pr := property.Properties{"Title": &p}
	_, ce := ps.handlePageProperties("key", "id", pr, details, pb.RpcObjectImportRequest_ALL_OR_NOTHING)

	assert.Nil(t, ce)
	assert.NotEmpty(t, details["name"])
}