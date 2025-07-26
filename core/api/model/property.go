package apimodel

import (
	"encoding/json"
	"fmt"

	"github.com/anyproto/anytype-heart/core/api/util"
)

type PropertyFormat string

const (
	PropertyFormatText        PropertyFormat = "text"
	PropertyFormatNumber      PropertyFormat = "number"
	PropertyFormatSelect      PropertyFormat = "select"
	PropertyFormatMultiSelect PropertyFormat = "multi_select"
	PropertyFormatDate        PropertyFormat = "date"
	PropertyFormatFiles       PropertyFormat = "files"
	PropertyFormatCheckbox    PropertyFormat = "checkbox"
	PropertyFormatUrl         PropertyFormat = "url"
	PropertyFormatEmail       PropertyFormat = "email"
	PropertyFormatPhone       PropertyFormat = "phone"
	PropertyFormatObjects     PropertyFormat = "objects"
)

func (pf *PropertyFormat) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch PropertyFormat(s) {
	case PropertyFormatText, PropertyFormatNumber, PropertyFormatSelect, PropertyFormatMultiSelect, PropertyFormatDate, PropertyFormatFiles, PropertyFormatCheckbox, PropertyFormatUrl, PropertyFormatEmail, PropertyFormatPhone, PropertyFormatObjects:
		*pf = PropertyFormat(s)
		return nil
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid property format: %q", s))
	}
}

type PropertyResponse struct {
	Property Property `json:"property"` // The property
}

type CreatePropertyRequest struct {
	Key    string             `json:"key" example:"some_user_defined_property_key"`                                                                  // The key of the property; should always be snake_case, otherwise it will be converted to snake_case
	Name   string             `json:"name" binding:"required" example:"Last modified date"`                                                          // The name of the property
	Format PropertyFormat     `json:"format" binding:"required" enums:"text,number,select,multi_select,date,files,checkbox,url,email,phone,objects"` // The format of the property
	Tags   []CreateTagRequest `json:"tags"`                                                                                                          // Tags to create for select/multi_select properties
}

type UpdatePropertyRequest struct {
	Key  *string `json:"key" example:"some_user_defined_property_key"`         // The key to set for the property; ; should always be snake_case, otherwise it will be converted to snake_case
	Name *string `json:"name" binding:"required" example:"Last modified date"` // The name to set for the property
}

type Property struct {
	Object string         `json:"object" example:"property"`                                                                  // The data model of the object
	Id     string         `json:"id" example:"bafyreids36kpw5ppuwm3ce2p4ezb3ab7cihhkq6yfbwzwpp4mln7rcgw7a"`                   // The id of the property
	Key    string         `json:"key" example:"last_modified_date"`                                                           // The key of the property
	Name   string         `json:"name" example:"Last modified date"`                                                          // The name of the property
	Format PropertyFormat `json:"format" enums:"text,number,select,multi_select,date,files,checkbox,url,email,phone,objects"` // The format of the property
	// Rk is internal-only to simplify lookup on entry, won't be serialized to property responses
	RelationKey string `json:"-" swaggerignore:"true"`
}

type PropertyLink struct {
	Key    string         `json:"key" binding:"required"  example:"last_modified_date"`                                                          // The key of the property
	Name   string         `json:"name" binding:"required" example:"Last modified date"`                                                          // The name of the property
	Format PropertyFormat `json:"format" binding:"required" enums:"text,number,select,multi_select,date,files,checkbox,url,email,phone,objects"` // The format of the property
}

type PropertyWithValue struct {
	WrappedPropertyWithValue `swaggerignore:"true"`
}

func (p PropertyWithValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.WrappedPropertyWithValue)
}

