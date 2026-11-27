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
	Status               string          `json:"status" enums:"synced,syncing,offline,error,network_needs_update,local_only,connecting"` // The app's object sync status. In local_only, devices sync on one network without node backup. While connecting, this device hasn't reached the backup node. Quota refusals don't affect status.
	Network              string          `json:"network" enums:"anytype,self_hosted,local_only"`                                         // Backup network, or local_only for none.
	Error                string          `json:"error,omitempty" enums:"network_error,incompatible_version"`                             // Cause of offline or error.
	PendingObjectCount   int             `json:"pending_object_count"`                                                                   // App count of objects out of sync with the backup node. Includes objects not yet downloaded, which `objects` omits.
	UploadingFileCount   int             `json:"uploading_file_count"`                                                                   // This account's files uploading here.
	NotBackedUpFileCount int             `json:"not_backed_up_file_count"`                                                               // Files refused for lack of room stay on the device that added them and upload once they fit.
	UpgradeNeeded        bool            `json:"upgrade_needed"`
	Storage              *Storage        `json:"storage,omitempty"`
	Objects              []PendingObject `json:"objects,omitempty"`  // This device's out-of-sync objects, last changed first. Page with objects=true, offset and limit. Omitted in local_only or when empty.
	HasMore              bool            `json:"has_more,omitempty"` // More objects follow.
	Notes                []string        `json:"notes,omitempty"`    // One sentence per sync or storage issue and its fix.
	Warnings             []Issue         `json:"warnings,omitempty"`
}

// Storage is the quota a space's files count against: the space owner's,
// and in a one-to-one space each member's own. Its numbers are estimates:
// a file is counted at its original's size, so an image's resized copies
// are left out. It is absent in local-only mode and until the backup node
// has reported the quota.
type Storage struct {
	UsedBytes    int64 `json:"used_bytes"`    // Node's last reported usage plus uploads it confirmed since.
	PendingBytes int64 `json:"pending_bytes"` // This account's files here awaiting upload, still uncounted by the node.
	LimitBytes   int64 `json:"limit_bytes"`   // Payer's storage limit.
	LeftBytes    int64 `json:"left_bytes"`    // Free space after pending uploads that fit. Refused files take no room.
}

// PendingObject is one object or file not in sync with the backup node.
type PendingObject struct {
	Id     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status" enums:"syncing,queued,error,not_backed_up"` // not_backed_up means the file didn't fit the quota.
	Error  string `json:"error,omitempty" enums:"network_error"`             // Cause of the error.
}
