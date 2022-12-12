package property

// This file represent property item from Notion https://developers.notion.com/reference/property-item-object

import (
	"strconv"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type ConfigType string

type Object interface {
	GetPropertyType() ConfigType
	GetID() string
	GetFormat() model.RelationFormat
}

const (
	PropertyConfigTypeTitle       ConfigType = "title"
	PropertyConfigTypeRichText    ConfigType = "rich_text"
	PropertyConfigTypeNumber      ConfigType = "number"
	PropertyConfigTypeSelect      ConfigType = "select"
	PropertyConfigTypeMultiSelect ConfigType = "multi_select"
	PropertyConfigTypeDate        ConfigType = "date"
	PropertyConfigTypePeople      ConfigType = "people"
	PropertyConfigTypeFiles       ConfigType = "files"
	PropertyConfigTypeCheckbox    ConfigType = "checkbox"
	PropertyConfigTypeURL         ConfigType = "url"
	PropertyConfigTypeEmail       ConfigType = "email"
	PropertyConfigTypePhoneNumber ConfigType = "phone_number"
	PropertyConfigTypeFormula     ConfigType = "formula"
	PropertyConfigTypeRelation    ConfigType = "relation"
	PropertyConfigTypeRollup      ConfigType = "rollup"
	PropertyConfigCreatedTime     ConfigType = "created_time"
	PropertyConfigCreatedBy       ConfigType = "created_by"
	PropertyConfigLastEditedTime  ConfigType = "last_edited_time"
	PropertyConfigLastEditedBy    ConfigType = "last_edited_by"
	PropertyConfigStatus          ConfigType = "status"
)

type DetailSetter interface {
	SetDetail(key string, details map[string]*types.Value)
}

type TitleItem struct {
	Object string          `json:"object"`
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Title  []*api.RichText `json:"title"`
}

func (t *TitleItem) SetDetail(key string, details map[string]*types.Value) {
	var richText strings.Builder
	for i, title := range t.Title {
		richText.WriteString(title.PlainText)
		if i != len(title.PlainText)-1 {
			richText.WriteString("\n")
		}
	}
	details[bundle.RelationKeyName.String()] = pbtypes.String(richText.String())
}

func (t *TitleItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeTitle
}

func (t *TitleItem) GetID() string {
	return t.ID
}

func (t *TitleItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_shorttext
}

type RichTextItem struct {
	Object   string          `json:"object"`
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	RichText []*api.RichText `json:"rich_text"`
}

func (rt *RichTextItem) SetDetail(key string, details map[string]*types.Value) {
	var richText strings.Builder
	for i, r := range rt.RichText {
		richText.WriteString(r.PlainText)
		if i != len(rt.RichText)-1 {
			richText.WriteString("\n")
		}
	}
	details[key] = pbtypes.String(richText.String())
}

func (rt *RichTextItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeRichText
}

func (rt *RichTextItem) GetID() string {
	return rt.ID
}

func (rt *RichTextItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_longtext
}

type NumberItem struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	Number *int64 `json:"number"`
}

func (np *NumberItem) SetDetail(key string, details map[string]*types.Value) {
	if np.Number != nil {
		details[key] = pbtypes.Int64(*np.Number)
	}
}

func (np *NumberItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeNumber
}

func (np *NumberItem) GetID() string {
	return np.ID
}

func (np *NumberItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_number
}

type SelectItem struct {
	Object string       `json:"object"`
	ID     string       `json:"id"`
	Type   string       `json:"type"`
	Select SelectOption `json:"select"`
}

type SelectOption struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (sp *SelectItem) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.StringList([]string{sp.Select.Name})
}

func (sp *SelectItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeSelect
}

func (sp *SelectItem) GetID() string {
	return sp.ID
}

func (sp *SelectItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_tag
}

type MultiSelectItem struct {
	Object      string          `json:"object"`
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	MultiSelect []*SelectOption `json:"multi_select"`
}

func (ms *MultiSelectItem) SetDetail(key string, details map[string]*types.Value) {
	msList := make([]string, 0)
	for _, so := range ms.MultiSelect {
		msList = append(msList, so.Name)
	}
	details[key] = pbtypes.StringList(msList)
}

func (ms *MultiSelectItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeMultiSelect
}

func (ms *MultiSelectItem) GetID() string {
	return ms.ID
}

func (ms *MultiSelectItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_tag
}

//can't support it yet
type DateItem struct {
	Object string          `json:"object"`
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Date   *api.DateObject `json:"date"`
}

