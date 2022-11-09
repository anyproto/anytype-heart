package property

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
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

type PropertyObject interface {
	GetPropertyType() PropertyConfigType
	GetID() string
	GetFormat() model.RelationFormat
}

type Title struct {
	Object string      `json:"object"`
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	Title  interface{} `json:"title"`
}

func (t *Title) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeTitle
}

func (t *Title) GetID() string {
	return t.ID
}

func (t *Title) GetFormat() model.RelationFormat {
	return model.RelationFormat_shorttext
}

type RichText struct {
	Object   string      `json:"object"`
	ID       string      `json:"id"`
	Type     string      `json:"type"`
	RichText interface{} `json:"rich_text"`
}

func (rt *RichText) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeRichText
}

func (rt *RichText) GetID() string {
	return rt.ID
}

func (rt *RichText) GetFormat() model.RelationFormat {
	return model.RelationFormat_longtext
}

type NumberProperty struct {
	Object string      `json:"object"`
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	Number interface{} `json:"number"`
}

func (np *NumberProperty) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeNumber
}

func (np *NumberProperty) GetID() string {
	return np.ID
}

func (np *NumberProperty) GetFormat() model.RelationFormat {
	return model.RelationFormat_number
}

type SelectProperty struct {
	Object string      `json:"object"`
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	Select interface{} `json:"select"`
}

func (sp *SelectProperty) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeSelect
}

func (sp *SelectProperty) GetID() string {
	return sp.ID
}

func (sp *SelectProperty) GetFormat() model.RelationFormat {
	return model.RelationFormat_tag
}

type MultiSelect struct {
	Object      string      `json:"object"`
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	MultiSelect interface{} `json:"multi_select"`
}

func (ms *MultiSelect) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeMultiSelect
}

func (ms *MultiSelect) GetID() string {
	return ms.ID
}

func (ms *MultiSelect) GetFormat() model.RelationFormat {
	return model.RelationFormat_tag
}

type DateProperty struct {
	Object string      `json:"object"`
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	Date   interface{} `json:"date"`
}

func (dp *DateProperty) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeDate
}

func (dp *DateProperty) GetID() string {
	return dp.ID
}

func (dp *DateProperty) GetFormat() model.RelationFormat {
	return model.RelationFormat_date
}

type Formula struct {
	Object  string      `json:"object"`
	ID      string      `json:"id"`
	Type    string      `json:"type"`
	Formula interface{} `json:"formula"`
}

func (f *Formula) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeFormula
}

func (f *Formula) GetID() string {
	return f.ID
}

func (f *Formula) GetFormat() model.RelationFormat {
	return model.RelationFormat_shorttext
}

type RelationProperty struct {
	Object   string      `json:"object"`
	ID       string      `json:"id"`
	Type     string      `json:"type"`
	Relation interface{} `json:"relation"`
}

func (rp *RelationProperty) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeRelation
}

func (rp *RelationProperty) GetID() string {
	return rp.ID
}

func (r *RelationProperty) GetFormat() model.RelationFormat {
	return model.RelationFormat_object
}

//can't support it yet
type Rollup struct {
	Object string `json:"object"`
	ID     string `json:"id"`
}

func (r *Rollup) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeRollup
}

func (p *Rollup) GetFormat() model.RelationFormat {
	return model.RelationFormat_longtext
}

func (r *Rollup) GetID() string {
	return r.ID
}

type People struct {
	Object string      `json:"object"`
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	People interface{} `json:"people"`
}

func (p *People) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypePeople
}

func (p *People) GetID() string {
	return p.ID
}

func (p *People) GetFormat() model.RelationFormat {
	return model.RelationFormat_tag
}

type File struct {
	Object string      `json:"object"`
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	File   interface{} `json:"files"`
}

func (f *File) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeFiles
}

func (f *File) GetID() string {
	return f.ID
}

func (f *File) GetFormat() model.RelationFormat {
	return model.RelationFormat_file
}

type Checkbox struct {
	Object   string      `json:"object"`
	ID       string      `json:"id"`
	Type     string      `json:"type"`
	Checkbox interface{} `json:"checkbox"`
}

func (c *Checkbox) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeCheckbox
}

func (c *Checkbox) GetID() string {
	return c.ID
}

func (c *Checkbox) GetFormat() model.RelationFormat {
	return model.RelationFormat_checkbox
}

type Url struct {
	Object string      `json:"object"`
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	URL    interface{} `json:"url"`
}

func (u *Url) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeURL
}

func (u *Url) GetID() string {
	return u.ID
}

func (u *Url) GetFormat() model.RelationFormat {
	return model.RelationFormat_url
}

type Email struct {
	Object string      `json:"object"`
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	Email  interface{} `json:"email"`
}

func (e *Email) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeURL
}

func (e *Email) GetID() string {
	return e.ID
}

func (e *Email) GetFormat() model.RelationFormat {
	return model.RelationFormat_email
}

type Phone struct {
	Object string      `json:"object"`
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	Phone  interface{} `json:"phone_number"`
}

func (p *Phone) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypePhoneNumber
}

func (p *Phone) GetID() string {
	return p.ID
}

func (p *Phone) GetFormat() model.RelationFormat {
	return model.RelationFormat_phone
}

type CreatedTime struct {
	Object      string      `json:"object"`
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	CreatedTime interface{} `json:"created_time"`
}

func (ct *CreatedTime) GetPropertyType() PropertyConfigType {
	return PropertyConfigCreatedTime
}

func (ct *CreatedTime) GetID() string {
	return ct.ID
}

func (ct *CreatedTime) GetFormat() model.RelationFormat {
	return model.RelationFormat_date
}

type CreatedBy struct {
	Object    string      `json:"object"`
	ID        string      `json:"id"`
	Type      string      `json:"type"`
	CreatedBy interface{} `json:"created_by"`
}

func (cb *CreatedBy) GetPropertyType() PropertyConfigType {
	return PropertyConfigCreatedBy
}

func (cb *CreatedBy) GetID() string {
	return cb.ID
}

func (cb *CreatedBy) GetFormat() model.RelationFormat {
	return model.RelationFormat_shorttext
}

type LastEditedTime struct {
	Object         string      `json:"object"`
	ID             string      `json:"id"`
	Type           string      `json:"type"`
	LastEditedTime interface{} `json:"last_edited_time"`
}

func (le *LastEditedTime) GetPropertyType() PropertyConfigType {
	return PropertyConfigLastEditedTime
}

func (le *LastEditedTime) GetID() string {
	return le.ID
}

func (le *LastEditedTime) GetFormat() model.RelationFormat {
	return model.RelationFormat_date
}

type LastEditedBy struct {
	Object       string      `json:"object"`
	ID           string      `json:"id"`
	Type         string      `json:"type"`
	LastEditedBy interface{} `json:"last_edited_by"`
}

func (lb *LastEditedBy) GetPropertyType() PropertyConfigType {
	return PropertyConfigLastEditedBy
}

func (lb *LastEditedBy) GetID() string {
	return lb.ID
}

func (lb *LastEditedBy) GetFormat() model.RelationFormat {
	return model.RelationFormat_shorttext
}

type StatusProperty struct {
	ID     string             `json:"id"`
	Type   PropertyConfigType `json:"type"`
	Status interface{}        `json:"status"`
}

func (sp *StatusProperty) GetPropertyType() PropertyConfigType {
	return PropertyConfigStatus
}

func (sp *StatusProperty) GetID() string {
	return sp.ID
}

func (sp *StatusProperty) GetFormat() model.RelationFormat {
	return model.RelationFormat_status
}