func (p *PropertyWithValue) UnmarshalJSON(data []byte) error {
	var raw struct {
		Format PropertyFormat `json:"format"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	switch raw.Format {
	case PropertyFormatText:
		var v TextPropertyValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyWithValue = v
	case PropertyFormatNumber:
		var v NumberPropertyValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyWithValue = v
	case PropertyFormatSelect:
		var v SelectPropertyValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyWithValue = v
	case PropertyFormatMultiSelect:
		var v MultiSelectPropertyValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyWithValue = v
	case PropertyFormatDate:
		var v DatePropertyValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyWithValue = v
	case PropertyFormatFiles:
		var v FilesPropertyValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyWithValue = v
	case PropertyFormatCheckbox:
		var v CheckboxPropertyValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyWithValue = v
	case PropertyFormatUrl:
		var v URLPropertyValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyWithValue = v
	case PropertyFormatEmail:
		var v EmailPropertyValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyWithValue = v
	case PropertyFormatPhone:
		var v PhonePropertyValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyWithValue = v
	case PropertyFormatObjects:
		var v ObjectsPropertyValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyWithValue = v
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid property value format: %q", raw.Format))
	}

	return nil
}

type WrappedPropertyWithValue interface{ isPropertyWithValue() }

type PropertyBase struct {
	Object string `json:"object" example:"property"`                                                // The data model of the object
	Id     string `json:"id" example:"bafyreids36kpw5ppuwm3ce2p4ezb3ab7cihhkq6yfbwzwpp4mln7rcgw7a"` // The id of the property
}
type TextPropertyValue struct {
	PropertyBase
	Key    string         `json:"key" example:"description"`   // The key of the property
	Name   string         `json:"name" example:"Description"`  // The name of the property
	Format PropertyFormat `json:"format" enums:"text"`         // The format of the property
	Text   string         `json:"text" example:"Some text..."` // The text value of the property
}

func (TextPropertyValue) isPropertyWithValue() {}

type NumberPropertyValue struct {
	PropertyBase
	Key    string         `json:"key" example:"height"`  // The key of the property
	Name   string         `json:"name" example:"Height"` // The name of the property
	Format PropertyFormat `json:"format" enums:"number"` // The format of the property
	Number float64        `json:"number" example:"42"`   // The number value of the property
}

func (NumberPropertyValue) isPropertyWithValue() {}

type SelectPropertyValue struct {
	PropertyBase
	Key    string         `json:"key" example:"status"`  // The key of the property
	Name   string         `json:"name" example:"Status"` // The name of the property
	Format PropertyFormat `json:"format" enums:"select"` // The format of the property
	Select *Tag           `json:"select"`                // The selected tag value of the property
}

func (SelectPropertyValue) isPropertyWithValue() {}

type MultiSelectPropertyValue struct {
	PropertyBase
	Key         string         `json:"key" example:"tag"`           // The key of the property
	Name        string         `json:"name" example:"Tag"`          // The name of the property
	Format      PropertyFormat `json:"format" enums:"multi_select"` // The format of the property
	MultiSelect []*Tag         `json:"multi_select"`                // The selected tag values of the property
}

func (MultiSelectPropertyValue) isPropertyWithValue() {}

type DatePropertyValue struct {
	PropertyBase
	Key    string         `json:"key" example:"last_modified_date"`    // The key of the property
	Name   string         `json:"name" example:"Last modified date"`   // The name of the property
	Format PropertyFormat `json:"format" enums:"date"`                 // The format of the property
	Date   string         `json:"date" example:"2025-02-14T12:34:56Z"` // The date value of the property
}

func (DatePropertyValue) isPropertyWithValue() {}

type FilesPropertyValue struct {
	PropertyBase
	Key    string         `json:"key" example:"files"`                                                         // The key of the property
	Name   string         `json:"name" example:"Files"`                                                        // The name of the property
	Format PropertyFormat `json:"format" enums:"files"`                                                        // The format of the property
	Files  []string       `json:"files" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"` // The file values of the property
}

func (FilesPropertyValue) isPropertyWithValue() {}

type CheckboxPropertyValue struct {
	PropertyBase
	Key      string         `json:"key" example:"done"`      // The key of the property
	Name     string         `json:"name" example:"Done"`     // The name of the property
	Format   PropertyFormat `json:"format" enums:"checkbox"` // The format of the property
	Checkbox bool           `json:"checkbox" example:"true"` // The checkbox value of the property
}

func (CheckboxPropertyValue) isPropertyWithValue() {}

type URLPropertyValue struct {
	PropertyBase
	Key    string         `json:"key" example:"source"`              // The key of the property
	Name   string         `json:"name" example:"Source"`             // The name of the property
	Format PropertyFormat `json:"format" enums:"url"`                // The format of the property
	Url    string         `json:"url" example:"https://example.com"` // The URL value of the property
}

func (URLPropertyValue) isPropertyWithValue() {}

type EmailPropertyValue struct {
	PropertyBase
	Key    string         `json:"key" example:"email"`                 // The key of the property
	Name   string         `json:"name" example:"Email"`                // The name of the property
	Format PropertyFormat `json:"format" enums:"email"`                // The format of the property
	Email  string         `json:"email" example:"example@example.com"` // The email value of the property
}

func (EmailPropertyValue) isPropertyWithValue() {}

type PhonePropertyValue struct {
	PropertyBase
	Key    string         `json:"key" example:"phone"`         // The key of the property
	Name   string         `json:"name" example:"Phone"`        // The name of the property
	Format PropertyFormat `json:"format" enums:"phone"`        // The format of the property
	Phone  string         `json:"phone" example:"+1234567890"` // The phone value of the property
}

func (PhonePropertyValue) isPropertyWithValue() {}

type ObjectsPropertyValue struct {
	PropertyBase
	Key     string         `json:"key" example:"creator"`                                                         // The key of the property
	Name    string         `json:"name" example:"Created by"`                                                     // The name of the property
	Format  PropertyFormat `json:"format" enums:"objects"`                                                        // The format of the property
	Objects []string       `json:"objects" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"` // The object values of the property
}

func (ObjectsPropertyValue) isPropertyWithValue() {}

type PropertyLinkWithValue struct {
	WrappedPropertyLinkWithValue `swaggerignore:"true"`
}

func (p PropertyLinkWithValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.WrappedPropertyLinkWithValue)
}

