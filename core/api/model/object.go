package apimodel

import (
	"encoding/json"
	"fmt"

	"github.com/anyproto/anytype-heart/core/api/util"
)

type ObjectLayout string

const (
	ObjectLayoutBasic       ObjectLayout = "basic"
	ObjectLayoutProfile     ObjectLayout = "profile"
	ObjectLayoutAction      ObjectLayout = "action"
	ObjectLayoutNote        ObjectLayout = "note"
	ObjectLayoutBookmark    ObjectLayout = "bookmark"
	ObjectLayoutSet         ObjectLayout = "set"
	ObjectLayoutCollection  ObjectLayout = "collection"
	ObjectLayoutParticipant ObjectLayout = "participant"
)

func (ol *ObjectLayout) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch ObjectLayout(s) {
	case ObjectLayoutBasic, ObjectLayoutProfile, ObjectLayoutAction, ObjectLayoutNote, ObjectLayoutBookmark, ObjectLayoutSet, ObjectLayoutCollection, ObjectLayoutParticipant:
		*ol = ObjectLayout(s)
		return nil
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid object layout: %q", s))
	}
}

type BodyFormat string

const (
	BodyFormatMarkdown BodyFormat = "md"
	// BodyFormatJSON     BodyFormat = "json" // TODO: implement multiple formats
)

type CreateObjectRequest struct {
	Name       string                  `json:"name" example:"My object"`                                                                                                                                                                                                                                                                 // The name of the object
	Icon       Icon                    `json:"icon" oneOf:"EmojiIcon,FileIcon,NamedIcon"`                                                                                                                                                                                                                                                // The icon of the object
	Body       string                  `json:"body" example:"This is the body of the object. Markdown syntax is supported here."`                                                                                                                                                                                                        // The body of the object
	TemplateId string                  `json:"template_id" example:"bafyreictrp3obmnf6dwejy5o4p7bderaaia4bdg2psxbfzf44yya5uutge"`                                                                                                                                                                                                        // The id of the template to use
	TypeKey    string                  `json:"type_key" binding:"required" example:"page"`                                                                                                                                                                                                                                               // The key of the type of object to create
	Properties []PropertyLinkWithValue `json:"properties" oneOf:"TextPropertyLinkValue,NumberPropertyLinkValue,SelectPropertyLinkValue,MultiSelectPropertyLinkValue,DatePropertyLinkValue,FilesPropertyLinkValue,CheckboxPropertyLinkValue,UrlPropertyLinkValue,EmailPropertyLinkValue,PhonePropertyLinkValue,ObjectsPropertyLinkValue"` // The properties to set on the object; see ListTypes or GetType endpoints for linked properties
}

type UpdateObjectRequest struct {
	Name       *string                  `json:"name" example:"My object"`                                                                                                                                                                                                                                                                 // The name of the object
	Icon       *Icon                    `json:"icon" oneOf:"EmojiIcon,FileIcon,NamedIcon"`                                                                                                                                                                                                                                                // The icon to set for the object
	TypeKey    *string                  `json:"type_key" example:"page"`                                                                                                                                                                                                                                                                  // The key of the type of object to set
	Properties *[]PropertyLinkWithValue `json:"properties" oneOf:"TextPropertyLinkValue,NumberPropertyLinkValue,SelectPropertyLinkValue,MultiSelectPropertyLinkValue,DatePropertyLinkValue,FilesPropertyLinkValue,CheckboxPropertyLinkValue,UrlPropertyLinkValue,EmailPropertyLinkValue,PhonePropertyLinkValue,ObjectsPropertyLinkValue"` // The properties to set for the object; see ListTypes or GetType endpoints for linked properties
}

type ObjectResponse struct {
	Object ObjectWithBody `json:"object"` // The object
}

type Object struct {
	Object     string              `json:"object" example:"object"`                                                                                                                                                                                                                      // The data model of the object
	Id         string              `json:"id" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"`                                                                                                                                                                     // The id of the object
	Name       string              `json:"name" example:"My object"`                                                                                                                                                                                                                     // The name of the object
	Icon       *Icon               `json:"icon" oneOf:"EmojiIcon,FileIcon,NamedIcon" extensions:"nullable"`                                                                                                                                                                              // The icon of the object, or null if the object has no icon
	Archived   bool                `json:"archived" example:"false"`                                                                                                                                                                                                                     // Whether the object is archived
	SpaceId    string              `json:"space_id" example:"bafyreigyfkt6rbv24sbv5aq2hko3bhmv5xxlf22b4bypdu6j7hnphm3psq.23me69r569oi1"`                                                                                                                                                 // The id of the space the object is in
	Snippet    string              `json:"snippet" example:"The beginning of the object body..."`                                                                                                                                                                                        // The snippet of the object, especially important for notes as they don't have a name
	Layout     ObjectLayout        `json:"layout" example:"basic"`                                                                                                                                                                                                                       // The layout of the object
	Type       *Type               `json:"type" extensions:"nullable"`                                                                                                                                                                                                                   // The type of the object, or null if the type has been deleted.
	Properties []PropertyWithValue `json:"properties" oneOf:"TextPropertyValue,NumberPropertyValue,SelectPropertyValue,MultiSelectPropertyValue,DatePropertyValue,FilesPropertyValue,CheckboxPropertyValue,UrlPropertyValue,EmailPropertyValue,PhonePropertyValue,ObjectsPropertyValue"` // The properties of the object
}

