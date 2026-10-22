package v2handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// RespondError writes the C6 error envelope and aborts the request.
// Errors that are not *v2model.Error become 500 internal_error.
func RespondError(c *gin.Context, err error) {
	var v2Err *v2model.Error
	if !errors.As(err, &v2Err) {
		v2Err = v2model.NewError(http.StatusInternalServerError, v2model.CodeInternalError, err.Error())
	}
	c.AbortWithStatusJSON(v2Err.Status, echoSpaceRef(c, v2Err))
}

// echoSpaceRef re-spells the resolved full space id back into the short
// reference the caller actually used (§8.35), across the message, every
// issue message and every hint. It runs HERE, at the one place a v2 error
// becomes bytes, rather than at the twenty Sprintf sites that interpolate a
// space id into a refusal or a repair URL — one hook cannot drift from
// nineteen others, and a message added tomorrow inherits it.
//
// It is a no-op unless the resolution middleware recorded an echo for this
// request, which it does only when the caller's spelling differed from the
// full id. Substituting a short reference into a repair URL keeps the URL
// valid: every /v2 route that takes a space id takes either spelling.
func echoSpaceRef(c *gin.Context, v2Err *v2model.Error) *v2model.Error {
	if c.Request == nil {
		return v2Err
	}
	full, ref, ok := v2service.SpaceEchoFromCtx(c.Request.Context())
	if !ok {
		return v2Err
	}
	echoed := *v2Err
	echoed.Message = strings.ReplaceAll(echoed.Message, full, ref)
	if len(v2Err.Issues) > 0 {
		echoed.Issues = make([]v2model.Issue, len(v2Err.Issues))
		for i, issue := range v2Err.Issues {
			issue.Message = strings.ReplaceAll(issue.Message, full, ref)
			// the typed references must keep spelling exactly what the hint
			// spells (v2model.Ref): a reference's space id is re-spelled, and
			// the hint's rendering of it with it
			before := issue.SeeAlso
			issue.SeeAlso = echoRefs(before, full, ref)
			issue.Hint = echoHint(issue.Hint, full, ref, before, issue.SeeAlso)
			echoed.Issues[i] = issue
		}
	}
	return &echoed
}

// echoRefs returns copies of refs with the space_id binding re-spelled to
// the caller's reference when it is the resolved id. Only that binding: any
// other parameter or query value is an identity of its own — a property key
// or option name that contains, or even equals, the space id names a
// different thing once shortened (round-two and round-three reviews).
func echoRefs(refs []v2model.Ref, full, ref string) []v2model.Ref {
	if len(refs) == 0 {
		return refs
	}
	out := make([]v2model.Ref, len(refs))
	for i, r := range refs {
		out[i] = r
		if r.Params["space_id"] != full {
			continue
		}
		params := make(map[string]string, len(r.Params))
		for k, v := range r.Params {
			params[k] = v
		}
		params["space_id"] = ref
		out[i].Params = params
	}
	return out
}

// echoHint re-spells a hint for the echo: each reference's rendering (as it
// was) becomes the rendering of the echoed reference, and the prose between
// the renderings gets the plain substitution. Done span-wise so a value
// inside a rendering that merely contains the id is left alone, exactly as
// echoRefs leaves it — the hint and its references keep agreeing.
func echoHint(hint, full, ref string, before, after []v2model.Ref) string {
	if hint == "" {
		return hint
	}
	if len(before) == 0 {
		return strings.ReplaceAll(hint, full, ref)
	}
	renders := make(map[string]string, len(before))
	patterns := make([]string, 0, len(before))
	for i, r := range before {
		rest := r.String()
		if rest == "" {
			continue
		}
		if _, seen := renders[rest]; !seen {
			patterns = append(patterns, rest)
		}
		renders[rest] = after[i].String()
	}
	if len(patterns) == 0 {
		return strings.ReplaceAll(hint, full, ref)
	}
	// longest first, so a rendering that extends another wins where both match
	sort.SliceStable(patterns, func(i, j int) bool { return len(patterns[i]) > len(patterns[j]) })
	for i, p := range patterns {
		patterns[i] = regexp.QuoteMeta(p)
	}
	spans := regexp.MustCompile(strings.Join(patterns, "|"))
	var b strings.Builder
	last := 0
	for _, m := range spans.FindAllStringIndex(hint, -1) {
		b.WriteString(strings.ReplaceAll(hint[last:m[0]], full, ref))
		b.WriteString(renders[hint[m[0]:m[1]]])
		last = m[1]
	}
	b.WriteString(strings.ReplaceAll(hint[last:], full, ref))
	return b.String()
}

// decodeStrictJSONBody decodes a v2 request body strictly: unknown fields
// are 400s with the field named (C13's spirit at the request layer), an
// empty body 400s with the hint, and an oversized body 413s naming the
// surface. A false return means the error response was already written.
func decodeStrictJSONBody(c *gin.Context, into any, hint v2model.Hint, maxBody int64, surface string) bool {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBody+1))
	if err != nil {
		RespondError(c, v2model.ValidationFailed("read request body",
			v2model.Issue{Message: err.Error()}))
		return false
	}
	if int64(len(body)) > maxBody {
		RespondError(c, v2model.RequestTooLarge(
			fmt.Sprintf("%s request body exceeds the %d-byte limit", surface, maxBody)))
		return false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		RespondError(c, v2model.ValidationFailed("request body is required",
			v2model.Issue{Message: "the body is empty"}.WithHint(hint)))
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		issue := v2model.Issue{Message: err.Error()}.WithHint(hint)
		if field, ok := unknownFieldName(err); ok {
			issue.Path = "/" + field
		}
		RespondError(c, v2model.ValidationFailed("invalid request body", issue))
		return false
	}
	return true
}
