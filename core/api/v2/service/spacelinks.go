package v2service

import (
	"context"
	"net/url"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// A cross-space object reference is the platform's own two-parameter deep
// link, `anytype://object?objectId=<id>&spaceId=<space id>`
// (core/block/export/writer.go). The codec keeps it as an ordinary Link
// mark, param verbatim — a second parameter makes it not a same-space
// object link, and nothing guesses. The API serves space ids SHORT (§8.35)
// and accepts the short form anywhere a space id is accepted, so a caller
// echoing a served space reference into such a link must get a link the
// client can open: the short reference is expanded to the full id here,
// on every path that turns caller text into marks.
const (
	crossSpaceLinkPrefix = "anytype://object?"
	crossSpaceLinkParam  = "spaceId"
)

// spaceRefExpander resolves a space reference to its full id for the
// caller's visible spaces, or returns it unchanged when it does not resolve
// (the link stays verbatim — lossless, and the client's own refusal
// answers it).
func (s *Service) spaceRefExpander(ctx context.Context) func(string) string {
	return func(ref string) string {
		full, err := s.ResolveSpaceRef(ctx, ref)
		if err != nil || full == "" {
			return ref
		}
		return full
	}
}

// expandSpaceRefInObjectLink rewrites the spaceId parameter of a
// cross-space object link through resolve; any other destination is
// returned untouched.
func expandSpaceRefInObjectLink(dest string, resolve func(string) string) string {
	if !strings.HasPrefix(dest, crossSpaceLinkPrefix) {
		return dest
	}
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "anytype" || u.Host != "object" {
		return dest
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q[crossSpaceLinkParam]) != 1 {
		return dest
	}
	ref := q[crossSpaceLinkParam][0]
	full := resolve(ref)
	if full == ref {
		return dest
	}
	q.Set(crossSpaceLinkParam, full)
	u.RawQuery = q.Encode()
	return u.String()
}

// expandSpaceRefsInMarks rewrites the Link marks that are cross-space object
// links, in place.
func expandSpaceRefsInMarks(marks []*model.BlockContentTextMark, resolve func(string) string) {
	for _, mark := range marks {
		if mark == nil || mark.Type != model.BlockContentTextMark_Link {
			continue
		}
		mark.Param = expandSpaceRefInObjectLink(mark.Param, resolve)
	}
}

// expandSpaceRefsInBlocks is expandSpaceRefsInMarks over every text block.
func expandSpaceRefsInBlocks(blocks []*model.Block, resolve func(string) string) {
	for _, block := range blocks {
		text := block.GetText()
		if text == nil || text.Marks == nil {
			continue
		}
		expandSpaceRefsInMarks(text.Marks.Marks, resolve)
	}
}

// spaceRefExpander is the applier's expander: the resolvers carry the
// request's context; without them (a read-only applier) nothing expands.
func (a *v2StateApplier) spaceRefExpander() func(string) string {
	if a.resolvers == nil {
		return func(ref string) string { return ref }
	}
	return a.s.spaceRefExpander(a.resolvers.ctx)
}
