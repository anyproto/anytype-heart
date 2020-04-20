package bookmark

import (
	"fmt"
	"regexp"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
)

var (
	// RFC 5322 mail regex
	noPrefixEmailRegexp = regexp.MustCompile(`^(?:[a-z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+(?:\.[a-z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+)*|"(?:[\x01-\x08\x0b\x0c\x0e-\x1f\x21\x23-\x5b\x5d-\x7f]|\\[\x01-\x09\x0b\x0c\x0e-\x7f])*")@(?:(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]*[a-z0-9])?|\[(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?|[a-z0-9-]*[a-z0-9]:(?:[\x01-\x08\x0b\x0c\x0e-\x1f\x21-\x5a\x53-\x7f]|\\[\x01-\x09\x0b\x0c\x0e-\x7f])+)\])$`)
	// RFC 3966 tel regex
	noPrefixTelRegexp  = regexp.MustCompile(`^((?:\+[\d().-]*\d[\d().-]*|[0-9A-F*#().-]*[0-9A-F*#][0-9A-F*#().-]*(?:;[a-z\d-]+(?:=(?:[a-z\d\[\]\/:&+$_!~*'().-]|%[\dA-F]{2})+)?)*;phone-context=(?:\+[\d().-]*\d[\d().-]*|(?:[a-z0-9]\.|[a-z0-9][a-z0-9-]*[a-z0-9]\.)*(?:[a-z]|[a-z][a-z0-9-]*[a-z0-9])))(?:;[a-z\d-]+(?:=(?:[a-z\d\[\]\/:&+$_!~*'().-]|%[\dA-F]{2})+)?)*(?:,(?:\+[\d().-]*\d[\d().-]*|[0-9A-F*#().-]*[0-9A-F*#][0-9A-F*#().-]*(?:;[a-z\d-]+(?:=(?:[a-z\d\[\]\/:&+$_!~*'().-]|%[\dA-F]{2})+)?)*;phone-context=\+[\d().-]*\d[\d().-]*)(?:;[a-z\d-]+(?:=(?:[a-z\d\[\]\/:&+$_!~*'().-]|%[\dA-F]{2})+)?)*)*)$`)
	noPrefixHttpRegex  = regexp.MustCompile(`^[\w.-]+(?:\.[\w\.-]+)+[\w\-\._~:/?#[\]@!\$&'\(\)\*\+,;=.]+$`)
	haveUriSchemeRegex = regexp.MustCompile(`^([a-zA-Z][A-Za-z0-9+.-]*):[\S]+`)
)

func NewBookmark(sb smartblock.SmartBlock, lp linkpreview.LinkPreview, ctrl DoBookmark) Bookmark {
	return &sbookmark{SmartBlock: sb, lp: lp, ctrl: ctrl}
}

type Bookmark interface {
	Fetch(ctx *state.Context, id string, url string) (err error)
	CreateAndFetch(ctx *state.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (newId string, err error)
	UpdateBookmark(id string, apply func(b bookmark.Block) error) (err error)
}

type DoBookmark interface {
	DoBookmark(id string, apply func(b Bookmark) error) error
}

type sbookmark struct {
	smartblock.SmartBlock
	lp   linkpreview.LinkPreview
	ctrl DoBookmark
}

func (b *sbookmark) Fetch(ctx *state.Context, id string, url string) (err error) {
	s := b.NewStateCtx(ctx)
	if err = b.fetch(s, id, url); err != nil {
		return
	}
	return b.Apply(s)
}

func (b *sbookmark) processUrl(url string) (urlOut string, err error) {
	if len(url) == 0 {
		return url, fmt.Errorf("url is empty")

	} else if noPrefixEmailRegexp.MatchString(url) {
		return "mailto:" + url, nil

	} else if noPrefixTelRegexp.MatchString(url) {
		return "tel:" + url, nil

	} else if noPrefixHttpRegex.MatchString(url) {
		return "http://" + url, nil

	} else if haveUriSchemeRegex.MatchString(url) {
		return url, nil
	}

	return url, fmt.Errorf("not a uri")
}

func (b *sbookmark) fetch(s *state.State, id, url string) (err error) {
	bb := s.Get(id)
	if b == nil {
		return smartblock.ErrSimpleBlockNotFound
	}

	url, err = b.processUrl(url)
	if err != nil {
		return err
	}

	if bm, ok := bb.(bookmark.Block); ok {
		return bm.Fetch(bookmark.FetchParams{
			Url:     url,
			Anytype: b.Anytype(),
			Updater: func(id string, apply func(b bookmark.Block) error) (err error) {
				return b.ctrl.DoBookmark(b.Id(), func(b Bookmark) error {
					return b.UpdateBookmark(id, apply)
				})
			},
			LinkPreview: b.lp,
		})
	}
	return fmt.Errorf("unexpected simple bock type: %T (want Bookmark)", bb)
}

func (b *sbookmark) CreateAndFetch(ctx *state.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (newId string, err error) {
	s := b.NewStateCtx(ctx)
	nb := simple.New(&model.Block{
		Content: &model.BlockContentOfBookmark{
			Bookmark: &model.BlockContentBookmark{
				Url: req.Url,
			},
		},
	})
	s.Add(nb)
	newId = nb.Model().Id
	if err = s.InsertTo(req.TargetId, req.Position, newId); err != nil {
		return
	}
	if err = b.fetch(s, newId, req.Url); err != nil {
		return
	}
	if err = b.Apply(s); err != nil {
		return
	}
	return
}

func (b *sbookmark) UpdateBookmark(id string, apply func(b bookmark.Block) error) (err error) {
	s := b.NewState()
	if bb := s.Get(id); bb != nil {
		if bm, ok := bb.(bookmark.Block); ok {
			if err = apply(bm); err != nil {
				return
			}
		} else {
			return fmt.Errorf("unexpected simple bock type: %T (want Bookmark)", bb)
		}
	} else {
		return smartblock.ErrSimpleBlockNotFound
	}
	return b.Apply(s, smartblock.NoHistory)
}
