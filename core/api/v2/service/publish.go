package v2service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"

	"github.com/anyproto/anytype-publish-server/publishclient/publishapi"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
)

// A slug is one URL path segment. Heart appends it verbatim to the account URL.
var publishSlug = regexp.MustCompile(`^[a-zA-Z0-9._~-]+$`)

func (s *Service) PublishObject(ctx context.Context, spaceId, objectId string, req v2model.PublishRequest, dryRun bool) (*v2model.PublishResult, error) {
	if err := s.ensureSpaceWritable(ctx, spaceId); err != nil {
		return nil, err
	}
	if !publishSlug.MatchString(req.Slug) || req.Slug == "." || req.Slug == ".." {
		return nil, v2model.ValidationFailed("provide a slug for the public URL",
			v2model.Issue{Path: "/slug", Message: "use letters A–Z or a–z, digits, hyphens, underscores, periods or tildes, for example meeting-notes. A slug cannot be . or .."})
	}
	if err := s.ensureSpaceLoaded(ctx, spaceId); err != nil {
		return nil, err
	}
	read, err := s.readAddressed(ctx, spaceId, objectId)
	if err != nil {
		return nil, err
	}
	inBin, err := s.reader.InBin(ctx, spaceId, objectId)
	if err != nil {
		return nil, fmt.Errorf("check archived state before publishing: %w", err)
	}
	if inBin || isArchivedRead(read) {
		return nil, v2model.ObjectArchived(spaceId, objectId, deletableSbType(read.SbType), dryRun)
	}
	if read.PublishRefused != nil {
		return nil, v2model.NewError(http.StatusForbidden, v2model.CodeForbidden, "this object's restrictions prevent web publishing")
	}
	if s.publisher == nil {
		return nil, publishingUnavailable()
	}
	if dryRun {
		return &v2model.PublishResult{DryRun: true}, nil
	}
	result, err := s.publisher.Publish(ctx, spaceId, objectId, req.Slug, req.IncludeInvite)
	if err != nil {
		return nil, publishingError(err)
	}
	return &v2model.PublishResult{URL: result.Url, InviteIncluded: &result.InviteIncluded}, nil
}

func (s *Service) ListPublications(ctx context.Context, spaceId string, offset, limit int) (*v2model.ListResponse[v2model.Publication], error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, err
	}
	if s.publisher == nil {
		return nil, publishingUnavailable()
	}
	pages, err := s.publisher.ListPublications(ctx, spaceId)
	if err != nil {
		return nil, publishingError(err)
	}
	slices.SortFunc(pages, func(a, b domain.Publication) int {
		return cmp.Or(cmp.Compare(a.Slug, b.Slug), cmp.Compare(a.ObjectId, b.ObjectId))
	})
	page, more := pagination.Paginate(pages, offset, limit)
	rows := make([]v2model.Publication, 0, len(page))
	for _, p := range page {
		rows = append(rows, v2model.Publication{ObjectId: p.ObjectId, URL: p.URL, Slug: p.Slug, SizeBytes: p.SizeBytes,
			IncludeInvite: p.JoinSpace, InviteIncluded: p.InviteIncluded})
	}
	list := v2model.NewListResponse(rows, len(pages), offset, limit, more, "request the next offset")
	return &list, nil
}

func (s *Service) RemovePublication(ctx context.Context, spaceId, objectId string, dryRun bool) (*v2model.RemovePublicationResult, error) {
	if err := s.ensureSpaceWritable(ctx, spaceId); err != nil {
		return nil, err
	}
	if s.publisher == nil {
		return nil, publishingUnavailable()
	}
	// A public copy can outlive its local object. Authorize the space, then
	// address the authenticated publisher's record by both space and object.
	state, err := s.publisher.GetStatus(ctx, spaceId, objectId)
	if err != nil {
		return nil, publishingError(err)
	}
	if state.GetSpaceId() != spaceId || state.GetObjectId() != objectId {
		return nil, v2model.NotFound("publication not found in this space")
	}
	if !dryRun {
		if err := s.publisher.Unpublish(ctx, spaceId, objectId); err != nil {
			return nil, publishingError(err)
		}
	}
	return &v2model.RemovePublicationResult{ObjectId: objectId, DryRun: dryRun}, nil
}

func publishingUnavailable() error {
	return v2model.NewError(http.StatusNotImplemented, v2model.CodeNotImplemented, "web publishing is unavailable on this server")
}

func publishingError(err error) error {
	switch {
	case errors.Is(err, domain.ErrPublishURLTaken):
		return v2model.NewError(http.StatusConflict, v2model.CodePublishURLTaken, "another published page uses this URL",
			v2model.Issue{Path: "/slug", Message: "choose a different slug"})
	case errors.Is(err, domain.ErrPublishLimitExceeded):
		return v2model.NewError(http.StatusRequestEntityTooLarge, v2model.CodePublicationTooLarge,
			"the snapshot and its files exceed the publication size limit. The standard hosted limits are 10 MiB without a registered Any Name and 100 MiB with one. Reduce the included content and publish again.")
	case errors.Is(err, publishapi.ErrNotFound):
		return v2model.NotFound("publication not found in this space")
	case errors.Is(err, publishapi.ErrAccessDenied):
		return v2model.NewError(http.StatusForbidden, v2model.CodeForbidden, "the publishing service denied access to this publication")
	default:
		return v2model.NewError(http.StatusInternalServerError, v2model.CodeInternalError, "the publishing operation failed")
	}
}
