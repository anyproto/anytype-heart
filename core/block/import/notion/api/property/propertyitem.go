package property

// This file represent property item from Notion https://developers.notion.com/reference/property-item-object

import (
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ConfigType string

type FormatGetter interface {
	GetFormat() model.RelationFormat
}

type IDGetter interface {
	GetID() string
}

type Object interface {
	GetPropertyType() ConfigType
	IDGetter
	FormatGetter
}

type PaginatedObject interface {
	Object
	IsEmpty() bool
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
	PropertyConfigVerification    ConfigType = "verification"
	PropertyConfigUniqueID        ConfigType = "unique_id"
)

type DetailSetter interface {
	SetDetail(key string, details *domain.Details)
}

type TitleItem struct {
	Object string          `json:"object"`
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Title  []*api.RichText `json:"title"`
}

func (t *TitleItem) GetTitle() string {
	var richText strings.Builder
	for i, ti := range t.Title {
		richText.WriteString(ti.PlainText)
		if i != len(t.Title)-1 {
			richText.WriteString("\n")
		}
	}
	return richText.String()
}

func (t *TitleItem) SetDetail(key string, details *domain.Details) {
	details.SetString(bundle.RelationKeyName, t.GetTitle())
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

func (rt *RichTextItem) IsEmpty() bool {
	return len(rt.RichText) == 0
}

func (rt *RichTextItem) SetDetail(key string, details *domain.Details) {
	var richText strings.Builder
	for i, r := range rt.RichText {
		richText.WriteString(r.PlainText)
		if i != len(rt.RichText)-1 {
			richText.WriteString("\n")
		}
	}
	details.SetString(domain.RelationKey(key), richText.String())
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
	Object string   `json:"object"`
	ID     string   `json:"id"`
	Type   string   `json:"type"`
	Number *float64 `json:"number"`
}

func (np *NumberItem) SetDetail(key string, details *domain.Details) {
	if np.Number != nil {
		details.SetFloat(domain.RelationKey(key), *np.Number)
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

func (sp *SelectItem) SetDetail(key string, details *domain.Details) {
	details.SetStringList(domain.RelationKey(key), []string{sp.Select.ID})
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

func (ms *MultiSelectItem) SetDetail(key string, details *domain.Details) {
	msList := make([]string, 0)
	for _, so := range ms.MultiSelect {
		msList = append(msList, so.ID)
	}
	details.SetStringList(domain.RelationKey(key), msList)
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

type DateItem struct {
	Object string          `json:"object"`
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Date   *api.DateObject `json:"date"`
}

func (dp *DateItem) SetDetail(key string, details *domain.Details) {
	if dp.Date != nil {
		date := common.ConvertStringToTime(dp.Date.Start)
		details.SetInt64(domain.RelationKey(key), date)
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

func (f *FormulaItem) SetDetail(key string, details *domain.Details) {
	if f.Formula == nil {
		return
	}
	switch f.Formula["type"].(string) {
	case StringFormula:
		if f.Formula["string"] != nil {
			details.SetString(domain.RelationKey(key), f.Formula["string"].(string))
		}
	case NumberFormula:
		if f.Formula["number"] != nil {
			stringNumber := strconv.FormatFloat(f.Formula["number"].(float64), 'f', 6, 64)
			details.SetString(domain.RelationKey(key), stringNumber)
		}
	case BooleanFormula:
		if f.Formula["boolean"] != nil {
			stringBool := strconv.FormatBool(f.Formula["boolean"].(bool))
			details.SetString(domain.RelationKey(key), stringBool)
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

func (rp *RelationItem) SetDetail(key string, details *domain.Details) {
	relation := make([]string, 0, len(rp.Relation))
	for _, rel := range rp.Relation {
		relation = append(relation, rel.ID)
	}
	details.SetStringList(domain.RelationKey(key), relation)
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

func (p *PeopleItem) IsEmpty() bool {
	return len(p.People) == 0
}

func (p *PeopleItem) SetDetail(key string, details *domain.Details) {
	peopleList := make([]string, 0, len(p.People))
	for _, people := range p.People {
		peopleList = append(peopleList, people.ID)
	}
	details.SetStringList(domain.RelationKey(key), peopleList)
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

func (f *FileItem) SetDetail(key string, details *domain.Details) {
	fileList := make([]string, len(f.File))
	for i, fo := range f.File {
		if fo.External.URL != "" {
			fileList[i] = fo.External.URL
		} else if fo.File.URL != "" {
			fileList[i] = fo.File.URL
		}
	}
	details.SetStringList(domain.RelationKey(key), fileList)
}

type CheckboxItem struct {
	Object   string `json:"object"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Checkbox bool   `json:"checkbox"`
}

func (c *CheckboxItem) SetDetail(key string, details *domain.Details) {
	details.SetBool(domain.RelationKey(key), c.Checkbox)
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

type URLItem struct {
	Object string  `json:"object"`
	ID     string  `json:"id"`
	Type   string  `json:"type"`
	URL    *string `json:"url"`
}

func (u *URLItem) SetDetail(key string, details *domain.Details) {
	if u.URL != nil {
		details.SetString(domain.RelationKey(key), *u.URL)
	}
}

func (u *URLItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeURL
}

func (u *URLItem) GetID() string {
	return u.ID
}

func (u *URLItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_url
}

type EmailItem struct {
	Object string  `json:"object"`
	ID     string  `json:"id"`
	Type   string  `json:"type"`
	Email  *string `json:"email"`
}

func (e *EmailItem) SetDetail(key string, details *domain.Details) {
	if e.Email != nil {
		details.SetString(domain.RelationKey(key), *e.Email)
	}
}

func (e *EmailItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeEmail
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

func (p *PhoneItem) SetDetail(key string, details *domain.Details) {
	if p.Phone != nil {
		details.SetString(domain.RelationKey(key), *p.Phone)
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

func (ct *CreatedTimeItem) SetDetail(key string, details *domain.Details) {
	t, err := time.Parse(time.RFC3339, ct.CreatedTime)
	if err != nil {
		log.With(zap.String("method", "SetDetail")).Errorf("failed to parse time %v", err)
		return
	}
	details.SetInt64(domain.RelationKey(key), t.Unix())
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

func (cb *CreatedByItem) SetDetail(key string, details *domain.Details) {
	details.SetString(domain.RelationKey(key), cb.CreatedBy.Name)
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

func (le *LastEditedTimeItem) SetDetail(key string, details *domain.Details) {
	t, err := time.Parse(time.RFC3339, le.LastEditedTime)
	if err != nil {
		log.With(zap.String("method", "SetDetail")).Errorf("failed to parse time %v", err)
		return
	}
	details.SetInt64(domain.RelationKey(key), t.Unix())
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

func (lb *LastEditedByItem) SetDetail(key string, details *domain.Details) {
	details.SetString(domain.RelationKey(key), lb.LastEditedBy.Name)
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

func (sp *StatusItem) SetDetail(key string, details *domain.Details) {
	if sp.Status != nil {
		details.SetStringList(domain.RelationKey(key), []string{sp.Status.ID})
	} else {
		details.SetStringList(domain.RelationKey(key), []string{})
	}
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

type rollupType string

const (
	rollupNumber rollupType = "number"
	rollupDate   rollupType = "date"
	rollupArray  rollupType = "array"
)

type propertyObjects []interface{}
type RollupItem struct {
	ID     string       `json:"id"`
	Type   string       `json:"type"`
	Rollup RollupObject `json:"rollup"`
}

type RollupObject struct {
	Type   rollupType      `json:"type"`
	Number float64         `json:"number"`
	Date   *api.DateObject `json:"date"`
	Array  propertyObjects `json:"array"`
}

func (r *RollupItem) SetDetail(key string, details *domain.Details) {
	switch r.Rollup.Type {
	case rollupNumber:
		details.SetFloat(domain.RelationKey(key), r.Rollup.Number)
	case rollupDate:
		di := DateItem{Date: r.Rollup.Date}
		di.SetDetail(key, details)
	case rollupArray:
		r.handleArrayType(key, details)
	}
}

func (r *RollupItem) handleArrayType(key string, details *domain.Details) {
	result := make([]string, 0)
	for _, pr := range r.Rollup.Array {
		tempDetails := domain.NewDetails()
		object := getPropertyObject(pr)
		if object == nil {
			continue
		}
		if ds, ok := object.(DetailSetter); ok {
			ds.SetDetail(key, tempDetails)
		}
		if _, ok := object.(*TitleItem); ok {
			name := tempDetails.GetString(bundle.RelationKeyName)
			result = append(result, name)
		}

		if v, ok := tempDetails.TryString(domain.RelationKey(key)); ok {
			result = append(result, v)
		} else if v, ok := tempDetails.TryBool(domain.RelationKey(key)); ok {
			result = append(result, strconv.FormatBool(v))
		} else if v, ok := tempDetails.TryFloat64(domain.RelationKey(key)); ok {
			result = append(result, strconv.FormatFloat(v, 'f', 0, 64))
		}
	}
	details.SetStringList(domain.RelationKey(key), result)
}

func (r *RollupItem) GetPropertyType() ConfigType {
	return PropertyConfigTypeRollup
}

func (r *RollupItem) GetFormat() model.RelationFormat {
	switch r.Rollup.Type {
	case rollupNumber:
		return model.RelationFormat_number
	case rollupDate:
		return model.RelationFormat_date
	case rollupArray:
		return model.RelationFormat_tag
	}
	return model.RelationFormat_longtext
}

func (r *RollupItem) GetID() string {
	return r.ID
}

type VerificationItem struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	Verification struct{} `json:"verification"`
}

func (v VerificationItem) SetDetail(_ string, _ *domain.Details) {}

func (v VerificationItem) GetPropertyType() ConfigType {
	return PropertyConfigVerification
}

func (v VerificationItem) GetID() string {
	return v.ID
}

func (v VerificationItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_date
}

type UniqueIDItem struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	UniqueID UniqueID `json:"unique_id"`
}

type UniqueID struct {
	Number int64  `json:"number"`
	Prefix string `json:"prefix"`
}

func (u UniqueIDItem) SetDetail(key string, details *domain.Details) {
	id := strconv.FormatInt(u.UniqueID.Number, 10)
	if u.UniqueID.Prefix != "" {
		id = u.UniqueID.Prefix + "-" + id
	}
	details.SetString(domain.RelationKey(key), id)
}

func (u UniqueIDItem) GetPropertyType() ConfigType {
	return PropertyConfigUniqueID
}

func (u UniqueIDItem) GetID() string {
	return u.ID
}

func (u UniqueIDItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_longtext
}