func (dp *DateItem) SetDetail(key string, details map[string]*types.Value) {
	if dp.Date != nil {
		var date strings.Builder
		if dp.Date.Start != "" {
			date.WriteString(dp.Date.Start)
		}
		if dp.Date.End != "" {
			if dp.Date.Start != "" {
				date.WriteString(" ")
			}
			date.WriteString(dp.Date.End)
		}
	}
}

func (dp *DateItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeDate
}

func (dp *DateItem) GetID() string {
	return dp.ID
}

func (dp *DateItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_date
}

const (
	NumberFormula  string = "number"
	StringFormula  string = "string"
	BooleanFormula string = "boolean"
	DateFormula    string = "date"
)

type FormulaItem struct {
	Object  string                 `json:"object"`
	ID      string                 `json:"id"`
	Type    string                 `json:"type"`
	Formula map[string]interface{} `json:"formula"`
}

func (f *FormulaItem) SetDetail(key string, details map[string]*types.Value) {
	if f.Formula == nil {
		return
	}
	switch f.Formula["type"].(string) {
	case StringFormula:
		if f.Formula["string"] != nil {
			details[key] = pbtypes.String(f.Formula["string"].(string))
		}
	case NumberFormula:
		if f.Formula["number"] != nil {
			stringNumber := strconv.FormatFloat(f.Formula["number"].(float64), 'f', 6, 64)
			details[key] = pbtypes.String(stringNumber)
		}
	case BooleanFormula:
		if f.Formula["boolean"] != nil {
			stringBool := strconv.FormatBool(f.Formula["boolean"].(bool))
			details[key] = pbtypes.String(stringBool)
		}
	default:
		return
	}
}

func (f *FormulaItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeFormula
}

func (f *FormulaItem) GetID() string {
	return f.ID
}

func (f *FormulaItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_shorttext
}

type RelationItem struct {
	Object   string      `json:"object"`
	ID       string      `json:"id"`
	Type     string      `json:"type"`
	Relation []*Relation `json:"relation"`
	HasMore  bool        `json:"has_more"`
}

type Relation struct {
	ID string `json:"id"`
}

func (rp *RelationItem) SetDetail(key string, details map[string]*types.Value) {
	relation := make([]string, 0, len(rp.Relation))
	for _, rel := range rp.Relation {
		relation = append(relation, rel.ID)
	}
	details[key] = pbtypes.StringList(relation)
}

func (rp *RelationItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeRelation
}

func (rp *RelationItem) GetID() string {
	return rp.ID
}

func (rp *RelationItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_object
}

type PeopleItem struct {
	Object string      `json:"object"`
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	People []*api.User `json:"people"`
}

func (p *PeopleItem) SetDetail(key string, details map[string]*types.Value) {
	peopleList := make([]string, 0, len(p.People))
	for _, people := range p.People {
		peopleList = append(peopleList, people.Name)
	}
	details[key] = pbtypes.StringList(peopleList)
}

func (p *PeopleItem) GetPropertyType() ConfigType {
	return PropertyConfigTypePeople
}

func (p *PeopleItem) GetID() string {
	return p.ID
}

func (p *PeopleItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_tag
}

type FileItem struct {
	Object string           `json:"object"`
	ID     string           `json:"id"`
	Type   string           `json:"type"`
	File   []api.FileObject `json:"files"`
}

func (f *FileItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeFiles
}

func (f *FileItem) GetID() string {
	return f.ID
}

func (f *FileItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_file
}

func (f *FileItem) SetDetail(key string, details map[string]*types.Value) {
	fileList := make([]string, len(f.File))
	for i, fo := range f.File {
		if fo.External.URL != "" {
			fileList[i] = fo.External.URL
		} else if fo.File.URL != "" {
			fileList[i] = fo.File.URL
		}
	}
	details[key] = pbtypes.StringList(fileList)
}

