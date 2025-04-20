package object

import "github.com/anyproto/anytype-heart/core/api/util"

type CreateObjectRequest struct {
	Name        string                 `json:"name" example:"My object"`                                                          // The name of the object
	Icon        util.Icon              `json:"icon"`                                                                              // The icon of the object
	Description string                 `json:"description" example:"This is a description of the object."`                        // The description of the object
	Body        string                 `json:"body" example:"This is the body of the object. Markdown syntax is supported here."` // The body of the object
	Source      string                 `json:"source" example:"https://bookmark-source.com"`                                      // The source url, only applicable for bookmarks
	TemplateId  string                 `json:"template_id" example:"bafyreictrp3obmnf6dwejy5o4p7bderaaia4bdg2psxbfzf44yya5uutge"` // The id of the template to use
	TypeKey     string                 `json:"type_key" example:"ot-page"`                                                        // The key of the type of object to create
	Properties  map[string]interface{} `json:"properties" example:"{\"property_key\": \"value\"}"`                                // Properties to set on the object
}

type ObjectResponse struct {
	Object ObjectWithBlocks `json:"object"` // The object
}

type Object struct {
	Object     string     `json:"object" example:"object"`                                                                      // The data model of the object
	Id         string     `json:"id" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"`                     // The id of the object
	Name       string     `json:"name" example:"My object"`                                                                     // The name of the object
	Icon       util.Icon  `json:"icon"`                                                                                         // The icon of the object
	Archived   bool       `json:"archived" example:"false"`                                                                     // Whether the object is archived
	SpaceId    string     `json:"space_id" example:"bafyreigyfkt6rbv24sbv5aq2hko3bhmv5xxlf22b4bypdu6j7hnphm3psq.23me69r569oi1"` // The id of the space the object is in
	Snippet    string     `json:"snippet" example:"The beginning of the object body..."`                                        // The snippet of the object, especially important for notes as they don't have a name
	Layout     string     `json:"layout" example:"basic"`                                                                       // The layout of the object
	Type       Type       `json:"type"`                                                                                         // The type of the object
	Properties []Property `json:"properties"`                                                                                   // The properties of the object
}

type ObjectWithBlocks struct {
	Object     string     `json:"object" example:"object"`                                                                      // The data model of the object
	Id         string     `json:"id" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"`                     // The id of the object
	Name       string     `json:"name" example:"My object"`                                                                     // The name of the object
	Icon       util.Icon  `json:"icon"`                                                                                         // The icon of the object
	Archived   bool       `json:"archived" example:"false"`                                                                     // Whether the object is archived
	SpaceId    string     `json:"space_id" example:"bafyreigyfkt6rbv24sbv5aq2hko3bhmv5xxlf22b4bypdu6j7hnphm3psq.23me69r569oi1"` // The id of the space the object is in
	Snippet    string     `json:"snippet" example:"The beginning of the object body..."`                                        // The snippet of the object, especially important for notes as they don't have a name
	Layout     string     `json:"layout" example:"basic"`                                                                       // The layout of the object
	Type       Type       `json:"type"`                                                                                         // The type of the object
	Properties []Property `json:"properties"`                                                                                   // The properties of the object
	Blocks     []Block    `json:"blocks"`                                                                                       // The blocks of the object. Omitted in endpoints for searching or listing objects, only included when getting single object.
}

type Block struct {
	Id              string    `json:"id" example:"64394517de52ad5acb89c66c"`                                                                      // The id of the block
	ChildrenIds     []string  `json:"children_ids" example:"['6797ce8ecda913cde14b02dc']"`                                                        // The ids of the block's children
	BackgroundColor string    `json:"background_color" example:"red"`                                                                             // The background color of the block
	Align           string    `json:"align" enums:"AlignLeft,AlignCenter,AlignRight,AlignJustify" example:"AlignLeft"`                            // The alignment of the block
	VerticalAlign   string    `json:"vertical_align" enums:"VerticalAlignTop,VerticalAlignMiddle,VerticalAlignBottom" example:"VerticalAlignTop"` // The vertical alignment of the block
	Text            *Text     `json:"text,omitempty"`                                                                                             // The text of the block, if applicable
	File            *File     `json:"file,omitempty"`                                                                                             // The file of the block, if applicable
	Property        *Property `json:"property,omitempty"`                                                                                         // The property block, if applicable
}

type Text struct {
	Text    string    `json:"text" example:"Some text..."`                                                                                                                      // The text
	Style   string    `json:"style" enums:"Paragraph,Header1,Header2,Header3,Header4,Quote,Code,Title,Checkbox,Marked,Numbered,Toggle,Description,Callout" example:"Paragraph"` // The style of the text
	Checked bool      `json:"checked" example:"true"`                                                                                                                           // Whether the text is checked
	Color   string    `json:"color" example:"red"`                                                                                                                              // The color of the text
	Icon    util.Icon `json:"icon" `                                                                                                                                            // The icon of the text
}

