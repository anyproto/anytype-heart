package apimodel

import (
	"encoding/json"
	"fmt"

	"github.com/anyproto/anytype-heart/core/api/util"
)

type MemberStatus string

const (
	MemberStatusActive   MemberStatus = "active"
	MemberStatusRemoved  MemberStatus = "removed"
	MemberStatusDeclined MemberStatus = "declined"
)

func (ms *MemberStatus) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch MemberStatus(s) {
	case MemberStatusActive, MemberStatusRemoved, MemberStatusDeclined:
		*ms = MemberStatus(s)
		return nil
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid member status: %q", s))
	}
}

type MemberRole string

const (
	MemberRoleViewer MemberRole = "viewer"
	MemberRoleEditor MemberRole = "editor"
)

func (mr *MemberRole) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch MemberRole(s) {
	case MemberRoleViewer, MemberRoleEditor:
		*mr = MemberRole(s)
		return nil
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid member role: %q", s))
	}
}

type MemberResponse struct {
	Member Member `json:"member"` // The member
}

type UpdateMemberRequest struct {
	Status *MemberStatus `json:"status" binding:"required" enums:"active,removed,declined" example:"active"` // Status of the member
	Role   *MemberRole   `json:"role" enums:"viewer,editor" example:"viewer"`                                // Role to assign if approving a joining member
}

type Member struct {
	Object     string `json:"object" example:"member"`                                                                                                                              // The data model of the object
	Id         string `json:"id" example:"_participant_bafyreigyfkt6rbv24sbv5aq2hko1bhmv5xxlf22b4bypdu6j7hnphm3psq_23me69r569oi1_AAjEaEwPF4nkEh9AWkqEnzcQ8HziBB4ETjiTpvRCQvWnSMDZ"` // The profile object id of the member
	Name       string `json:"name" example:"John Doe"`                                                                                                                              // The name of the member
	Icon       *Icon  `json:"icon" oneOf:"EmojiIcon,FileIcon,NamedIcon" extensions:"nullable"`                                                                                      // The icon of the member, or null if the member has no icon
	Identity   string `json:"identity" example:"AAjEaEwPF4nkEh7AWkqEnzcQ8HziGB4ETjiTpvRCQvWnSMDZ"`                                                                                  // The identity of the member in the network
	GlobalName string `json:"global_name" example:"john.any"`                                                                                                                       // The global name of the member in the network
	Status     string `json:"status" enums:"joining,active,removed,declined,removing,canceled" example:"active"`                                                                    // The status of the member
	Role       string `json:"role" enums:"viewer,editor,owner,no_permission" example:"owner"`                                                                                       // The role of the member
}
