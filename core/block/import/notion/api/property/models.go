package property

import (
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

type PropertyConfigType string

const (
	PropertyConfigTypeTitle       PropertyConfigType = "title"
	PropertyConfigTypeRichText    PropertyConfigType = "rich_text"
	PropertyConfigTypeNumber      PropertyConfigType = "number"
	PropertyConfigTypeSelect      PropertyConfigType = "select"
	PropertyConfigTypeMultiSelect PropertyConfigType = "multi_select"
	PropertyConfigTypeDate        PropertyConfigType = "date"
	PropertyConfigTypePeople      PropertyConfigType = "people"
	PropertyConfigTypeFiles       PropertyConfigType = "files"
	PropertyConfigTypeCheckbox    PropertyConfigType = "checkbox"
	PropertyConfigTypeURL         PropertyConfigType = "url"
	PropertyConfigTypeEmail       PropertyConfigType = "email"
	PropertyConfigTypePhoneNumber PropertyConfigType = "phone_number"
	PropertyConfigTypeFormula     PropertyConfigType = "formula"
	PropertyConfigTypeRelation    PropertyConfigType = "relation"
	PropertyConfigTypeRollup      PropertyConfigType = "rollup"
	PropertyConfigCreatedTime     PropertyConfigType = "created_time"
	PropertyConfigCreatedBy       PropertyConfigType = "created_by"
	PropertyConfigLastEditedTime  PropertyConfigType = "last_edited_time"
	PropertyConfigLastEditedBy    PropertyConfigType = "last_edited_by"
	PropertyConfigStatus          PropertyConfigType = "status"
)

type Setter interface {
	SetDetail(key string, details map[string]*types.Value)
}

type PropertyObject interface {
	GetPropertyType() PropertyConfigType
	GetID() string
}

type Title struct {
	Object string         `json:"object"`
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Title  api.RichText `json:"title"`
}

func (t *Title) SetDetail(key string, details map[string]*types.Value) {
	var title string
	if existingTitle, ok := details[key]; ok {
		title = existingTitle.GetStringValue()
	}
	title += t.Title.PlainText
	title += "\n"
	details[key] = pbtypes.String(title)
}

func (t *Title) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeTitle
}

func (t *Title) GetID() string {
	return t.ID
}

type RichText struct {
	Object   string         `json:"object"`
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	RichText api.RichText `json:"rich_text"`
}

func (rt *RichText) SetDetail(key string, details map[string]*types.Value) {
	var richText string
	if existingText, ok := details[key]; ok {
		richText = existingText.GetStringValue()
	}
	richText += rt.RichText.PlainText
	richText += "\n"
	details[key] = pbtypes.String(richText)
}

func (rt *RichText) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeRichText
}

func (rt *RichText) GetID() string {
	return rt.ID
}
type NumberProperty struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	Number int64  `json:"number"`
}

func (np *NumberProperty) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.Int64(np.Number)
}

func (np *NumberProperty) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeNumber
}

func (np *NumberProperty) GetID() string {
	return np.ID
}

type SelectProperty struct {
	Object string       `json:"object"`
	ID     string       `json:"id"`
	Type   string       `json:"type"`
	Select SelectOption `json:"select"`
}

type SelectOption struct {
	ID    string    `json:"id,omitempty"`
	Name  string    `json:"name"`
	Color api.Color `json:"color"`
}

func (sp *SelectProperty) SetDetail(key string, details map[string]*types.Value) {
	//TODO
}

func (sp *SelectProperty) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeSelect
}

func (sp *SelectProperty) GetID() string {
	return sp.ID
}

type Select struct {
	Options []SelectOption `json:"options"`
}

type MultiSelect struct {
	Object      string         `json:"object"`
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	MultiSelect []SelectOption `json:"multi_select"`
}

func (ms *MultiSelect) SetDetail(key string, details map[string]*types.Value) {
	//TODO
}

func (ms *MultiSelect) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeMultiSelect
}

func (ms *MultiSelect) GetID() string {
	return ms.ID
}

type DateProperty struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	Date   Date   `json:"date"`
}

func (dp *DateProperty) SetDetail(key string, details map[string]*types.Value) {
	return
}

func (dp *DateProperty) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeDate
}

func (dp *DateProperty) GetID() string {
	return dp.ID
}

type Date struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	TimeZone string `json:"time_zone"`
}

const (
	NumberFormula string = "number"
	StringFormula string = "string"
	BooleanFormula string = "boolean"
	DateFormula string = "date"
)

type Formula struct {
	Object  string      `json:"object"`
	ID      string      `json:"id"`
	Type    string      `json:"type"`
	Formula map[string]interface{} `json:"formula"`
}

func (f *Formula) SetDetail(key string, details map[string]*types.Value) {
	switch f.Formula["type"].(string) {
	case StringFormula:
		details[key] = pbtypes.String(f.Formula["string"].(string))
	case NumberFormula:
		details[key] = pbtypes.Float64(f.Formula["number"].(float64))
	case BooleanFormula:
		details[key] = pbtypes.Bool(f.Formula["boolean"].(bool))
	default:
		return
	}
}

func (f *Formula) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeFormula
}

func (f *Formula) GetID() string {
	return f.ID
}

type RelationProperty struct {
	Object   string   `json:"object"`
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Relation []Relation `json:"relation"`
	HasMore  bool       `json:"has_more"`
}

type Relation struct {
	ID string `json:"id"`
}

