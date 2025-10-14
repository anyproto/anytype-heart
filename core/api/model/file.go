package apimodel

type File struct {
	Object     string              `json:"object" example:"file"`                                                                                          // The data model of the object
	Id         string              `json:"id" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"`                                       // The id of the file
	Name       string              `json:"name" example:"My document.pdf"`                                                                                 // The name of the file
	SpaceId    string              `json:"space_id" example:"bafyreigyfkt6rbv24sbv5aq2hko3bhmv5xxlf22b4bypdu6j7hnphm3psq.23me69r569oi1"`                   // The id of the space the file is in
	Layout     string              `json:"layout" example:"file" enums:"file,image"`                                                                       // The layout of the file object (file or image)
	Type       *Type               `json:"type" extensions:"nullable"`                                                                                     // The type of the file, or null if the type has been deleted
	URL        string              `json:"url" example:"http://localhost:31006/image/bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"`         // The URL to access/download the file content via the gateway
	Properties []PropertyWithValue `json:"properties" oneOf:"TextPropertyValue,NumberPropertyValue,DatePropertyValue,FilesPropertyValue,UrlPropertyValue"` // The properties of the file
}

type FileResponse struct {
	File File `json:"file"` // The file object
}
