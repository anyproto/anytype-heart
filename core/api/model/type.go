package apimodel

import (
	"encoding/json"
	"fmt"

	"github.com/anyproto/anytype-heart/core/api/util"
)

type TypeLayout string

const (
	TypeLayoutBasic   TypeLayout = "basic"
	TypeLayoutProfile TypeLayout = "profile"
	TypeLayoutTodo    TypeLayout = "todo"
	TypeLayoutNote    TypeLayout = "note"
)

func (tl *TypeLayout) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch TypeLayout(s) {
	case TypeLayoutBasic, TypeLayoutProfile, TypeLayoutTodo, TypeLayoutNote:
		*tl = TypeLayout(s)
		return nil
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid type layout: %q", s))
	}
}

type TypeResponse struct {
	Type Type `json:"type"` // The type
}

type CreateTypeRequest struct {
	Name       string         `json:"name" binding:"required" example:"Page"`   // The name of the type
	PluralName string         `json:"plural_name" example:"Pages"`              // The plural name of the type
	Icon       Icon           `json:"icon"`                                     // The icon of the type
	Layout     TypeLayout     `json:"layout" binding:"required" example:"todo"` // The layout of the type
	Properties []PropertyLink `json:"properties"`                               // The properties linked to the type
}

type UpdateTypeRequest struct {
	Name       *string        `json:"name,omitempty" example:"Page"`         // The name to set for the type
	PluralName *string        `json:"plural_name,omitempty" example:"Pages"` // The plural name to set for the type
	Icon       Icon           `json:"icon"`                                  // The icon to set for the type
	Layout     *TypeLayout    `json:"layout,omitempty" example:"todo"`       // The layout to set for the type
	Properties []PropertyLink `json:"properties"`                            // The properties to set for the type
}

type Type struct {
	Object     string       `json:"object" example:"type"`                                                    // The data model of the object
	Id         string       `json:"id" example:"bafyreigyb6l5szohs32ts26ku2j42yd65e6hqy2u3gtzgdwqv6hzftsetu"` // The id of the type (which is unique across spaces)
	Key        string       `json:"key" example:"ot-page"`                                                    // The key of the type (can be the same across spaces for known types)
	Name       string       `json:"name" example:"Page"`                                                      // The name of the type
	Icon       Icon         `json:"icon"`                                                                     // The icon of the type
	Archived   bool         `json:"archived" example:"false"`                                                 // Whether the type is archived
	Layout     ObjectLayout `json:"layout" example:"todo"`                                                    // The layout of the type
	Properties []Property   `json:"properties"`                                                               // The properties linked to the type
}
