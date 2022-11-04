package property

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

type DetailSetter interface {
	SetDetail(key string, details map[string]*types.Value)
}

const endpoint = "/pages/%s/properties/%s"

type Title struct {
	Object string         `json:"object"`
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Title  []api.RichText `json:"title"`
}

func (t Title) SetDetail(key string, details map[string]*types.Value) {
	var title string
	for i, rt := range t.Title {
		title += rt.PlainText
		if i != len(t.Title) {
			title += "\n"
		}
	}
	details[key] = pbtypes.String(title)
}

type TitleResponse struct {
	Results    []Title `json:"results"`
	HasMore    bool    `json:"has_more"`
	NextCursor string  `json:"next_cursor"`
}

type RichText struct {
	Object   string         `json:"object"`
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	RichText []api.RichText `json:"rich_text"`
}

func (rt RichText) SetDetail(key string, details map[string]*types.Value) {
	var richText string
	for i, r := range rt.RichText {
		richText += r.PlainText
		if i != len(rt.RichText) {
			richText += "\n"
		}
	}
	details[key] = pbtypes.String(richText)
}

type RichTextResponse struct {
	Results    []RichText `json:"results"`
	HasMore    bool       `json:"has_more"`
	NextCursor string     `json:"next_cursor"`
}

type NumberProperty struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	Number int64  `json:"number"`
}

func (np NumberProperty) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.Int64(np.Number)
}

type SelectProperty struct {
	Object string       `json:"object"`
	ID     string       `json:"id"`
	Type   string       `json:"type"`
	Select SelectOption `json:"select"`
}

func (sp SelectProperty) SetDetail(key string, details map[string]*types.Value) {
	//TODO
}

type MultiSelect struct {
	Object      string         `json:"object"`
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	MultiSelect []SelectOption `json:"multi_select"`
}

func (ms MultiSelect) SetDetail(key string, details map[string]*types.Value) {
	//TODO
}

type DateProperty struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	Date   Date   `json:"date"`
}

func (dp DateProperty) SetDetail(key string, details map[string]*types.Value) {
	return
}

type Date struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	TimeZone string `json:"time_zone"`
}

type Formula struct {
	Object  string      `json:"object"`
	ID      string      `json:"id"`
	Type    string      `json:"type"`
	Formula FormulaType `json:"formula"`
}

func (f Formula) SetDetail(key string, details map[string]*types.Value) {
	switch t := f.Formula.(type) {
	case StringFormula:
		details[key] = pbtypes.String(t.String)
	case NumberFormula:
		details[key] = pbtypes.Int64(t.Number)
	case BooleanFormula:
		details[key] = pbtypes.Bool(t.Boolean)
	default:
		return
	}
}

type FormulaType interface {
	FormulaType()
}

type StringFormula struct {
	Type   string `json:"type"`
	String string `json:"string"`
}

func (StringFormula) FormulaType() {}

type NumberFormula struct {
	Type   string `json:"type"`
	Number int64  `json:"number"`
}

func (NumberFormula) FormulaType() {}

type BooleanFormula struct {
	Type    string `json:"type"`
	Boolean bool   `json:"boolean"`
}

func (BooleanFormula) FormulaType() {}

type DateFormula struct {
	Type string `json:"type"`
	Date Date   `json:"date"`
}

func (DateFormula) FormulaType() {}

type RelationProperty struct {
	Object   string   `json:"object"`
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Relation Relation `json:"relation"`
}

func (rp RelationProperty) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(rp.Relation.ID)
}

type Relation struct {
	ID string `json:"id"`
}

type RelationResponse struct {
	Results    []Relation `json:"results"`
	HasMore    bool       `json:"has_more"`
	NextCursor string     `json:"next_cursor"`
}

type Rollup struct {
	Object string `json:"object"`
}

func (r Rollup) SetDetail(key string, details map[string]*types.Value) {
	return
}

type People struct {
	Object string   `json:"object"`
	ID     string   `json:"id"`
	Type   string   `json:"type"`
	People api.User `json:"type"`
}

func (p People) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(p.People.Name)
}

type PeopleResponse struct {
	Results    []People `json:"results"`
	HasMore    bool     `json:"has_more"`
	NextCursor string   `json:"next_cursor"`
}

type File struct {
	Object string           `json:"object"`
	ID     string           `json:"id"`
	Type   string           `json:"type"`
	File   []api.FileObject `json:"files"`
}