func (p *PropertyLinkWithValue) UnmarshalJSON(data []byte) error {
	var aux map[string]json.RawMessage
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	switch {
	case aux["text"] != nil:
		var v TextPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case aux["number"] != nil:
		var v NumberPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case aux["select"] != nil:
		var v SelectPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case aux["multi_select"] != nil:
		var v MultiSelectPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case aux["date"] != nil:
		var v DatePropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case aux["files"] != nil:
		var v FilesPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case aux["checkbox"] != nil:
		var v CheckboxPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case aux["url"] != nil:
		var v URLPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case aux["email"] != nil:
		var v EmailPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case aux["phone"] != nil:
		var v PhonePropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case aux["objects"] != nil:
		var v ObjectsPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	default:
		return util.ErrBadInput("could not determine property link value type")
	}
	return nil
}

type WrappedPropertyLinkWithValue interface {
	isPropertyLinkWithValue()
	Key() string
	Value() interface{}
}

type TextPropertyLinkValue struct {
	PropertyKey string  `json:"key" example:"description"`
	Text        *string `json:"text" example:"Some text..."` // The text value of the property
}

func (TextPropertyLinkValue) isPropertyLinkWithValue() {}

func (v TextPropertyLinkValue) Key() string {
	return v.PropertyKey
}

func (v TextPropertyLinkValue) Value() interface{} {
	if v.Text == nil {
		return nil
	}
	return *v.Text
}

type NumberPropertyLinkValue struct {
	PropertyKey string   `json:"key" example:"height"`
	Number      *float64 `json:"number" example:"42"` // The number value of the property
}

func (NumberPropertyLinkValue) isPropertyLinkWithValue() {}

func (v NumberPropertyLinkValue) Key() string {
	return v.PropertyKey
}

func (v NumberPropertyLinkValue) Value() interface{} {
	if v.Number == nil {
		return nil
	}
	return *v.Number
}

type SelectPropertyLinkValue struct {
	PropertyKey string  `json:"key" example:"status"`
	Select      *string `json:"select" example:"tag_id"` // The selected tag id of the property; see ListTags endpoint for valid values
}

func (SelectPropertyLinkValue) isPropertyLinkWithValue() {}

func (v SelectPropertyLinkValue) Key() string {
	return v.PropertyKey
}

func (v SelectPropertyLinkValue) Value() interface{} {
	if v.Select == nil {
		return nil
	}
	return *v.Select
}

type MultiSelectPropertyLinkValue struct {
	PropertyKey string    `json:"key" example:"tag"`
	MultiSelect *[]string `json:"multi_select" example:"bafyreiaixlnaefu3ci22zdenjhsdlyaeeoyjrsid5qhfeejzlccijbj7sq,bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"` // The selected tag ids of the property; see ListTags endpoint for valid values
}

