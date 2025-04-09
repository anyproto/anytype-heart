package space

import (
	"github.com/anyproto/anytype-heart/core/api/util"
)

type SpaceResponse struct {
	Space Space `json:"space"` // The space
}

type CreateSpaceRequest struct {
	Name        string `json:"name" example:"New Space"`                   // The name of the space
	Description string `json:"description" example:"The local-first wiki"` // The description of the space
}

type Space struct {
	Object      string    `json:"object" example:"space"`                                                                 // The data model of the object
	Id          string    `json:"id" example:"bafyreigyfkt6rbv24sbv5aq2hko3bhmv5xxlf22b4bypdu6j7hnphm3psq.23me69r569oi1"` // The id of the space
	Name        string    `json:"name" example:"My Space"`                                                                // The name of the space
	Icon        util.Icon `json:"icon"`                                                                                   // The icon of the space
	Description string    `json:"description" example:"The local-first wiki"`                                             // The description of the space
	GatewayUrl  string    `json:"gateway_url" example:"http://127.0.0.1:31006"`                                           // The gateway url to serve files and media
	NetworkId   string    `json:"network_id" example:"N83gJpVd9MuNRZAuJLZ7LiMntTThhPc6DtzWWVjb1M3PouVU"`                  // The network id of the space
}

type MemberResponse struct {
	Member Member `json:"member"` // The member
}

type UpdateMemberRequest struct {
	Status string `json:"status" enums:"active,removed,declined" example:"active"` // Status of the member
	Role   string `json:"role,omitempty" enums:"viewer,editor" example:"viewer"`   // Role to assign if approving a joining member
}

type Member struct {
	Object     string    `json:"object" example:"member"`                                                                                                                              // The data model of the object
	Id         string    `json:"id" example:"_participant_bafyreigyfkt6rbv24sbv5aq2hko1bhmv5xxlf22b4bypdu6j7hnphm3psq_23me69r569oi1_AAjEaEwPF4nkEh9AWkqEnzcQ8HziBB4ETjiTpvRCQvWnSMDZ"` // The profile object id of the member
	Name       string    `json:"name" example:"John Doe"`                                                                                                                              // The name of the member
	Icon       util.Icon `json:"icon"`                                                                                                                                                 // The icon of the member
	Identity   string    `json:"identity" example:"AAjEaEwPF4nkEh7AWkqEnzcQ8HziGB4ETjiTpvRCQvWnSMDZ"`                                                                                  // The identity of the member in the network
	GlobalName string    `json:"global_name" example:"john.any"`                                                                                                                       // The global name of the member in the network
	Status     string    `json:"status" enums:"joining,active,removed,declined,removing,canceled" example:"active"`                                                                    // The status of the member
	Role       string    `json:"role" enums:"viewer,editor,owner,no_permission" example:"owner"`                                                                                       // The role of the member
}