func (f File) SetDetail(key string, details map[string]*types.Value) {
	var fileList = make([]string, len(f.File))
	for i, fo := range f.File {
		if fo.External != nil {
			fileList[i] = fo.External.URL
		} else if fo.File != nil {
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

func (c Checkbox) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.Bool(c.Checkbox)
}

type Url struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	URL    string `json:"url"`
}

func (u Url) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(u.URL)
}

type Email struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	Email  string `json:"email"`
}

func (e Email) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(e.Email)
}

type Phone struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Type   string `json:"type"`
	Phone  string `json:"phone_number"`
}

func (p Phone) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(p.Phone)
}

type CreatedTime struct {
	Object      string `json:"object"`
	ID          string `json:"id"`
	Type        string `json:"type"`
	CreatedTime string `json:"created_time"`
}

func (ct CreatedTime) SetDetail(key string, details map[string]*types.Value) {
	t, _ := time.Parse(time.RFC3339, ct.CreatedTime)
	details[key] = pbtypes.Int64(t.Unix())
}

type CreatedBy struct {
	Object    string   `json:"object"`
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	CreatedBy api.User `json:"created_by"`
}

func (cb CreatedBy) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(cb.CreatedBy.Name)
}

type LastEditedTime struct {
	Object         string `json:"object"`
	ID             string `json:"id"`
	Type           string `json:"type"`
	LastEditedTime string `json:"last_edited_time"`
}

func (le LastEditedTime) SetDetail(key string, details map[string]*types.Value) {
	t, _ := time.Parse(time.RFC3339, le.LastEditedTime)
	details[key] = pbtypes.Int64(t.Unix())
}

type LastEditedBy struct {
	Object       string   `json:"object"`
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	LastEditedBy api.User `json:"last_edited_by"`
}

type Service struct {
	client *client.Client
}

func (lb LastEditedBy) SetDetail(key string, details map[string]*types.Value) {
	details[key] = pbtypes.String(lb.LastEditedBy.Name)
}

func New(client *client.Client) *Service {
	return &Service{
		client: client,
	}
}

func (s *Service) GetPropertyObject(ctx context.Context, pageID, propertyID, apiKey string, propertyType PropertyConfigType) ([]interface{}, error) {
	var (
		hasMore           = true
		body              = &bytes.Buffer{}
		startCursor       string
		response          interface{}
		paginatedResponse = make([]interface{}, 0)
	)

	type Request struct {
		StartCursor string `json:"start_cursor,omitempty"`
	}

	for hasMore {
		err := json.NewEncoder(body).Encode(&Request{StartCursor: startCursor})

		if err != nil {
			return nil, fmt.Errorf("ListDatabases: %s", err)
		}

		request := fmt.Sprintf(endpoint, pageID, propertyID)
		req, err := s.client.PrepareRequest(ctx, apiKey, http.MethodGet, request, body)

		if err != nil {
			return nil, fmt.Errorf("ListDatabases: %s", err)
		}
		res, err := s.client.HttpClient.Do(req)

		if err != nil {
			return nil, fmt.Errorf("ListDatabases: %s", err)
		}
		defer res.Body.Close()

		b, err := ioutil.ReadAll(res.Body)

		if err != nil {
			return nil, err
		}

		if res.StatusCode != http.StatusOK {
			notionErr := client.TransformHttpCodeToError(b)
			if notionErr == nil {
				return nil, fmt.Errorf("failed http request, %d code", res.StatusCode)
			}
			return nil, notionErr
		}

		switch propertyType {
		case PropertyConfigTypeTitle:
			response = &TitleResponse{}
		case PropertyConfigTypeRichText:
			response = &RichTextResponse{}
		case PropertyConfigTypeNumber:
			response = &NumberProperty{}
		case PropertyConfigTypeSelect:
			response = &SelectProperty{}
		case PropertyConfigTypeMultiSelect:
			response = &MultiSelect{}
		case PropertyConfigTypeDate:
			response = &DateProperty{}
		case PropertyConfigTypePeople:
			response = &PeopleResponse{}
		case PropertyConfigTypeFiles:
			response = &File{}
		case PropertyConfigTypeCheckbox:
			response = &Checkbox{}
		case PropertyConfigTypeURL:
			response = &Url{}
		case PropertyConfigTypeEmail:
			response = &Email{}
		case PropertyConfigTypePhoneNumber:
			response = &Phone{}
		case PropertyConfigTypeFormula:
			response = &Formula{}
		case PropertyConfigTypeRelation:
			response = &RelationProperty{}
		case PropertyConfigTypeRollup:
			response = &Rollup{}
		case PropertyConfigCreatedTime:
			response = &CreatedTime{}
		case PropertyConfigCreatedBy:
			response = &CreatedBy{}
		case PropertyConfigLastEditedTime:
			response = &LastEditedTime{}
		case PropertyConfigLastEditedBy:
			response = &LastEditedBy{}
		default:
			return nil, fmt.Errorf("unsupported property type: %s", propertyType)
		}

		err = json.Unmarshal(b, &response)

		if err != nil {
			return nil, err
		}
		if propertyType == PropertyConfigTypeTitle {
			title := response.(TitleResponse)
			if title.HasMore {
				for _, t := range title.Results {
					paginatedResponse = append(paginatedResponse, t)
				}
				startCursor = title.NextCursor
				continue
			}
		}
		if propertyType == PropertyConfigTypeRichText {
			richText := response.(RichTextResponse)
			if richText.HasMore {
				for _, rt := range richText.Results {
					paginatedResponse = append(paginatedResponse, rt)
				}
				startCursor = richText.NextCursor
				continue
			}
		}
		if propertyType == PropertyConfigTypePeople {
			people := response.(PeopleResponse)
			if people.HasMore {
				for _, people := range people.Results {
					paginatedResponse = append(paginatedResponse, people)
				}
				startCursor = people.NextCursor
				continue
			}
		}
		if propertyType == PropertyConfigTypeRelation {
			relations := response.(RelationResponse)
			if relations.HasMore {
				for _, relations := range relations.Results {
					paginatedResponse = append(paginatedResponse, relations)
				}
				startCursor = relations.NextCursor
				continue
			}
		}
		paginatedResponse = append(paginatedResponse, response)
		hasMore = false
	}
	return paginatedResponse, nil
}

