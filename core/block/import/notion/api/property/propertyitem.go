package property

// This file represent property item from Notion https://developers.notion.com/reference/property-item-object

import (
	"strconv"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type DetailSetter interface {
	SetDetail(key string, details map[string]*types.Value) 
}

type TitleItem struct {
	Object string       `json:"object"`
	ID     string       `json:"id"`
	Type   string       `json:"type"`
	Title  api.RichText `json:"title"`
}

func (t *TitleItem) SetDetail(key string, details map[string]*types.Value) {
	var title string
	if existingTitle, ok := details[bundle.RelationKeyName.String()]; ok {
		title = existingTitle.GetStringValue()
	}
	title += t.Title.PlainText
	title += "\n"
	details[bundle.RelationKeyName.String()] = pbtypes.String(title)
}

type RichTextItem struct {
	Object   string       `json:"object"`
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	RichText api.RichText `json:"rich_text"`
}

func (rt *RichTextItem) SetDetail(key string, details map[string]*types.Value) {
	var richText string
	if existingText, ok := details[key]; ok {
		richText = existingText.GetStringValue()
	}
	richText += rt.RichText.PlainText
	richText += "\n"
	details[key] = pbtypes.String(richText)
}

type NumberItem struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	Number int64  `json:"number"`
}

func (np *NumberItem) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.Int64(np.Number)
}

type SelectItem struct {
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

func (sp *SelectItem) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.StringList([]string{sp.Select.Name})
}

type MultiSelectItem struct {
	Object      string         `json:"object"`
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	MultiSelect []SelectOption `json:"multi_select"`
}

func (ms *MultiSelectItem) SetDetail(key string, details map[string]*types.Value) {
	msList := make([]string, 0)
	for _, so := range ms.MultiSelect {
		msList = append(msList, so.Name)
	}
	details[key] = pbtypes.StringList(msList)
}

//can't support it yet
type DateItem struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	Date   Date   `json:"date"`
}

func (dp *DateItem) SetDetail(key string, details map[string]*types.Value) {}

type Date struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	TimeZone string `json:"time_zone"`
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
	switch f.Formula["type"].(string) {
	case StringFormula:
		if f.Formula["string"] != nil {
			details[key] = pbtypes.String(f.Formula["string"].(string))
		}
	case NumberFormula:
		if f.Formula["number"] != nil {
			stringNumber := strconv.FormatFloat(f.Formula["number"].(float64),'f', 6, 64)
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

type RelationItem struct {
	Object   string     `json:"object"`
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Relation Relation `json:"relation"`
	HasMore  bool       `json:"has_more"`
}

type Relation struct {
	ID string `json:"id"`
}

func (r *RelationItem) SetDetail(key string, details map[string]*types.Value) {
	var (
		relation = make([]string, 0)
	)
	if rel, ok := details[key]; ok {
		existingRelation := rel.GetListValue()
		for _, v := range existingRelation.Values {
			relation = append(relation, v.GetStringValue())
		}
	}
	relation = append(relation, r.Relation.ID)
	details[key] = pbtypes.StringList(relation)
}

type PeopleItem struct {
	Object string   `json:"object"`
	ID     string   `json:"id"`
	Type   string   `json:"type"`
	People api.User `json:"people"`
}

func (p *PeopleItem) SetDetail(key string, details map[string]*types.Value) {
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

type FileItem struct {
	Object string           `json:"object"`
	ID     string           `json:"id"`
	Type   string           `json:"type"`
	File   []api.FileObject `json:"files"`
}

func (f *FileItem) SetDetail(key string, details map[string]*types.Value) {
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

func (f *FileItem) GetFormat() model.RelationFormat {
	return model.RelationFormat_file
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

type UrlItem struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	URL    string `json:"url"`
}

func (u *UrlItem) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(u.URL)
}

type EmailItem struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	Email  string `json:"email"`
}

func (e *EmailItem) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(e.Email)
}

type PhoneItem struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	Phone  string `json:"phone_number"`
}

func (p *PhoneItem) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(p.Phone)
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

type CreatedByItem struct {
	Object    string   `json:"object"`
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	CreatedBy api.User `json:"created_by"`
}

func (cb *CreatedByItem) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(cb.CreatedBy.Name)
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

type LastEditedByItem struct {
	Object       string   `json:"object"`
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	LastEditedBy api.User `json:"last_edited_by"`
}

func (lb *LastEditedByItem) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(lb.LastEditedBy.Name)
}

type StatusItem struct {
	ID     string             `json:"id"`
	Type   PropertyConfigType `json:"type"`
	Status Status             `json:"status"`
}

type Status struct {
	Name  string `json:"name,omitempty"`
	ID    string `json:"id,omitempty"`
	Color string `json:"color,omitempty"`
}

func (sp *StatusItem) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.StringList([]string{sp.Status.Name})
}

type RollupItem struct {}

func (sp *RollupItem) SetDetail(key string, details map[string]*types.Value) {}