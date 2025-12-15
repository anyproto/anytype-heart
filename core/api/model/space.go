package apimodel

type SpaceResponse struct {
	Space Space `json:"space"` // The space
}

type CreateSpaceRequest struct {
	Name        *string `json:"name" binding:"required" example:"New Space"` // The name of the space
	Description *string `json:"description" example:"The local-first wiki"`  // The description of the space
}

type UpdateSpaceRequest struct {
	Name        *string `json:"name" example:"New Space"`                   // The name of the space
	Description *string `json:"description" example:"The local-first wiki"` // The description of the space
}

type Space struct {
	Object      string `json:"object" enums:"space,chat" example:"space"`                                              // The data model of the object
	Id          string `json:"id" example:"bafyreigyfkt6rbv24sbv5aq2hko3bhmv5xxlf22b4bypdu6j7hnphm3psq.23me69r569oi1"` // The id of the space
	Name        string `json:"name" example:"My Space"`                                                                // The name of the space
	Icon        *Icon  `json:"icon" oneOf:"EmojiIcon,FileIcon,NamedIcon" extensions:"nullable"`                        // The icon of the space, or null if the space has no icon
	Description string `json:"description" example:"The local-first wiki"`                                             // The description of the space
	GatewayUrl  string `json:"gateway_url" example:"http://127.0.0.1:31006"`                                           // The gateway url to serve files and media
	NetworkId   string `json:"network_id" example:"N83gJpVd9MuNRZAuJLZ7LiMntTThhPc6DtzWWVjb1M3PouVU"`                  // The network id of the space
}
