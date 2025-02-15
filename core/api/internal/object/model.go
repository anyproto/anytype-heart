package object

type CreateObjectRequest struct {
	Name                string `json:"name" example:"My object"`                                                          // The name of the object
	Icon                string `json:"icon" example:"📄"`                                                                  // The icon of the object
	Description         string `json:"description" example:"This is a description of the object."`                        // The description of the object
	Body                string `json:"body" example:"This is the body of the object. Markdown syntax is supported here."` // The body of the object
	Source              string `json:"source" example:"https://bookmark-source.com"`                                      // The source url, only applicable for bookmarks
	TemplateId          string `json:"template_id" example:"bafyreictrp3obmnf6dwejy5o4p7bderaaia4bdg2psxbfzf44yya5uutge"` // The id of the template to use
	ObjectTypeUniqueKey string `json:"object_type_unique_key" example:"ot-page"`                                          // The unique key of the object type
}

type ObjectResponse struct {
	Object Object `json:"object"` // The object
}

type Object struct {
	Object  string   `json:"object" example:"object"`                                                                      // The data model of the object
	Id      string   `json:"id" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"`                     // The id of the object
	Name    string   `json:"name" example:"My object"`                                                                     // The name of the object
	Icon    string   `json:"icon" example:"📄"`                                                                             // The icon of the object
	Type    Type     `json:"type"`                                                                                         // The type of the object
	Snippet string   `json:"snippet" example:"The beginning of the object body..."`                                        // The snippet of the object, especially important for notes as they don't have a name
	Layout  string   `json:"layout" example:"basic"`                                                                       // The layout of the object
	SpaceId string   `json:"space_id" example:"bafyreigyfkt6rbv24sbv5aq2hko3bhmv5xxlf22b4bypdu6j7hnphm3psq.23me69r569oi1"` // The id of the space the object is in
	RootId  string   `json:"root_id" example:"bafyreicypzj6uvu54664ucv3hmbsd5cmdy2dv4fwua26sciq74khzpyn4u"`                // The id of the object's root
	Blocks  []Block  `json:"blocks"`                                                                                       // The blocks of the object
	Details []Detail `json:"details"`                                                                                      // The details of the object
}

type Block struct {
	Id              string    `json:"id" example:"64394517de52ad5acb89c66c"`                                                                      // The id of the block
	ChildrenIds     []string  `json:"children_ids" example:"['6797ce8ecda913cde14b02dc']"`                                                        // The ids of the block's children
	BackgroundColor string    `json:"background_color" example:"red"`                                                                             // The background color of the block
	Align           string    `json:"align" enums:"AlignLeft,AlignCenter,AlignRight,AlignJustify" example:"AlignLeft"`                            // The alignment of the block
	VerticalAlign   string    `json:"vertical_align" enums:"VerticalAlignTop,VerticalAlignMiddle,VerticalAlignBottom" example:"VerticalAlignTop"` // The vertical alignment of the block
	Text            *Text     `json:"text,omitempty"`                                                                                             // The text of the block, if applicable
	File            *File     `json:"file,omitempty"`                                                                                             // The file of the block, if applicable
	Relation        *Relation `json:"relation,omitempty"`                                                                                         // The relation of the block, if applicable
}

type Text struct {
	Text    string `json:"text" example:"Some text..."`                                                                                                                      // The text
	Style   string `json:"style" enums:"Paragraph,Header1,Header2,Header3,Header4,Quote,Code,Title,Checkbox,Marked,Numbered,Toggle,Description,Callout" example:"Paragraph"` // The style of the text
	Checked bool   `json:"checked" example:"true"`                                                                                                                           // Whether the text is checked
	Color   string `json:"color" example:"red"`                                                                                                                              // The color of the text
	Icon    string `json:"icon" example:"📄"`                                                                                                                                 // The icon of the text
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

// TODO: fill in the relation struct
type Relation struct {
	Id string
}

type Detail struct {
	Id      string                 `json:"id" enums:"last_modified_date,last_modified_by,created_date,created_by,last_opened_date,tags" example:"last_modified_date"` // The id of the detail
	Details map[string]interface{} `json:"details"`                                                                                                                   // The details
}

type Tag struct {
	Id    string `json:"id" example:"bafyreiaixlnaefu3ci22zdenjhsdlyaeeoyjrsid5qhfeejzlccijbj7sq"` // The id of the tag
	Name  string `json:"name" example:"in-progress"`                                               // The name of the tag
	Color string `json:"color" example:"yellow"`                                                   // The color of the tag
}

type TypeResponse struct {
	Type Type `json:"type"` // The type
}

type Type struct {
	Object            string `json:"object" example:"type"`                                                    // The data model of the object
	Id                string `json:"id" example:"bafyreigyb6l5szohs32ts26ku2j42yd65e6hqy2u3gtzgdwqv6hzftsetu"` // The id of the type
	UniqueKey         string `json:"unique_key" example:"ot-page"`                                             // The unique key of the type
	Name              string `json:"name" example:"Page"`                                                      // The name of the type
	Icon              string `json:"icon" example:"📄"`                                                         // The icon of the type
	RecommendedLayout string `json:"recommended_layout" example:"todo"`                                        // The recommended layout of the type
}

type TemplateResponse struct {
	Template Template `json:"template"` // The template
}

type Template struct {
	Object string `json:"object" example:"template"`                                                // The data model of the object
	Id     string `json:"id" example:"bafyreictrp3obmnf6dwejy5o4p7bderaaia4bdg2psxbfzf44yya5uutge"` // The id of the template
	Name   string `json:"name" example:"My template"`                                               // The name of the template
	Icon   string `json:"icon" example:"📄"`                                                         // The icon of the template
}
