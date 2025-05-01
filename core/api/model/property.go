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
	Name string `json:"name" binding:"required" example:"Last modified date"` // The name to set for the property
}

type Property struct {
	Object string         `json:"object" example:"property"`                                                                                 // The data model of the object
	Id     string         `json:"id" example:"bafyreids36kpw5ppuwm3ce2p4ezb3ab7cihhkq6yfbwzwpp4mln7rcgw7a"`                                  // The id of the property
	Key    string         `json:"key" example:"last_modified_date"`                                                                          // The key of the property
	Name   string         `json:"name" example:"Last modified date"`                                                                         // The name of the property
	Format PropertyFormat `json:"format" example:"date" enums:"text,number,select,multi_select,date,files,checkbox,url,email,phone,objects"` // The format of the property
}

type PropertyWithValue struct {
	Object      string         `json:"object" example:"property"`                                                                                 // The data model of the object
	Id          string         `json:"id" example:"bafyreids36kpw5ppuwm3ce2p4ezb3ab7cihhkq6yfbwzwpp4mln7rcgw7a"`                                  // The id of the property
	Key         string         `json:"key" example:"last_modified_date"`                                                                          // The key of the property
	Name        string         `json:"name" example:"Last modified date"`                                                                         // The name of the property
	Format      PropertyFormat `json:"format" example:"date" enums:"text,number,select,multi_select,date,files,checkbox,url,email,phone,objects"` // The format of the property
	Text        *string        `json:"text,omitempty" example:"Some text..."`                                                                     // The text value, if applicable
	Number      *float64       `json:"number,omitempty" example:"42"`                                                                             // The number value, if applicable
	Select      *Tag           `json:"select,omitempty"`                                                                                          // The select value, if applicable
	MultiSelect []Tag          `json:"multi_select,omitempty"`                                                                                    // The multi-select values, if applicable
	Date        *string        `json:"date,omitempty" example:"2025-02-14T12:34:56Z"`                                                             // The date value, if applicable
	Files       []string       `json:"files,omitempty" example:"['fileId']"`                                                                      // The file references, if applicable
	Checkbox    *bool          `json:"checkbox,omitempty" example:"true" enums:"true,false"`                                                      // The checkbox value, if applicable
	Url         *string        `json:"url,omitempty" example:"https://example.com"`                                                               // The url value, if applicable
	Email       *string        `json:"email,omitempty" example:"example@example.com"`                                                             // The email value, if applicable
	Phone       *string        `json:"phone,omitempty" example:"+1234567890"`                                                                     // The phone number value, if applicable
	Objects     []string       `json:"objects,omitempty" example:"['objectId']"`                                                                  // The object references, if applicable
}

type PropertyLink struct {
	Key    string         `json:"key" binding:"required"  example:"last_modified_date"` // The key of the property
	Name   string         `json:"name" binding:"required" example:"Last modified date"` // The name of the property
	Format PropertyFormat `json:"format" binding:"required" example:"date"`             // The format of the property
}

type PropertyLinkWithValue struct {
	Key         string   `json:"key" binding:"required" example:"last_modified_date"`  // The key of the property
	Text        *string  `json:"text,omitempty" example:"Some text..."`                // The text value, if applicable
	Number      *float64 `json:"number,omitempty" example:"42"`                        // The number value, if applicable
	Select      *string  `json:"select,omitempty"`                                     // The select value, if applicable
	MultiSelect []string `json:"multi_select,omitempty"`                               // The multi-select values, if applicable
	Date        *string  `json:"date,omitempty" example:"2025-02-14T12:34:56Z"`        // The date value, if applicable
	Files       []string `json:"files,omitempty" example:"['fileId']"`                 // The file references, if applicable
	Checkbox    *bool    `json:"checkbox,omitempty" example:"true" enums:"true,false"` // The checkbox value, if applicable
	Url         *string  `json:"url,omitempty" example:"https://example.com"`          // The url value, if applicable
	Email       *string  `json:"email,omitempty" example:"example@example.com"`        // The email value, if applicable
	Phone       *string  `json:"phone,omitempty" example:"+1234567890"`                // The phone number value, if applicable
	Objects     []string `json:"objects,omitempty" example:"['objectId']"`             // The object references, if applicable
}
