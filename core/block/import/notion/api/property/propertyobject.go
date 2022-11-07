package property

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
)

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
		case PropertyConfigTypeTitle, PropertyConfigTypeRichText, PropertyConfigTypeRelation, PropertyConfigTypePeople:
			err = json.Unmarshal(b, &response)
			if err != nil {
				continue
			}
			res := response.Results
			for _, v := range res {
				buffer, err := json.Marshal(v)
				if err != nil {
					continue
				}
				if propertyType == PropertyConfigTypeTitle {
					p := Title{}
					err = json.Unmarshal(buffer, &p)
					if err != nil { 
						continue
					}
					paginatedResponse = append(paginatedResponse, p)
				}
				if propertyType == PropertyConfigTypeRichText {
					p := RichText{}
					err = json.Unmarshal(buffer, &p)
					if err != nil { 
						continue
					}
					paginatedResponse = append(paginatedResponse, p)
				}
				if propertyType == PropertyConfigTypeRelation {
					p := Relation{}
					err = json.Unmarshal(buffer, &p)
					if err != nil { 
						continue
					}
					paginatedResponse = append(paginatedResponse, p)
				}
				if propertyType == PropertyConfigTypePeople {
					p := People{}
					err = json.Unmarshal(buffer, &p)
					if err != nil { 
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
			p := NumberProperty{}
			err = json.Unmarshal(b, &p)
			if err != nil { 
				continue
			}
			paginatedResponse = append(paginatedResponse, p)
		case PropertyConfigTypeSelect:
			p := SelectProperty{}
			err = json.Unmarshal(b, &p)
			if err != nil { 
				continue
			}
			paginatedResponse = append(paginatedResponse, p)
		case PropertyConfigTypeMultiSelect:
			p := MultiSelect{}
			err = json.Unmarshal(b, &p)
			if err != nil { 
				continue
			}
			paginatedResponse = append(paginatedResponse, p)
		case PropertyConfigTypeDate:
			date := DateProperty{}
			err = json.Unmarshal(b, &date)
			if err != nil { 
				continue
			}
			paginatedResponse = append(paginatedResponse, date)
		case PropertyConfigTypeFiles:
			file := File{}
			err = json.Unmarshal(b, &file)
			if err != nil { 
				continue
			}
			paginatedResponse = append(paginatedResponse, file)
		case PropertyConfigTypeCheckbox:
			checkbox := Checkbox{}
			err = json.Unmarshal(b, &checkbox)
			if err != nil { 
				continue
			}
			paginatedResponse = append(paginatedResponse, checkbox)
		case PropertyConfigTypeURL:
			url := Url{}
			err = json.Unmarshal(b, &url)
			if err != nil { 
				continue
			}
			paginatedResponse = append(paginatedResponse, url)
		case PropertyConfigTypeEmail:
			email := Email{}
			err = json.Unmarshal(b, &email)
			if err != nil { 
				continue
			}
			paginatedResponse = append(paginatedResponse, email)
		case PropertyConfigTypePhoneNumber:
			phone := Phone{}
			err = json.Unmarshal(b, &phone)
			if err != nil { 
				continue
			}
			paginatedResponse = append(paginatedResponse, phone)
		case PropertyConfigTypeFormula:
			formula := Formula{}
			err = json.Unmarshal(b, &formula)
			if err != nil { 
				continue
			}
			paginatedResponse = append(paginatedResponse, formula)
		case PropertyConfigTypeRollup:
			rollup := Rollup{}
			err = json.Unmarshal(b, &rollup)
			if err != nil { 
				continue
			}
			paginatedResponse = append(paginatedResponse, rollup)
		case PropertyConfigCreatedTime:
			ct := CreatedTime{}
			err = json.Unmarshal(b, &ct)
			if err != nil { 
				continue
			}
			paginatedResponse = append(paginatedResponse, ct)
		case PropertyConfigCreatedBy:
			cb := CreatedBy{}
			err = json.Unmarshal(b, &cb)
			if err != nil { 
				continue
			}
			paginatedResponse = append(paginatedResponse, cb)
		case PropertyConfigLastEditedTime:
			lt := LastEditedTime{}
			err = json.Unmarshal(b, &lt)
			if err != nil { 
				continue
			}
			paginatedResponse = append(paginatedResponse, lt)
		case PropertyConfigLastEditedBy:
			le := LastEditedBy{}
			err = json.Unmarshal(b, &le)
			if err != nil {
				continue
			}
			paginatedResponse = append(paginatedResponse, le)
		case PropertyConfigStatus:
			sp := StatusProperty{}
			err = json.Unmarshal(b, &sp)
			if err != nil {
				continue
			}
			paginatedResponse = append(paginatedResponse, sp)
		default:
			return nil, fmt.Errorf("unsupported property type: %s", propertyType)
		}
		hasMore = false
	}
	return paginatedResponse, nil
}
