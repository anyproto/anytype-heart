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
	Name   string         `json:"name" binding:"required" example:"Last modified date"`                                                                         // The name of the property
	Format PropertyFormat `json:"format" binding:"required" example:"date" enums:"text,number,select,multi_select,date,files,checkbox,url,email,phone,objects"` // The format of the property
}

type UpdatePropertyRequest struct {
	Name *string `json:"name,omitempty" binding:"required" example:"Last modified date"` // The name to set for the property
}

type Property struct {
	Object string         `json:"object" example:"property"`                                                                  // The data model of the object
	Id     string         `json:"id" example:"bafyreids36kpw5ppuwm3ce2p4ezb3ab7cihhkq6yfbwzwpp4mln7rcgw7a"`                   // The id of the property
	Key    string         `json:"key" example:"last_modified_date"`                                                           // The key of the property
	Name   string         `json:"name" example:"Last modified date"`                                                          // The name of the property
	Format PropertyFormat `json:"format" enums:"text,number,select,multi_select,date,files,checkbox,url,email,phone,objects"` // The format of the property
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
	Object string         `json:"object" example:"property"`                                                                  // The data model of the object
	Id     string         `json:"id" example:"bafyreids36kpw5ppuwm3ce2p4ezb3ab7cihhkq6yfbwzwpp4mln7rcgw7a"`                   // The id of the property
	Key    string         `json:"key" example:"last_modified_date"`                                                           // The key of the property
	Name   string         `json:"name" example:"Last modified date"`                                                          // The name of the property
	Format PropertyFormat `json:"format" enums:"text,number,select,multi_select,date,files,checkbox,url,email,phone,objects"` // The format of the property
}
type TextPropertyValue struct {
	PropertyBase
	Text string `json:"text" example:"Some text..."` // The text value of the property
}

func (TextPropertyValue) isPropertyWithValue() {}

type NumberPropertyValue struct {
	PropertyBase
	Number float64 `json:"number" example:"42"` // The number value of the property
}

func (NumberPropertyValue) isPropertyWithValue() {}

type SelectPropertyValue struct {
	PropertyBase
	Select *Tag `json:"select,omitempty"` // The selected tag value of the property
}

func (SelectPropertyValue) isPropertyWithValue() {}

type MultiSelectPropertyValue struct {
	PropertyBase
	MultiSelect []Tag `json:"multi_select,omitempty"` // The selected tag values of the property
}

func (MultiSelectPropertyValue) isPropertyWithValue() {}

type DatePropertyValue struct {
	PropertyBase
	Date string `json:"date" example:"2025-02-14T12:34:56Z"` // The date value of the property
}

func (DatePropertyValue) isPropertyWithValue() {}

type FilesPropertyValue struct {
	PropertyBase
	Files []string `json:"files" example:"['fileId']"` // The file values of the property
}

func (FilesPropertyValue) isPropertyWithValue() {}

type CheckboxPropertyValue struct {
	PropertyBase
	Checkbox bool `json:"checkbox" example:"true"` // The checkbox value of the property
}

func (CheckboxPropertyValue) isPropertyWithValue() {}

type URLPropertyValue struct {
	PropertyBase
	Url string `json:"url" example:"https://example.com"` // The URL value of the property
}

func (URLPropertyValue) isPropertyWithValue() {}

type EmailPropertyValue struct {
	PropertyBase
	Email string `json:"email" example:"example@example.com"` // The email value of the property
}

func (EmailPropertyValue) isPropertyWithValue() {}

type PhonePropertyValue struct {
	PropertyBase
	Phone string `json:"phone" example:"+1234567890"` // The phone value of the property
}

func (PhonePropertyValue) isPropertyWithValue() {}

type ObjectsPropertyValue struct {
	PropertyBase
	Objects []string `json:"objects" example:"['objectId']"` // The object values of the property
}

func (ObjectsPropertyValue) isPropertyWithValue() {}

type PropertyLinkWithValue struct {
	WrappedPropertyLinkWithValue `swaggerignore:"true"`
}

