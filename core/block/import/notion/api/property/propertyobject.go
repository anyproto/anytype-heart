package property

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

var logger = logging.Logger("notion-property-retriever")

const endpoint = "/pages/%s/properties/%s"
type Service struct {
	client *client.Client
}

func New(client *client.Client) *Service {
	return &Service{
		client: client,
	}
}

type Properties map[string]PropertyObject

func (p *Properties) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	props, err := parsePropertyConfigs(raw)
	if err != nil {
		return err
	}

	*p = props
	return nil
}

func parsePropertyConfigs(raw map[string]interface{}) (Properties, error) {
	result := make(Properties)
	for k, v := range raw {
		var p PropertyObject
		switch rawProperty := v.(type) {
		case map[string]interface{}:
			switch PropertyConfigType(rawProperty["type"].(string)) {
			case PropertyConfigTypeTitle:
				p = &Title{}
			case PropertyConfigTypeRichText:
				p = &RichText{}
			case PropertyConfigTypeNumber:
				p = &NumberProperty{}
			case PropertyConfigTypeSelect:
				p = &SelectProperty{}
			case PropertyConfigTypeMultiSelect:
				p = &MultiSelect{}
			case PropertyConfigTypeDate:
				p = &DateProperty{}
			case PropertyConfigTypePeople:
				p = &People{}
			case PropertyConfigTypeFiles:
				p = &File{}
			case PropertyConfigTypeCheckbox:
				p = &Checkbox{}
			case PropertyConfigTypeURL:
				p = &Url{}
			case PropertyConfigTypeEmail:
				p = &Email{}
			case PropertyConfigTypePhoneNumber:
				p = &Phone{}
			case PropertyConfigTypeFormula:
				p = &Formula{}
			case PropertyConfigTypeRelation:
				p = &RelationProperty{}
			case PropertyConfigTypeRollup:
				p = &Rollup{}
			case PropertyConfigCreatedTime:
				p = &CreatedTime{}
			case PropertyConfigCreatedBy:
				p = &CreatedBy{}
			case PropertyConfigLastEditedTime:
				p = &LastEditedTime{}
			case PropertyConfigLastEditedBy:
				p = &LastEditedBy{}
			case PropertyConfigStatus:
				p = &StatusProperty{}
			default:
				return nil, fmt.Errorf("unsupported property type: %s", rawProperty["type"].(string))
			}
			b, err := json.Marshal(rawProperty)
			if err != nil {
				return nil, err
			}

			if err = json.Unmarshal(b, &p); err != nil {
				return nil, err
			}

			result[k] = p
		default:
			return nil, fmt.Errorf("unsupported property format %T", v)
		}
	}

	return result, nil
}

type PropertyPaginatedRespone struct{
	Object       string   `json:"object"`
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	Results      []interface{}   `json:"results"`
	Item         interface{} `json:"property_item"`
	HasMore      bool       `json:"has_more"`
	NextCursor   string     `json:"next_cursor"`
}

