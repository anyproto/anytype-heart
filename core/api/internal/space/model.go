package space

type SpaceResponse struct {
	Space Space `json:"space"` // The space
}

type CreateSpaceRequest struct {
	Name string `json:"name" example:"New Space"` // The name of the space
}

type Space struct {
	Type                   string `json:"type" example:"space"`                                                                                                                  // The type of the object
	Id                     string `json:"id" example:"bafyreigyfkt6rbv24sbv5aq2hko3bhmv5xxlf22b4bypdu6j7hnphm3psq.23me69r569oi1"`                                                // The id of the space
	Name                   string `json:"name" example:"My Space"`                                                                                                               // The name of the space
	Icon                   string `json:"icon" example:"http://127.0.0.1:31006/image/bafybeieptz5hvcy6txplcvphjbbh5yjc2zqhmihs3owkh5oab4ezauzqay"`                               // The icon of the space
	HomeObjectId           string `json:"home_object_id" example:"bafyreie4qcl3wczb4cw5hrfyycikhjyh6oljdis3ewqrk5boaav3sbwqya"`                                                  // The id of the home object
	ArchiveObjectId        string `json:"archive_object_id" example:"bafyreialsgoyflf3etjm3parzurivyaukzivwortf32b4twnlwpwocsrri"`                                               // The id of the archive object
	ProfileObjectId        string `json:"profile_object_id" example:"bafyreiaxhwreshjqwndpwtdsu4mtihaqhhmlygqnyqpfyfwlqfq3rm3gw4"`                                               // The id of the profile object
	MarketplaceWorkspaceId string `json:"marketplace_workspace_id" example:"_anytype_marketplace"`                                                                               // The id of the marketplace workspace
	WorkspaceObjectId      string `json:"workspace_object_id" example:"bafyreiapey2g6e6za4zfxvlgwdy4hbbfu676gmwrhnqvjbxvrchr7elr3y"`                                             // The id of the workspace object
	DeviceId               string `json:"device_id" example:"12D3KooWGZMJ4kQVyQVXaj7gJPZr3RZ2nvd9M2Eq2pprEoPih9WF"`                                                              // The id of the device
	AccountSpaceId         string `json:"account_space_id" example:"bafyreihpd2knon5wbljhtfeg3fcqtg3i2pomhhnigui6lrjmzcjzep7gcy.23me69r569oi1"`                                  // The id of the account space
	WidgetsId              string `json:"widgets_id" example:"bafyreialj7pceh53mifm5dixlho47ke4qjmsn2uh4wsjf7xq2pnlo5xfva"`                                                      // The id of the widgets
	SpaceViewId            string `json:"space_view_id" example:"bafyreigzv3vq7qwlrsin6njoduq727ssnhwd6bgyfj6nm4hv3pxoc2rxhy"`                                                   // The id of the space view
	TechSpaceId            string `json:"tech_space_id" example:"bafyreif4xuwncrjl6jajt4zrrfnylpki476nv2w64yf42ovt7gia7oypii.23me69r569oi1"`                                     // The id of tech space, where objects outside of user's actual spaces are stored, e.g. spaces itself
	GatewayUrl             string `json:"gateway_url" example:"http://127.0.0.1:31006"`                                                                                          // The gateway url to serve files and media
	LocalStoragePath       string `json:"local_storage_path" example:"/Users/johndoe/Library/Application Support/Anytype/data/AAHTtt1wuQEnaYBNZ2Cyfcvs6DqPqxgn8VXDVk4avsUkMuha"` // The local storage path of the account
	Timezone               string `json:"timezone" example:""`                                                                                                                   // The timezone of the account
	AnalyticsId            string `json:"analytics_id" example:"624aecdd-4797-4611-9d61-a2ae5f53cf1c"`                                                                           // The analytics id of the account
	NetworkId              string `json:"network_id" example:"N83gJpVd9MuNRZAuJLZ7LiMntTThhPc6DtzWWVjb1M3PouVU"`                                                                 // The network id of the space
}

type MemberResponse struct {
	Member Member `json:"member"` // The member
}

type Member struct {
	Type       string `json:"type" example:"member"`                                                                                                                                // The type of the object
	Id         string `json:"id" example:"_participant_bafyreigyfkt6rbv24sbv5aq2hko1bhmv5xxlf22b4bypdu6j7hnphm3psq_23me69r569oi1_AAjEaEwPF4nkEh9AWkqEnzcQ8HziBB4ETjiTpvRCQvWnSMDZ"` // The profile object id of the member
	Name       string `json:"name" example:"John Doe"`                                                                                                                              // The name of the member
	Icon       string `json:"icon" example:"http://127.0.0.1:31006/image/bafybeieptz5hvcy6txplcvphjbbh5yjc2zqhmihs3owkh5oab4ezauzqay?width=100"`                                    // The icon of the member
	Identity   string `json:"identity" example:"AAjEaEwPF4nkEh7AWkqEnzcQ8HziGB4ETjiTpvRCQvWnSMDZ"`                                                                                  // The identity of the member in the network
	GlobalName string `json:"global_name" example:"john.any"`                                                                                                                       // The global name of the member in the network
	Role       string `json:"role" enums:"Reader,Writer,Owner,NoPermission" example:"Owner"`                                                                                        // The role of the member
}
