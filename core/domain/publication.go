package domain

import "errors"

var ErrPublishLimitExceeded = errors.New("limit exceeded")
var ErrPublishURLTaken = errors.New("url is already taken by another page")

type PublishResult struct {
	Url            string
	InviteIncluded bool
}

type Publication struct {
	ObjectId  string
	Slug      string
	URL       string
	SizeBytes int64
	JoinSpace bool
	// Nil for older versions that requested an invite without recording the outcome.
	InviteIncluded *bool
}