type ObjectWithBody struct {
	Object     string              `json:"object" example:"object"`                                                                                                                                                                                                                      // The data model of the object
	Id         string              `json:"id" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"`                                                                                                                                                                     // The id of the object
	Name       string              `json:"name" example:"My object"`                                                                                                                                                                                                                     // The name of the object
	Icon       *Icon               `json:"icon" oneOf:"EmojiIcon,FileIcon,NamedIcon" extensions:"nullable"`                                                                                                                                                                              // The icon of the object, or null if the object has no icon
	Archived   bool                `json:"archived" example:"false"`                                                                                                                                                                                                                     // Whether the object is archived
	SpaceId    string              `json:"space_id" example:"bafyreigyfkt6rbv24sbv5aq2hko3bhmv5xxlf22b4bypdu6j7hnphm3psq.23me69r569oi1"`                                                                                                                                                 // The id of the space the object is in
	Snippet    string              `json:"snippet" example:"The beginning of the object body..."`                                                                                                                                                                                        // The snippet of the object, especially important for notes as they don't have a name
	Layout     ObjectLayout        `json:"layout" example:"basic"`                                                                                                                                                                                                                       // The layout of the object
	Type       *Type               `json:"type" extensions:"nullable"`                                                                                                                                                                                                                   // The type of the object, or null if the type has been deleted.
	Properties []PropertyWithValue `json:"properties" oneOf:"TextPropertyValue,NumberPropertyValue,SelectPropertyValue,MultiSelectPropertyValue,DatePropertyValue,FilesPropertyValue,CheckboxPropertyValue,UrlPropertyValue,EmailPropertyValue,PhonePropertyValue,ObjectsPropertyValue"` // The properties of the object
	Markdown   string              `json:"markdown" example:"# This is the title\n..."`                                                                                                                                                                                                  // The markdown body of the object
}

// ! Deprecated schemas, until json blocks properly implemented
// type Block struct {
// 	Object          string    `json:"object" example:"block"`                                                                                     // The data model of the object
// 	Id              string    `json:"id" example:"64394517de52ad5acb89c66c"`                                                                      // The id of the block
// 	ChildrenIds     []string  `json:"children_ids" example:"['6797ce8ecda913cde14b02dc']"`                                                        // The ids of the block's children
// 	BackgroundColor string    `json:"background_color" example:"red"`                                                                             // The background color of the block
// 	Align           string    `json:"align" enums:"AlignLeft,AlignCenter,AlignRight,AlignJustify" example:"AlignLeft"`                            // The alignment of the block
// 	VerticalAlign   string    `json:"vertical_align" enums:"VerticalAlignTop,VerticalAlignMiddle,VerticalAlignBottom" example:"VerticalAlignTop"` // The vertical alignment of the block
// 	Text            *Text     `json:"text,omitempty"`                                                                                             // The text of the block, if applicable
// 	File            *File     `json:"file,omitempty"`                                                                                             // The file of the block, if applicable
// 	Property        *Property `json:"property,omitempty"`                                                                                         // The property block, if applicable
// }
//
// type Text struct {
// 	Object  string `json:"object" example:"text"`                                                                                                                            // The data model of the object
// 	Text    string `json:"text" example:"Some text..."`                                                                                                                      // The text
// 	Style   string `json:"style" enums:"Paragraph,Header1,Header2,Header3,Header4,Quote,Code,Title,Checkbox,Marked,Numbered,Toggle,Description,Callout" example:"Paragraph"` // The style of the text
// 	Checked bool   `json:"checked" example:"true"`                                                                                                                           // Whether the text is checked
// 	Color   string `json:"color" example:"red"`                                                                                                                              // The color of the text
// 	Icon    Icon   `json:"icon" `                                                                                                                                            // The icon of the text
// }
//
// type File struct {
// 	Object         string `json:"object" example:"file"` // The data model of the object
// 	Hash           string `json:"hash"`                  // The hash of the file
// 	Name           string `json:"name"`                  // The name of the file
// 	Type           string `json:"type"`                  // The type of the file
// 	Mime           string `json:"mime"`                  // The mime of the file
// 	Size           int    `json:"size"`                  // The size of the file
// 	AddedAt        int    `json:"added_at"`              // The added at of the file
// 	TargetObjectId string `json:"target_object_id"`      // The target object id of the file
// 	State          string `json:"state"`                 // The state of the file
// 	Style          string `json:"style"`                 // The style of the file
// }
