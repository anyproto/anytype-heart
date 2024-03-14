package aclobjectmanager

import "github.com/anyproto/any-sync/commonspace/object/acl/list"

type accountStatus struct {
	Permission string `json:"permission"`
	Status     string `json:"status"`
	Identity   string `json:"identity"`
}

type aclStat struct {
	TotalRecords int    `json:"totalRecords"`
	Loaded       bool   `json:"loaded"`
	AclHeadId    string `json:"aclHeadId"`
	SpaceId      string `json:"spaceId"`
	AclId        string
	Statuses     []accountStatus `json:"statuses"`
}

func parseAcl(acl list.AclList, spaceId string) aclStat {
	if acl == nil {
		return aclStat{
			Loaded:  false,
			SpaceId: spaceId,
		}
	}
	acl.RLock()
	defer acl.RUnlock()
	statuses := make([]accountStatus, 0, len(acl.AclState().CurrentAccounts()))
	for _, acc := range acl.AclState().CurrentAccounts() {
		statuses = append(statuses, accountStatus{
			Permission: convPermissionToString(acc.Permissions),
			Status:     convStatusToString(acc.Status),
			Identity:   acc.PubKey.Account(),
		})
	}
	totalRecs := len(acl.Records())
	headId := acl.Head().Id
	return aclStat{
		TotalRecords: totalRecs,
		Loaded:       true,
		SpaceId:      spaceId,
		AclHeadId:    headId,
		Statuses:     statuses,
		AclId:        acl.Id(),
	}
}

func convPermissionToString(perm list.AclPermissions) string {
	switch perm {
	case list.AclPermissionsNone:
		return "none"
	case list.AclPermissionsReader:
		return "read"
	case list.AclPermissionsWriter:
		return "write"
	case list.AclPermissionsAdmin:
		return "admin"
	case list.AclPermissionsOwner:
		return "owner"
	}
	return "unknown"
}

func convStatusToString(status list.AclStatus) string {
	switch status {
	case list.StatusJoining:
		return "joining"
	case list.StatusActive:
		return "active"
	case list.StatusDeclined:
		return "declined"
	case list.StatusRemoved:
		return "removed"
	case list.StatusRemoving:
		return "removing"
	case list.StatusCanceled:
		return "canceled"
	case list.StatusNone:
		return "none"
	}
	return "unknown"
}
