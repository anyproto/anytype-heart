package v2model

// PublishRequest selects the page address and whether to include a space invite.
type PublishRequest struct {
	// Last segment of https://any.coop/{identity}/{slug}. A named subdomain requires a plan with a registered Any Name. Use A–Z, a–z, digits, hyphens, underscores, periods or tildes. The segments . and .. are reserved.
	Slug string `json:"slug" binding:"required" example:"meeting-notes"`
	// Request shared-space details and an existing invite. Defaults to false. The invite must be available on this device and require approval or grant at most read access. Check invite_included for the outcome.
	IncludeInvite bool `json:"include_invite,omitempty"`
}

type PublishResult struct {
	// Public URL returned after publishing.
	URL string `json:"url,omitempty"`
	// Whether the published snapshot contains a space invite link. Omitted on a dry run.
	InviteIncluded *bool `json:"invite_included,omitempty"`
	// True when the request was checked without exporting or uploading.
	DryRun bool `json:"dry_run,omitempty"`
}

type Publication struct {
	// Source object id. Use this as publication_id to remove the public copy.
	ObjectId string `json:"object_id"`
	Slug     string `json:"slug"`
	URL      string `json:"url"`
	// Total published bytes, including files.
	SizeBytes int64 `json:"size_bytes"`
	// The include_invite setting used to publish this snapshot.
	IncludeInvite bool `json:"include_invite"`
	// Whether the public snapshot contains an invite link. Omitted when an older publication did not record the outcome.
	InviteIncluded *bool `json:"invite_included,omitempty"`
}

type RemovePublicationResult struct {
	ObjectId string `json:"object_id"`
	DryRun   bool   `json:"dry_run,omitempty"`
}
