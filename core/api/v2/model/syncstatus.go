package v2model

// The sync status read (GET /v2/spaces/{space_id}/sync) and the storage
// report an upload carries. Both serve what an agent needs to answer "is my
// data syncing?" and "why is this file not backed up?": the status the app
// shows, the counts behind it, and the quota the space's files count against.

// The wire values of SyncStatus.Status.
const (
	SyncStatusSynced             = "synced"
	SyncStatusSyncing            = "syncing"
	SyncStatusOffline            = "offline"
	SyncStatusError              = "error"
	SyncStatusNetworkNeedsUpdate = "network_needs_update"
	SyncStatusLocalOnly          = "local_only"
	SyncStatusConnecting         = "connecting"
)

// The wire values of SyncStatus.Network.
const (
	SyncNetworkAnytype    = "anytype"
	SyncNetworkSelfHosted = "self_hosted"
	SyncNetworkLocalOnly  = "local_only"
)

// The wire values of SyncStatus.Error.
const (
	SyncErrorNetwork             = "network_error"
	SyncErrorIncompatibleVersion = "incompatible_version"
)

// The wire values of PendingObject.Status.
const (
	PendingObjectSyncing     = "syncing"
	PendingObjectQueued      = "queued"
	PendingObjectError       = "error"
	PendingObjectNotBackedUp = "not_backed_up"
)

// SyncStatus is a space's sync state. Field descriptions are served in the
// OpenAPI document, so they name wire fields and never the Go ones. Storage
// carries no field comment: swag moves a comment on a $ref field onto the
// shared schema.
type SyncStatus struct {
	SpaceId              string          `json:"space_id"`
	Status               string          `json:"status" enums:"synced,syncing,offline,error,network_needs_update,local_only,connecting"` // How objects sync, as the app shows it. local_only: nothing is backed up to a node, though devices on one network still sync. connecting: this device has not reached the backup node yet. Files refused for the storage limit do not change it.
	Network              string          `json:"network" enums:"anytype,self_hosted,local_only"`                                         // Where the space is backed up: the Anytype network, a self-hosted one, or no node.
	Error                string          `json:"error,omitempty" enums:"network_error,incompatible_version"`                             // Why the status is offline or error.
	PendingObjectCount   int             `json:"pending_object_count"`                                                                   // Objects not yet in sync with the backup node, as the app counts them. It includes objects not downloaded to this device yet, which objects does not list.
	UploadingFileCount   int             `json:"uploading_file_count"`                                                                   // This account's files in the space still uploading.
	NotBackedUpFileCount int             `json:"not_backed_up_file_count"`                                                               // Files that did not fit in the storage limit. They stay on the device that added them, and upload from it once there is room.
	UpgradeNeeded        bool            `json:"upgrade_needed"`                                                                         // The space is short of storage by the backup node's own report, the caller pays for it on the Anytype network, and the account's whole limit is its plan's, so an upgrade raises it.
	Storage              *Storage        `json:"storage,omitempty"`
	Objects              []PendingObject `json:"objects,omitempty"`  // What is not in sync on this device, most recently changed first, paged by offset and limit. Absent with count_only=true, in local-only mode, or when nothing is pending.
	HasMore              bool            `json:"has_more,omitempty"` // More objects past this page.
	Notes                []string        `json:"notes,omitempty"`    // One sentence each on what is wrong with the space's sync or storage and what fixes it.
	Warnings             []Issue         `json:"warnings,omitempty"`
}

// Storage is the quota a space's files count against: the space owner's,
// and in a one-to-one space each member's own. Its numbers are estimates:
// a file is counted at its original's size, so an image's resized copies
// are left out. It is absent in local-only mode and until the backup node
// has reported the quota.
type Storage struct {
	UsedBytes    int64 `json:"used_bytes"`    // Usage against this limit: the backup node's last report and the uploads it confirmed since.
	PendingBytes int64 `json:"pending_bytes"` // This account's files in the space still waiting to upload. The node has not counted them yet.
	LimitBytes   int64 `json:"limit_bytes"`   // The storage limit of whoever pays for the space.
	LeftBytes    int64 `json:"left_bytes"`    // What is left once the pending files that fit have uploaded. A file that does not fit is refused without taking room.
}

// PendingObject is one object or file not in sync with the backup node.
type PendingObject struct {
	Id     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status" enums:"syncing,queued,error,not_backed_up"` // not_backed_up: a file that did not fit in the storage limit.
	Error  string `json:"error,omitempty" enums:"network_error"`             // Why the status is error.
}