func (MultiSelectPropertyLinkValue) isPropertyLinkWithValue() {}

func (v MultiSelectPropertyLinkValue) Key() string {
	return v.PropertyKey
}

func (v MultiSelectPropertyLinkValue) Value() interface{} {
	if v.MultiSelect == nil || len(*v.MultiSelect) == 0 {
		return nil
	}
	ids := make([]interface{}, len(*v.MultiSelect))
	for i, id := range *v.MultiSelect {
		ids[i] = id
	}
	return ids
}

type DatePropertyLinkValue struct {
	PropertyKey string  `json:"key" example:"last_modified_date"`
	Date        *string `json:"date" example:"2025-02-14T12:34:56Z"` // The date value of the property
}

func (DatePropertyLinkValue) isPropertyLinkWithValue() {}

func (v DatePropertyLinkValue) Key() string {
	return v.PropertyKey
}

func (v DatePropertyLinkValue) Value() interface{} {
	if v.Date == nil {
		return nil
	}
	return *v.Date
}

type FilesPropertyLinkValue struct {
	PropertyKey string    `json:"key" example:"files"`
	Files       *[]string `json:"files" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"` // The file ids of the property
}

func (FilesPropertyLinkValue) isPropertyLinkWithValue() {}

func (v FilesPropertyLinkValue) Key() string {
	return v.PropertyKey
}

func (v FilesPropertyLinkValue) Value() interface{} {
	if v.Files == nil || len(*v.Files) == 0 {
		return nil
	}
	ids := make([]interface{}, len(*v.Files))
	for i, id := range *v.Files {
		ids[i] = id
	}
	return ids
}

type CheckboxPropertyLinkValue struct {
	PropertyKey string `json:"key" example:"done"`
	Checkbox    *bool  `json:"checkbox" example:"true"` // The checkbox value of the property
}

func (CheckboxPropertyLinkValue) isPropertyLinkWithValue() {}

func (v CheckboxPropertyLinkValue) Key() string {
	return v.PropertyKey
}

func (v CheckboxPropertyLinkValue) Value() interface{} {
	if v.Checkbox == nil {
		return nil
	}
	return *v.Checkbox
}

type URLPropertyLinkValue struct {
	PropertyKey string  `json:"key" example:"source"`
	Url         *string `json:"url" example:"https://example.com"` // The URL value of the property
}

func (URLPropertyLinkValue) isPropertyLinkWithValue() {}

func (v URLPropertyLinkValue) Key() string {
	return v.PropertyKey
}

func (v URLPropertyLinkValue) Value() interface{} {
	if v.Url == nil {
		return nil
	}
	return *v.Url
}

type EmailPropertyLinkValue struct {
	PropertyKey string  `json:"key" example:"email"`
	Email       *string `json:"email" example:"example@example.com"` // The email value of the property
}

func (EmailPropertyLinkValue) isPropertyLinkWithValue() {}

func (v EmailPropertyLinkValue) Key() string {
	return v.PropertyKey
}

func (v EmailPropertyLinkValue) Value() interface{} {
	if v.Email == nil {
		return nil
	}
	return *v.Email
}

type PhonePropertyLinkValue struct {
	PropertyKey string  `json:"key" example:"phone"`
	Phone       *string `json:"phone" example:"+1234567890"` // The phone value of the property
}

func (PhonePropertyLinkValue) isPropertyLinkWithValue() {}

func (v PhonePropertyLinkValue) Key() string {
	return v.PropertyKey
}

func (v PhonePropertyLinkValue) Value() interface{} {
	if v.Phone == nil {
		return nil
	}
	return *v.Phone
}

type ObjectsPropertyLinkValue struct {
	PropertyKey string    `json:"key" example:"creator"`
	Objects     *[]string `json:"objects" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"` // The object ids of the property
}

func (ObjectsPropertyLinkValue) isPropertyLinkWithValue() {}

func (v ObjectsPropertyLinkValue) Key() string {
	return v.PropertyKey
}

func (v ObjectsPropertyLinkValue) Value() interface{} {
	if v.Objects == nil || len(*v.Objects) == 0 {
		return nil
	}
	ids := make([]interface{}, len(*v.Objects))
	for i, id := range *v.Objects {
		ids[i] = id
	}
	return ids
}
