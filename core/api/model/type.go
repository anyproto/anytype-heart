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
	TypeLayoutAction  TypeLayout = "action"
	TypeLayoutNote    TypeLayout = "note"
)

func (tl *TypeLayout) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch TypeLayout(s) {
	case TypeLayoutBasic, TypeLayoutProfile, TypeLayoutAction, TypeLayoutNote:
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
	Key        string         `json:"key" example:"some_user_defined_type_key"`                    // The key of the type; should always be snake_case, otherwise it will be converted to snake_case
	Name       string         `json:"name" binding:"required" example:"Page"`                      // The name of the type
	PluralName string         `json:"plural_name" binding:"required" example:"Pages"`              // The plural name of the type
	Icon       Icon           `json:"icon" oneOf:"EmojiIcon,FileIcon,NamedIcon"`                   // The icon of the type
	Layout     TypeLayout     `json:"layout" binding:"required" enums:"basic,profile,action,note"` // The layout of the type
	Properties []PropertyLink `json:"properties"`                                                  // The properties linked to the type
}

type UpdateTypeRequest struct {
	Key        *string         `json:"key" example:"some_user_defined_type_key"`  // The key to set for the type; should always be snake_case, otherwise it will be converted to snake_case
	Name       *string         `json:"name" example:"Page"`                       // The name to set for the type
	PluralName *string         `json:"plural_name" example:"Pages"`               // The plural name to set for the type
	Icon       *Icon           `json:"icon" oneOf:"EmojiIcon,FileIcon,NamedIcon"` // The icon to set for the type
	Layout     *TypeLayout     `json:"layout" enums:"basic,profile,action,note"`  // The layout to set for the type
	Properties *[]PropertyLink `json:"properties"`                                // The properties to set for the type
}

type Type struct {
	Object     string       `json:"object" example:"type"`                                                        // The data model of the object
	Id         string       `json:"id" example:"bafyreigyb6l5szohs32ts26ku2j42yd65e6hqy2u3gtzgdwqv6hzftsetu"`     // The id of the type (which is unique across spaces)
	Key        string       `json:"key" example:"page"`                                                           // The key of the type (can be the same across spaces for known types)
	Name       string       `json:"name" example:"Page"`                                                          // The name of the type
	PluralName string       `json:"plural_name" example:"Pages"`                                                  // The plural name of the type
	Icon       *Icon        `json:"icon" oneOf:"EmojiIcon,FileIcon,NamedIcon" extensions:"nullable"`              // The icon of the type, or null if the type has no icon
	Archived   bool         `json:"archived" example:"false"`                                                     // Whether the type is archived
	Layout     ObjectLayout `json:"layout" enums:"basic,profile,action,note,bookmark,set,collection,participant"` // The layout of the type
	Properties []Property   `json:"properties"`                                                                   // The properties linked to the type
	// Uk is internal-only to simplify lookup on entry, won't be serialized to type responses
	UniqueKey string `json:"-" swaggerignore:"true"`
}
