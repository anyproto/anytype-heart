package v2service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// A cross-space object reference is the platform's own two-parameter deep
// link, `anytype://object?objectId=<id>&spaceId=<space id>`
// (core/block/export/writer.go). The codec keeps it as an ordinary Link
// mark, destination verbatim — a second parameter makes it not a same-space
// object link, and nothing guesses. The API serves space ids SHORT (§8.35)
// and accepts the short form anywhere a space id is accepted, so a caller
// echoing a served space reference into such a link must get a link the
// client can open: the short reference is expanded to the full id on every
// path that turns caller text into marks.
const (
	crossSpaceLinkPrefix = "anytype://object?"
	crossSpaceLinkParam  = "spaceId"
	// maxCrossSpaceLinkDest is the codec's destination bound (INLINE_MARKUP:
	// 2048 code points AS SPELLED in the document). Expanding a short
	// reference makes a destination longer, so a link that parsed could
	// become one the renderer refuses to write — and export then fails with
	// no partial output. Such a link keeps its short reference instead, with
	// the warning below.
	maxCrossSpaceLinkDest = 2048
)

// spaceLinkExpander rewrites the spaceId of cross-space object links for one
// request. It exists as a value rather than a bare function because both of
// its jobs are per-request: ResolveSpaceRef reads the visible-space census
// from the store for any reference that is not already a full id, and a
// document can carry hundreds of links, so every distinct reference is
// resolved once; and a reference it could NOT expand is reported to the
// caller rather than silently stored (round-six review — a stored link that
// quietly does not open is the R6-1 failure again).
type spaceLinkExpander struct {
	svc      *Service
	ctx      context.Context
	resolved map[string]string
	issues   []v2model.Issue
	// reported keeps one warning per reference, however many links carry it
	reported map[string]bool
}

func (s *Service) newSpaceLinkExpander(ctx context.Context) *spaceLinkExpander {
	return &spaceLinkExpander{svc: s, ctx: ctx, resolved: map[string]string{}, reported: map[string]bool{}}
}

// Blocks expands every text block's Link marks.
func (e *spaceLinkExpander) Blocks(blocks []*model.Block) {
	for _, block := range blocks {
		text := block.GetText()
		if text == nil || text.Marks == nil {
			continue
		}
		e.Marks(text.Marks.Marks)
	}
}

// Marks expands the Link marks that are cross-space object links, in place.
// An Object mark's param is an object id and a Mention's is an object id
// too, so neither is touched.
func (e *spaceLinkExpander) Marks(marks []*model.BlockContentTextMark) {
	for _, mark := range marks {
		if mark == nil || mark.Type != model.BlockContentTextMark_Link {
			continue
		}
		mark.Param = e.destination(mark.Param)
	}
}

// Warnings returns what could not be expanded, addressed at path. Empty when
// every link resolved, which is the ordinary case.
func (e *spaceLinkExpander) Warnings(path string) []v2model.Issue {
	if len(e.issues) == 0 {
		return nil
	}
	out := make([]v2model.Issue, len(e.issues))
	for i, issue := range e.issues {
		issue.Path = path
		out[i] = issue
	}
	return out
}

// destination rewrites the spaceId parameter of a cross-space object link;
// any other destination is returned untouched, byte for byte.
func (e *spaceLinkExpander) destination(dest string) string {
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
	full := e.resolve(ref)
	if full == ref {
		return dest
	}
	q.Set(crossSpaceLinkParam, full)
	u.RawQuery = q.Encode()
	expanded := u.String()
	if spelledDestLen(expanded) > maxCrossSpaceLinkDest {
		// storing it would produce a document the renderer cannot write
		e.warn(ref, v2model.Issue{
			Message: "expanding the space reference in an object link would make its destination too long to write back, so the link keeps the reference as sent",
		}.Hintf("shorten the link's destination, or write the full space id — %s prints it", v2model.RefListSpaces()))
		return dest
	}
	return expanded
}

// resolve is ResolveSpaceRef memoized per reference, with the diagnostics
// ResolveSpaceRef would have given a path parameter kept as warnings: the
// link is preserved either way, but the caller is told it will not open.
func (e *spaceLinkExpander) resolve(ref string) string {
	if full, ok := e.resolved[ref]; ok {
		return full
	}
	full, err := e.svc.ResolveSpaceRef(e.ctx, ref)
	switch {
	case err != nil:
		// ambiguous: the refusal a path parameter gets, as a warning here
		full = ref
		message := "the reference matches more than one space"
		var v2Err *v2model.Error
		if errors.As(err, &v2Err) && len(v2Err.Issues) > 0 {
			message = v2Err.Issues[0].Message
		}
		e.warn(ref, v2model.Issue{
			Message: fmt.Sprintf("the space reference %q in an object link is ambiguous, so the link keeps it as sent: %s", ref, message),
		}.Hintf("use a longer reference, or the id exactly as %s prints it", v2model.RefListSpaces()))
	case full == ref && !isSpaceIdShaped(ref):
		e.warn(ref, v2model.Issue{
			Message: fmt.Sprintf("the space reference %q in an object link names no space this key can read, so the link keeps it as sent and will not open", ref),
		}.Hintf("use a space id exactly as %s prints it, or the full id", v2model.RefListSpaces()))
	}
	e.resolved[ref] = full
	return full
}

func (e *spaceLinkExpander) warn(ref string, issue v2model.Issue) {
	if e.reported[ref] {
		return
	}
	e.reported[ref] = true
	e.issues = append(e.issues, issue)
}

// spelledDestLen is the length of a destination as the renderer WRITES it:
// code points, plus one backslash for each character it escapes, plus the
// angle wrapping whitespace forces (any-block inline.go escapeDest). It
// over-counts nothing that matters and never under-counts.
func spelledDestLen(dest string) int {
	n := utf8.RuneCountInString(dest)
	for _, r := range dest {
		switch r {
		case '\\', '(', ')', '&', '<', '[', ']', '`':
			n++
		case ' ', '\t', '\n':
			n += 2 // the angle brackets, counted once each is enough
		}
	}
	return n
}
