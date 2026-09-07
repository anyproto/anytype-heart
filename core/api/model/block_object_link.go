package apimodel

// SetBlockObjectLinkRequest sets or updates an object link on a block (UI-equivalent: link block / text→link).
type SetBlockObjectLinkRequest struct {
	TargetObjectId string `json:"target_object_id" binding:"required" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"`
	// LinkStyle embed layout: page (default for new from text), dataview, dashboard, archive. Omit to keep existing on link blocks or use page for new text→link.
	LinkStyle string `json:"link_style,omitempty" example:"dashboard"`
	// CardStyle presentation: text, card (default for new from text), inline. Omit to keep existing on link blocks or use card for new text→link.
	CardStyle string `json:"card_style,omitempty" example:"card"`
	// SyncLinkPresentationFromBlockId, if set, must be the id of another link block on the same page. IconSize, Description, Relations, Fields and (unless link_style / card_style are set) Style and CardStyle are copied from that block so the result matches a manually tuned card (e.g. description snippet + type). target_object_id is always taken from this request.
	SyncLinkPresentationFromBlockId string `json:"sync_link_presentation_from_block_id,omitempty" example:"69e29106f6ec12739aaf32e6"`
	// BackgroundColor sets the block highlight (palette keys only: grey, yellow, orange, red, pink, purple, blue, ice, teal, lime — same as tags; UI «Зелёный» = lime, not "green"). Pointer: null/absent = unchanged, "" = clear.
	BackgroundColor *string `json:"background_color,omitempty" extensions:"nullable" example:"lime"`
	// IconSize: none, small, medium. Omit to keep existing.
	IconSize string `json:"icon_size,omitempty" example:"medium"`
	// LinkDescription: none, added, content. Omit to keep existing.
	LinkDescription string `json:"link_description,omitempty" example:"content"`
	// Relations lists relation keys to show on the card (e.g. "github_stars", "tag"). Omit to keep existing.
	Relations []string `json:"relations,omitempty" example:"[\"github_stars\",\"tag\"]"`
}

// SetBlockObjectLinkResponse returns the block id after replace (may differ from the request path id).
type SetBlockObjectLinkResponse struct {
	Object   string `json:"object" example:"block_object_link"`
	BlockId  string `json:"block_id"`
	ObjectId string `json:"object_id"`
	SpaceId  string `json:"space_id"`
	TargetId string `json:"target_object_id"`
	Replaced bool   `json:"replaced"` // false when target was already set (idempotent)
}