type CheckboxItem struct {
	Object   string `json:"object"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Checkbox bool   `json:"checkbox"`
}

func (c *CheckboxItem) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.Bool(c.Checkbox)
}

func (c *CheckboxItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeCheckbox
}

func (c *CheckboxItem) GetID() string {
	return c.ID
}

func (c *CheckboxItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_checkbox
}

type UrlItem struct {
	Object string  `json:"object"`
	ID     string  `json:"id"`
	Type   string  `json:"type"`
	URL    *string `json:"url"`
}

func (u *UrlItem) SetDetail(key string, details map[string]*types.Value) {
	if u.URL != nil {
		details[key] = pbtypes.String(*u.URL)
	}
}

func (u *UrlItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeURL
}

func (u *UrlItem) GetID() string {
	return u.ID
}

func (u *UrlItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_url
}

type EmailItem struct {
	Object string  `json:"object"`
	ID     string  `json:"id"`
	Type   string  `json:"type"`
	Email  *string `json:"email"`
}

func (e *EmailItem) SetDetail(key string, details map[string]*types.Value) {
	if e.Email != nil {
		details[key] = pbtypes.String(*e.Email)
	}
}

func (e *EmailItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeURL
}

func (e *EmailItem) GetID() string {
	return e.ID
}

func (e *EmailItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_email
}

type PhoneItem struct {
	Object string  `json:"object"`
	ID     string  `json:"id"`
	Type   string  `json:"type"`
	Phone  *string `json:"phone_number"`
}

func (p *PhoneItem) SetDetail(key string, details map[string]*types.Value) {
	if p.Phone != nil {
		details[key] = pbtypes.String(*p.Phone)
	}
}

func (p *PhoneItem) GetPropertyType() ConfigType {
	return PropertyConfigTypePhoneNumber
}

func (p *PhoneItem) GetID() string {
	return p.ID
}

func (p *PhoneItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_phone
}

type CreatedTimeItem struct {
	Object      string `json:"object"`
	ID          string `json:"id"`
	Type        string `json:"type"`
	CreatedTime string `json:"created_time"`
}

func (ct *CreatedTimeItem) SetDetail(key string, details map[string]*types.Value) {
	t, _ := time.Parse(time.RFC3339, ct.CreatedTime)
	details[key] = pbtypes.Int64(t.Unix())
}

func (ct *CreatedTimeItem) GetPropertyType() ConfigType {
	return PropertyConfigCreatedTime
}

func (ct *CreatedTimeItem) GetID() string {
	return ct.ID
}

func (ct *CreatedTimeItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_date
}

type CreatedByItem struct {
	Object    string   `json:"object"`
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	CreatedBy api.User `json:"created_by"`
}

func (cb *CreatedByItem) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(cb.CreatedBy.Name)
}

func (cb *CreatedByItem) GetPropertyType() ConfigType {
	return PropertyConfigCreatedBy
}

func (cb *CreatedByItem) GetID() string {
	return cb.ID
}

func (cb *CreatedByItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_shorttext
}

type LastEditedTimeItem struct {
	Object         string `json:"object"`
	ID             string `json:"id"`
	Type           string `json:"type"`
	LastEditedTime string `json:"last_edited_time"`
}

func (le *LastEditedTimeItem) SetDetail(key string, details map[string]*types.Value) {
	t, _ := time.Parse(time.RFC3339, le.LastEditedTime)
	details[key] = pbtypes.Int64(t.Unix())
}

func (le *LastEditedTimeItem) GetPropertyType() ConfigType {
	return PropertyConfigLastEditedTime
}

func (le *LastEditedTimeItem) GetID() string {
	return le.ID
}

func (le *LastEditedTimeItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_date
}

type LastEditedByItem struct {
	Object       string   `json:"object"`
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	LastEditedBy api.User `json:"last_edited_by"`
}

func (lb *LastEditedByItem) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(lb.LastEditedBy.Name)
}

func (lb *LastEditedByItem) GetPropertyType() ConfigType {
	return PropertyConfigLastEditedBy
}

func (lb *LastEditedByItem) GetID() string {
	return lb.ID
}

func (lb *LastEditedByItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_shorttext
}

type StatusItem struct {
	ID     string     `json:"id"`
	Type   ConfigType `json:"type"`
	Status *Status    `json:"status"`
}

type Status struct {
	Name  string `json:"name,omitempty"`
	ID    string `json:"id,omitempty"`
	Color string `json:"color,omitempty"`
}

func (sp *StatusItem) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.StringList([]string{sp.Status.Name})
}

func (sp *StatusItem) GetPropertyType() ConfigType {
	return PropertyConfigStatus
}

func (sp *StatusItem) GetID() string {
	return sp.ID
}

func (sp *StatusItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_status
}

type RollupItem struct{}

func (r *RollupItem) SetDetail(key string, details map[string]*types.Value) {}

func (r *RollupItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeRollup
}

func (r *RollupItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_longtext
}

func (r *RollupItem) GetID() string {
	return ""
}