func (p PropertyLinkWithValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.WrappedPropertyLinkWithValue)
}

func (p *PropertyLinkWithValue) UnmarshalJSON(data []byte) error {
	var raw struct {
		Format PropertyFormat `json:"format"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	switch raw.Format {
	case PropertyFormatText:
		var v TextPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case PropertyFormatNumber:
		var v NumberPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case PropertyFormatSelect:
		var v SelectPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case PropertyFormatMultiSelect:
		var v MultiSelectPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case PropertyFormatDate:
		var v DatePropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case PropertyFormatFiles:
		var v FilesPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case PropertyFormatCheckbox:
		var v CheckboxPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case PropertyFormatUrl:
		var v URLPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case PropertyFormatEmail:
		var v EmailPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case PropertyFormatPhone:
		var v PhonePropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	case PropertyFormatObjects:
		var v ObjectsPropertyLinkValue
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		p.WrappedPropertyLinkWithValue = v
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid property link value format: %q", raw.Format))
	}

	return nil
}

type WrappedPropertyLinkWithValue interface{ isPropertyLinkWithValue() }

type PropertyLinkBase struct {
	Key    string         `json:"key" example:"last_modified_date"`                                                                           // The key of the property
	Format PropertyFormat `json:"format" required:"true" enums:"text,number,select,multi_select,date,files,checkbox,url,email,phone,objects"` // The format of the property
}

type TextPropertyLinkValue struct {
	PropertyLinkBase
	Text string `json:"text" example:"Some text..."` // The text value of the property
}

func (TextPropertyLinkValue) isPropertyLinkWithValue() {}

type NumberPropertyLinkValue struct {
	PropertyLinkBase
	Number *float64 `json:"number" example:"42"` // The number value of the property
}

func (NumberPropertyLinkValue) isPropertyLinkWithValue() {}

type SelectPropertyLinkValue struct {
	PropertyLinkBase
	Select *string `json:"select,omitempty"` // The selected tag value of the property
}

func (SelectPropertyLinkValue) isPropertyLinkWithValue() {}

type MultiSelectPropertyLinkValue struct {
	PropertyLinkBase
	MultiSelect []string `json:"multi_select,omitempty"` // The selected tag values of the property
}

func (MultiSelectPropertyLinkValue) isPropertyLinkWithValue() {}

type DatePropertyLinkValue struct {
	PropertyLinkBase
	Date *string `json:"date" example:"2025-02-14T12:34:56Z"` // The date value of the property
}

func (DatePropertyLinkValue) isPropertyLinkWithValue() {}

type FilesPropertyLinkValue struct {
	PropertyLinkBase
	Files []string `json:"files" example:"['fileId']"` // The file values of the property
}

func (FilesPropertyLinkValue) isPropertyLinkWithValue() {}

type CheckboxPropertyLinkValue struct {
	PropertyLinkBase
	Checkbox bool `json:"checkbox" example:"true"` // The checkbox value of the property
}

func (CheckboxPropertyLinkValue) isPropertyLinkWithValue() {}

type URLPropertyLinkValue struct {
	PropertyLinkBase
	Url string `json:"url" example:"https://example.com"` // The URL value of the property
}

func (URLPropertyLinkValue) isPropertyLinkWithValue() {}

type EmailPropertyLinkValue struct {
	PropertyLinkBase
	Email string `json:"email" example:"example@example.com"` // The email value of the property
}

func (EmailPropertyLinkValue) isPropertyLinkWithValue() {}

type PhonePropertyLinkValue struct {
	PropertyLinkBase
	Phone string `json:"phone" example:"+1234567890"` // The phone value of the property
}

func (PhonePropertyLinkValue) isPropertyLinkWithValue() {}

type ObjectsPropertyLinkValue struct {
	PropertyLinkBase
	Objects []string `json:"objects" example:"['objectId']"` // The object values of the property
}

func (ObjectsPropertyLinkValue) isPropertyLinkWithValue() {}