func (r *Relation) SetDetail(key string, details map[string]*types.Value) {
	var (
		relation string
		space string
	)
	if existingRelation, ok := details[key]; ok {
		relation = existingRelation.GetStringValue()
	}
	if relation != "" {
		space = " "
	}
	details[key] = pbtypes.String(relation + space + r.ID)
}

func (rp *RelationProperty) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeRelation
}

func (rp *RelationProperty) GetID() string {
	return rp.ID
}

//can't support it yet
type Rollup struct {
	Object string `json:"object"`
}

func (r *Rollup) SetDetail(key string, details map[string]*types.Value) {}

func (r *Rollup) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeRollup
}

func (r *Rollup) GetID() string {
	return ""
}

type People struct {
	Object string   `json:"object"`
	ID     string   `json:"id"`
	Type   string   `json:"type"`
	People api.User `json:"people"`
}

func (p *People) SetDetail(key string, details map[string]*types.Value) {
	var peopleList = make([]string, 0)
	if existingPeople, ok := details[key]; ok {
		list := existingPeople.GetListValue()
		for _, v := range list.Values {
			peopleList = append(peopleList, v.GetStringValue())
		}
	}
	peopleList = append(peopleList, p.People.Name)
	details[key] = pbtypes.StringList(peopleList)
}

func (p *People) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypePeople
}

func (p *People) GetID() string {
	return p.ID
}

type File struct {
	Object string           `json:"object"`
	ID     string           `json:"id"`
	Type   string           `json:"type"`
	File   []api.FileObject `json:"files"`
}

func (f *File) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeFiles
}

func (f *File) GetID() string {
	return f.ID
}

func (f *File) SetDetail(key string, details map[string]*types.Value) {
	var fileList = make([]string, len(f.File))
	for i, fo := range f.File {
		if fo.External.URL != "" {
			fileList[i] = fo.External.URL
		} else if fo.File.URL != "" {
			fileList[i] = fo.File.URL
		}
	}
	details[key] = pbtypes.StringList(fileList)
}

type Checkbox struct {
	Object   string `json:"object"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Checkbox bool   `json:"checkbox"`
}

func (c *Checkbox) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.Bool(c.Checkbox)
}

func (c *Checkbox) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeFiles
}

func (c *Checkbox) GetID() string {
	return c.ID
}

type Url struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	URL    string `json:"url"`
}

func (u *Url) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(u.URL)
}

func (u *Url) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeURL
}

func (u *Url) GetID() string {
	return u.ID
}

type Email struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	Email  string `json:"email"`
}

func (e *Email) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(e.Email)
}

func (e *Email) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeURL
}

func (e *Email) GetID() string {
	return e.ID
}

type Phone struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	Phone  string `json:"phone_number"`
}

func (p *Phone) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(p.Phone)
}

func (p *Phone) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypePhoneNumber
}

func (p *Phone) GetID() string {
	return p.ID
}

type CreatedTime struct {
	Object      string `json:"object"`
	ID          string `json:"id"`
	Type        string `json:"type"`
	CreatedTime string `json:"created_time"`
}

func (ct *CreatedTime) SetDetail(key string, details map[string]*types.Value) {
	t, _ := time.Parse(time.RFC3339, ct.CreatedTime)
	details[key] = pbtypes.Int64(t.Unix())
}

func (ct *CreatedTime) GetPropertyType() PropertyConfigType {
	return PropertyConfigCreatedTime
}

func (ct *CreatedTime) GetID() string {
	return ct.ID
}

type CreatedBy struct {
	Object    string   `json:"object"`
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	CreatedBy api.User `json:"created_by"`
}

func (cb *CreatedBy) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(cb.CreatedBy.Name)
}

func (cb *CreatedBy) GetPropertyType() PropertyConfigType {
	return PropertyConfigCreatedBy
}

func (cb *CreatedBy) GetID() string {
	return cb.ID
}

type LastEditedTime struct {
	Object         string `json:"object"`
	ID             string `json:"id"`
	Type           string `json:"type"`
	LastEditedTime string `json:"last_edited_time"`
}

func (le *LastEditedTime) SetDetail(key string, details map[string]*types.Value) {
	t, _ := time.Parse(time.RFC3339, le.LastEditedTime)
	details[key] = pbtypes.Int64(t.Unix())
}

func (le *LastEditedTime) GetPropertyType() PropertyConfigType {
	return PropertyConfigLastEditedTime
}

func (le *LastEditedTime) GetID() string {
	return le.ID
}

type LastEditedBy struct {
	Object       string   `json:"object"`
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	LastEditedBy api.User `json:"last_edited_by"`
}


func (lb *LastEditedBy) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(lb.LastEditedBy.Name)
}

func (lb *LastEditedBy) GetPropertyType() PropertyConfigType {
	return PropertyConfigLastEditedBy
}

func (lb *LastEditedBy) GetID() string {
	return lb.ID
}

type StatusProperty struct {
	ID           string             `json:"id"`
	Type         PropertyConfigType `json:"type"`
	Status 		 Status `json:"status"` 
}

type Status struct {
	Name  string `json:"name,omitempty"`  
	ID    string `json:"id,omitempty"`    
	Color string `json:"color,omitempty"` 
}

func (sp *StatusProperty) GetPropertyType() PropertyConfigType {
	return PropertyConfigStatus
}

func (sp *StatusProperty) GetID() string {
	return sp.ID
}

func (sp *StatusProperty) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.StringList([]string{sp.Status.Name})
}