type File struct {
	Hash           string `json:"hash"`             // The hash of the file
	Name           string `json:"name"`             // The name of the file
	Type           string `json:"type"`             // The type of the file
	Mime           string `json:"mime"`             // The mime of the file
	Size           int    `json:"size"`             // The size of the file
	AddedAt        int    `json:"added_at"`         // The added at of the file
	TargetObjectId string `json:"target_object_id"` // The target object id of the file
	State          string `json:"state"`            // The state of the file
	Style          string `json:"style"`            // The style of the file
}

type PropertyResponse struct {
	Property Property `json:"property"` // The property
}

type Property struct {
	Id          string         `json:"id" example:"bafyreids36kpw5ppuwm3ce2p4ezb3ab7cihhkq6yfbwzwpp4mln7rcgw7a"`                                // The id of the property
	Key         string         `json:"key" example:"last_modified_date"`                                                                        // The key of the property
	Name        string         `json:"name" example:"Last modified date"`                                                                       // The name of the property
	Format      PropertyFormat `json:"format" example:"date" enums:"text,number,select,multi_select,date,file,checkbox,url,email,phone,object"` // The format of the property
	Text        *string        `json:"text,omitempty" example:"Some text..."`                                                                   // The text value, if applicable
	Number      *float64       `json:"number,omitempty" example:"42"`                                                                           // The number value, if applicable
	Select      *Tag           `json:"select,omitempty"`                                                                                        // The select value, if applicable
	MultiSelect []Tag          `json:"multi_select,omitempty"`                                                                                  // The multi-select values, if applicable
	Date        *string        `json:"date,omitempty" example:"2025-02-14T12:34:56Z"`                                                           // The date value, if applicable
	File        []string       `json:"file,omitempty" example:"['fileId']"`                                                                     // The file references, if applicable
	Checkbox    *bool          `json:"checkbox,omitempty" example:"true" enums:"true,false"`                                                    // The checkbox value, if applicable
	Url         *string        `json:"url,omitempty" example:"https://example.com"`                                                             // The url value, if applicable
	Email       *string        `json:"email,omitempty" example:"example@example.com"`                                                           // The email value, if applicable
	Phone       *string        `json:"phone,omitempty" example:"+1234567890"`                                                                   // The phone number value, if applicable
	Object      []string       `json:"object,omitempty" example:"['objectId']"`                                                                 // The object references, if applicable
}

type PropertyFormat string

const (
	PropertyFormatText        PropertyFormat = "text"
	PropertyFormatNumber      PropertyFormat = "number"
	PropertyFormatSelect      PropertyFormat = "select"
	PropertyFormatMultiSelect PropertyFormat = "multi_select"
	PropertyFormatDate        PropertyFormat = "date"
	PropertyFormatFile        PropertyFormat = "file"
	PropertyFormatCheckbox    PropertyFormat = "checkbox"
	PropertyFormatUrl         PropertyFormat = "url"
	PropertyFormatEmail       PropertyFormat = "email"
	PropertyFormatPhone       PropertyFormat = "phone"
	PropertyFormatObject      PropertyFormat = "object"
)

type Tag struct {
	Id    string     `json:"id" example:"bafyreiaixlnaefu3ci22zdenjhsdlyaeeoyjrsid5qhfeejzlccijbj7sq"`             // The id of the tag
	Name  string     `json:"name" example:"in-progress"`                                                           // The name of the tag
	Color util.Color `json:"color" example:"yellow" enums:"grey,yellow,orange,red,pink,purple,blue,ice,teal,lime"` // The color of the tag
}

type TypeResponse struct {
	Type Type `json:"type"` // The type
}

type Type struct {
	Object     string     `json:"object" example:"type"`                                                    // The data model of the object
	Id         string     `json:"id" example:"bafyreigyb6l5szohs32ts26ku2j42yd65e6hqy2u3gtzgdwqv6hzftsetu"` // The id of the type (which is unique across spaces)
	Key        string     `json:"key" example:"ot-page"`                                                    // The key of the type (can be the same across spaces for known types)
	Name       string     `json:"name" example:"Page"`                                                      // The name of the type
	Icon       util.Icon  `json:"icon"`                                                                     // The icon of the type
	Archived   bool       `json:"archived" example:"false"`                                                 // Whether the type is archived
	Layout     string     `json:"layout" example:"todo"`                                                    // The recommended layout of the type
	Properties []Property `json:"properties"`                                                               // The properties linked to the type
}

type TemplateResponse struct {
	Template Template `json:"template"` // The template
}

type Template struct {
	Object   string    `json:"object" example:"template"`                                                // The data model of the object
	Id       string    `json:"id" example:"bafyreictrp3obmnf6dwejy5o4p7bderaaia4bdg2psxbfzf44yya5uutge"` // The id of the template
	Name     string    `json:"name" example:"My template"`                                               // The name of the template
	Icon     util.Icon `json:"icon"`                                                                     // The icon of the template
	Archived bool      `json:"archived" example:"false"`                                                 // Whether the template is archived
}