func (s *Service) GetPropertyObject(ctx context.Context, pageID, propertyID, apiKey string, propertyType PropertyConfigType) ([]interface{}, error) {
	var (
		hasMore           = true
		body              = &bytes.Buffer{}
		startCursor       string
		response          PropertyPaginatedRespone
		paginatedResponse = make([]interface{}, 0)
	)

	type Request struct {
		StartCursor string `json:"start_cursor,omitempty"`
	}

	for hasMore {
		err := json.NewEncoder(body).Encode(&Request{StartCursor: startCursor})

		if err != nil {
			return nil, fmt.Errorf("GetPropertyObject: %s", err)
		}

		request := fmt.Sprintf(endpoint, pageID, propertyID)
		req, err := s.client.PrepareRequest(ctx, apiKey, http.MethodGet, request, body)

		if err != nil {
			return nil, fmt.Errorf("GetPropertyObject: %s", err)
		}
		res, err := s.client.HttpClient.Do(req)

		if err != nil {
			return nil, fmt.Errorf("GetPropertyObject: %s", err)
		}
		defer res.Body.Close()

		b, err := ioutil.ReadAll(res.Body)

		if err != nil {
			return nil, err
		}

		if res.StatusCode != http.StatusOK {
			notionErr := client.TransformHttpCodeToError(b)
			if notionErr == nil {
				return nil, fmt.Errorf("GetPropertyObject: failed http request, %d code", res.StatusCode)
			}
			return nil, notionErr
		}

		switch propertyType {
		case PropertyConfigTypeTitle, PropertyConfigTypeRichText, PropertyConfigTypeRelation, PropertyConfigTypePeople:
			err = json.Unmarshal(b, &response)
			if err != nil {
				continue
			}
			res := response.Results
			for _, v := range res {
				buffer, err := json.Marshal(v)
				if err != nil {
					logger.Errorf("GetPropertyObject: failed to marshal: %s", err)
					continue
				}
				if propertyType == PropertyConfigTypeTitle {
					p := TitleItem{}
					err = json.Unmarshal(buffer, &p)
					if err != nil { 
						logger.Errorf("GetPropertyObject: failed to marshal TitleItem: %s", err)
						continue
					}
					paginatedResponse = append(paginatedResponse, p)
				}
				if propertyType == PropertyConfigTypeRichText {
					p := RichTextItem{}
					err = json.Unmarshal(buffer, &p)
					if err != nil { 
						logger.Errorf("GetPropertyObject: failed to marshal RichTextItem: %s", err)
						continue
					}
					paginatedResponse = append(paginatedResponse, p)
				}
				if propertyType == PropertyConfigTypeRelation {
					p := RelationItem{}
					err = json.Unmarshal(buffer, &p)
					if err != nil { 
						logger.Errorf("GetPropertyObject: failed to marshal RelationItem: %s", err)
						continue
					}
					paginatedResponse = append(paginatedResponse, p)
				}
				if propertyType == PropertyConfigTypePeople {
					p := PeopleItem{}
					err = json.Unmarshal(buffer, &p)
					if err != nil { 
						logger.Errorf("GetPropertyObject: failed to marshal PeopleItem: %s", err)
						continue
					}
					paginatedResponse = append(paginatedResponse, p)
				}
			}
			if response.HasMore {
				startCursor = response.NextCursor
				continue
			}
		case PropertyConfigTypeNumber:
			p := NumberItem{}
			err = json.Unmarshal(b, &p)
			if err != nil { 
				logger.Errorf("GetPropertyObject: failed to marshal NumberItem: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, p)
		case PropertyConfigTypeSelect:
			p := SelectItem{}
			err = json.Unmarshal(b, &p)
			if err != nil { 
				logger.Errorf("GetPropertyObject: failed to marshal SelectItem: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, p)
		case PropertyConfigTypeMultiSelect:
			p := MultiSelectItem{}
			err = json.Unmarshal(b, &p)
			if err != nil { 
				logger.Errorf("GetPropertyObject: failed to marshal MultiSelectItem: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, p)
		case PropertyConfigTypeDate:
			date := DateItem{}
			err = json.Unmarshal(b, &date)
			if err != nil { 
				logger.Errorf("GetPropertyObject: failed to marshal DateItem: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, date)
		case PropertyConfigTypeFiles:
			file := FileItem{}
			err = json.Unmarshal(b, &file)
			if err != nil { 
				logger.Errorf("GetPropertyObject: failed to marshal FileItem: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, file)
		case PropertyConfigTypeCheckbox:
			checkbox := CheckboxItem{}
			err = json.Unmarshal(b, &checkbox)
			if err != nil { 
				logger.Errorf("GetPropertyObject: failed to marshal CheckboxItem: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, checkbox)
		case PropertyConfigTypeURL:
			url := UrlItem{}
			err = json.Unmarshal(b, &url)
			if err != nil { 
				logger.Errorf("GetPropertyObject: failed to marshal UrlItem: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, url)
		case PropertyConfigTypeEmail:
			email := EmailItem{}
			err = json.Unmarshal(b, &email)
			if err != nil { 
				logger.Errorf("GetPropertyObject: failed to marshal EmailItem: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, email)
		case PropertyConfigTypePhoneNumber:
			phone := PhoneItem{}
			err = json.Unmarshal(b, &phone)
			if err != nil { 
				logger.Errorf("GetPropertyObject: failed to marshal PhoneItem: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, phone)
		case PropertyConfigTypeFormula:
			formula := FormulaItem{}
			err = json.Unmarshal(b, &formula)
			if err != nil { 
				logger.Errorf("GetPropertyObject: failed to marshal FormulaItem: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, formula)
		case PropertyConfigTypeRollup:
			rollup := Rollup{}
			err = json.Unmarshal(b, &rollup)
			if err != nil { 
				logger.Errorf("GetPropertyObject: failed to marshal Rollup: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, rollup)
		case PropertyConfigCreatedTime:
			ct := CreatedTimeItem{}
			err = json.Unmarshal(b, &ct)
			if err != nil { 
				logger.Errorf("GetPropertyObject: failed to marshal CreatedTimeItem: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, ct)
		case PropertyConfigCreatedBy:
			cb := CreatedByItem{}
			err = json.Unmarshal(b, &cb)
			if err != nil { 
				logger.Errorf("GetPropertyObject: failed to marshal CreatedByItem: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, cb)
		case PropertyConfigLastEditedTime:
			lt := LastEditedTimeItem{}
			err = json.Unmarshal(b, &lt)
			if err != nil { 
				logger.Errorf("GetPropertyObject: failed to marshal LastEditedTimeItem: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, lt)
		case PropertyConfigLastEditedBy:
			le := LastEditedByItem{}
			err = json.Unmarshal(b, &le)
			if err != nil {
				logger.Errorf("GetPropertyObject: failed to marshal LastEditedByItem: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, le)
		case PropertyConfigStatus:
			sp := StatusItem{}
			err = json.Unmarshal(b, &sp)
			if err != nil {
				logger.Errorf("GetPropertyObject: failed to marshal StatusItem: %s", err)
				continue
			}
			paginatedResponse = append(paginatedResponse, sp)
		default:
			return nil, fmt.Errorf("GetPropertyObject: unsupported property type: %s", propertyType)
		}
		hasMore = false
	}
	return paginatedResponse, nil
}