func (s *Service) SetDetailValue(key string, propertyType PropertyConfigType, property []interface{}, details map[string]*types.Value) error {
	switch propertyType {
	case PropertyConfigTypeTitle:
		for _, v := range property {
			title := v.(Title)
			title.SetDetail(key, details)
		}
	case PropertyConfigTypeRichText:
		for _, v := range property {
			rt := v.(RichText)
			rt.SetDetail(key, details)
		}
	case PropertyConfigTypeNumber:
		number := property[0].(NumberProperty)
		number.SetDetail(key, details)
	case PropertyConfigTypeSelect:
		selectProperty := property[0].(SelectProperty)
		selectProperty.SetDetail(key, details)
	case PropertyConfigTypeMultiSelect:
		multiSelect := property[0].(MultiSelect)
		multiSelect.SetDetail(key, details)
	case PropertyConfigTypeDate:
	case PropertyConfigTypePeople:
		p := property[0].(People)
		p.SetDetail(key, details)
	case PropertyConfigTypeFiles:
		f := property[0].(File)
		f.SetDetail(key, details)
	case PropertyConfigTypeCheckbox:
		c := property[0].(Checkbox)
		c.SetDetail(key, details)
	case PropertyConfigTypeURL:
		url := property[0].(Url)
		url.SetDetail(key, details)
	case PropertyConfigTypeEmail:
		email := property[0].(Email)
		email.SetDetail(key, details)
	case PropertyConfigTypePhoneNumber:
		phone := property[0].(Phone)
		phone.SetDetail(key, details)
	case PropertyConfigTypeFormula:
		formula := property[0].(Formula)
		formula.SetDetail(key, details)
	case PropertyConfigTypeRelation:
		relation := property[0].(RelationProperty)
		relation.SetDetail(key, details)
	case PropertyConfigTypeRollup:
	case PropertyConfigCreatedTime:
		ct := property[0].(CreatedTime)
		ct.SetDetail(key, details)
	case PropertyConfigCreatedBy:
		cb := property[0].(CreatedBy)
		cb.SetDetail(key, details)
	case PropertyConfigLastEditedTime:
		lt := property[0].(LastEditedTime)
		lt.SetDetail(key, details)
	case PropertyConfigLastEditedBy:
		lb := property[0].(LastEditedBy)
		lb.SetDetail(key, details)
	default:
		return fmt.Errorf("unsupported property type: %s", propertyType)
	}
	return nil
}
