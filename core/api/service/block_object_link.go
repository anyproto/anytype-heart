package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/pbtypes"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
)

var allowedBlockBackgroundColors = map[string]struct{}{
	string(constant.ColorGrey):   {},
	string(constant.ColorYellow): {},
	string(constant.ColorOrange): {},
	string(constant.ColorRed):    {},
	string(constant.ColorPink):   {},
	string(constant.ColorPurple): {},
	string(constant.ColorBlue):   {},
	string(constant.ColorIce):   {},
	string(constant.ColorTeal):  {},
	string(constant.ColorLime):  {},
}

// parseBlockHighlightColor returns a normalized palette key or error. Empty input means clear highlight.
func parseBlockHighlightColor(s string) (string, error) {
	v := strings.TrimSpace(strings.ToLower(s))
	if v == "" {
		return "", nil
	}
	if _, ok := allowedBlockBackgroundColors[v]; !ok {
		return "", util.ErrBadInput(fmt.Sprintf(
			`invalid background_color %q: use grey, yellow, orange, red, pink, purple, blue, ice, teal, lime (UI "green" / Зелёный = lime, not "green")`,
			s,
		))
	}
	return v, nil
}

var (
	ErrBlockNotFound           = errors.New("block not found in object")
	ErrBlockReplaceFailed      = errors.New("block replace failed")
	ErrBlockDeleteFailed       = errors.New("block delete failed")
	ErrRequiredBlock           = errors.New("cannot replace a required system block")
	ErrUnsupportedBlockForLink = errors.New("block content does not support object link (allowed: text, link)")
	ErrNotLinkBlock            = errors.New("block is not a link block")
	ErrTargetMismatch          = errors.New("link target does not match requested target_object_id")
)

func findBlockInView(ov *model.ObjectView, blockId string) *model.Block {
	if ov == nil {
		return nil
	}
	for _, b := range ov.Blocks {
		if b != nil && b.Id == blockId {
			return b
		}
	}
	return nil
}

func objectShowOrErr(s *Service, ctx context.Context, spaceId, objectId string) (*model.ObjectView, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: objectId,
	})
	if resp.Error == nil || resp.Error.Code == pb.RpcObjectShowResponseError_NULL {
		if resp.ObjectView == nil {
			return nil, ErrObjectNotFound
		}
		return resp.ObjectView, nil
	}
	switch resp.Error.Code {
	case pb.RpcObjectShowResponseError_NOT_FOUND:
		return nil, ErrObjectNotFound
	case pb.RpcObjectShowResponseError_OBJECT_DELETED:
		return nil, ErrObjectDeleted
	default:
		return nil, ErrFailedRetrieveObject
	}
}

func parseLinkStyle(s string) (model.BlockContentLinkStyle, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return model.BlockContentLink_Page, nil
	}
	switch s {
	case "page":
		return model.BlockContentLink_Page, nil
	case "dataview":
		return model.BlockContentLink_Dataview, nil
	case "dashboard":
		return model.BlockContentLink_Dashboard, nil
	case "archive":
		return model.BlockContentLink_Archive, nil
	default:
		return 0, util.ErrBadInput(fmt.Sprintf("invalid link_style %q (use page, dataview, dashboard, archive)", s))
	}
}

func linkStyleToAPIString(st model.BlockContentLinkStyle) string {
	switch st {
	case model.BlockContentLink_Page:
		return "page"
	case model.BlockContentLink_Dataview:
		return "dataview"
	case model.BlockContentLink_Dashboard:
		return "dashboard"
	case model.BlockContentLink_Archive:
		return "archive"
	default:
		return ""
	}
}

func cardStyleToAPIString(cs model.BlockContentLinkCardStyle) string {
	switch cs {
	case model.BlockContentLink_Text:
		return "text"
	case model.BlockContentLink_Card:
		return "card"
	case model.BlockContentLink_Inline:
		return "inline"
	default:
		return ""
	}
}

func iconSizeToAPIString(sz model.BlockContentLinkIconSize) string {
	switch sz {
	case model.BlockContentLink_SizeNone:
		return "none"
	case model.BlockContentLink_SizeSmall:
		return "small"
	case model.BlockContentLink_SizeMedium:
		return "medium"
	default:
		return ""
	}
}

func linkDescriptionToAPIString(d model.BlockContentLinkDescription) string {
	switch d {
	case model.BlockContentLink_None:
		return "none"
	case model.BlockContentLink_Added:
		return "added"
	case model.BlockContentLink_Content:
		return "content"
	default:
		return ""
	}
}

func parseCardStyle(s string) (model.BlockContentLinkCardStyle, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return model.BlockContentLink_Card, nil
	}
	switch s {
	case "text":
		return model.BlockContentLink_Text, nil
	case "card":
		return model.BlockContentLink_Card, nil
	case "inline":
		return model.BlockContentLink_Inline, nil
	default:
		return 0, util.ErrBadInput(fmt.Sprintf("invalid card_style %q (use text, card, inline)", s))
	}
}

func parseIconSize(s string) (model.BlockContentLinkIconSize, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return model.BlockContentLink_SizeNone, nil
	}
	switch s {
	case "none":
		return model.BlockContentLink_SizeNone, nil
	case "small":
		return model.BlockContentLink_SizeSmall, nil
	case "medium":
		return model.BlockContentLink_SizeMedium, nil
	default:
		return 0, util.ErrBadInput(fmt.Sprintf("invalid icon_size %q (use none, small, medium)", s))
	}
}

func parseLinkDescription(s string) (model.BlockContentLinkDescription, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return model.BlockContentLink_None, nil
	}
	switch s {
	case "none":
		return model.BlockContentLink_None, nil
	case "added":
		return model.BlockContentLink_Added, nil
	case "content":
		return model.BlockContentLink_Content, nil
	default:
		return 0, util.ErrBadInput(fmt.Sprintf("invalid link_description %q (use none, added, content)", s))
	}
}

func linkPresentationEqual(a, b *model.BlockContentLink) bool {
	if a == nil || b == nil {
		return false
	}
	if a.TargetBlockId != b.TargetBlockId ||
		a.Style != b.Style ||
		a.CardStyle != b.CardStyle ||
		a.IconSize != b.IconSize ||
		a.Description != b.Description {
		return false
	}
	if !slices.Equal(a.Relations, b.Relations) {
		return false
	}
	return reflect.DeepEqual(a.Fields, b.Fields)
}

// blockLinkReplaceEqual is true when link payload and block-level chrome (highlight color) match.
func blockLinkReplaceEqual(oldB, newB *model.Block) bool {
	if oldB == nil || newB == nil {
		return false
	}
	if oldB.BackgroundColor != newB.BackgroundColor {
		return false
	}
	oldL, newL := oldB.GetLink(), newB.GetLink()
	if oldL == nil || newL == nil {
		return false
	}
	return linkPresentationEqual(oldL, newL)
}

// applySyncBlockChrome copies block-level styling used by the link card (e.g. background highlight).
func applySyncBlockChrome(dst, ref *model.Block) {
	if dst == nil || ref == nil {
		return
	}
	raw := strings.TrimSpace(ref.BackgroundColor)
	if raw == "" {
		dst.BackgroundColor = ""
		return
	}
	v, err := parseBlockHighlightColor(raw)
	if err != nil {
		dst.BackgroundColor = ""
		return
	}
	dst.BackgroundColor = v
}

// applySyncLinkPresentation copies rich-card fields from ref onto dst (dst.TargetBlockId must already be set).
func applySyncLinkPresentation(dst, ref *model.BlockContentLink, linkStyleReq, cardStyleReq string) error {
	if dst == nil || ref == nil {
		return errors.New("internal: nil link in applySyncLinkPresentation")
	}
	dst.IconSize = ref.IconSize
	dst.Description = ref.Description
	dst.Relations = slices.Clone(ref.Relations)
	dst.Fields = pbtypes.CopyStruct(ref.Fields, true)
	if strings.TrimSpace(linkStyleReq) != "" {
		st, err := parseLinkStyle(linkStyleReq)
		if err != nil {
			return err
		}
		dst.Style = st
	} else {
		dst.Style = ref.Style
	}
	if strings.TrimSpace(cardStyleReq) != "" {
		cs, err := parseCardStyle(cardStyleReq)
		if err != nil {
			return err
		}
		dst.CardStyle = cs
	} else {
		dst.CardStyle = ref.CardStyle
	}
	return nil
}

func buildLinkReplacementBlock(current *model.Block, targetObjectId, linkStyleReq, cardStyleReq string) (*model.Block, error) {
	switch current.Content.(type) {
	case *model.BlockContentOfText:
		st, err := parseLinkStyle(linkStyleReq)
		if err != nil {
			return nil, err
		}
		cs, err := parseCardStyle(cardStyleReq)
		if err != nil {
			return nil, err
		}
		nb := pbtypes.CopyBlock(current)
		nb.Id = ""
		nb.ChildrenIds = nil
		nb.Content = &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: targetObjectId,
				Style:         st,
				CardStyle:     cs,
			},
		}
		return nb, nil
	case *model.BlockContentOfLink:
		nb := pbtypes.CopyBlock(current)
		nb.Id = ""
		nb.ChildrenIds = nil
		link := nb.GetLink()
		if link == nil {
			return nil, fmt.Errorf("%w", ErrUnsupportedBlockForLink)
		}
		link.TargetBlockId = targetObjectId
		if linkStyleReq != "" {
			st, err := parseLinkStyle(linkStyleReq)
			if err != nil {
				return nil, err
			}
			link.Style = st
		}
		if cardStyleReq != "" {
			cs, err := parseCardStyle(cardStyleReq)
			if err != nil {
				return nil, err
			}
			link.CardStyle = cs
		} else if link.CardStyle == model.BlockContentLink_Text {
			link.CardStyle = model.BlockContentLink_Card
		}
		return nb, nil
	default:
		return nil, fmt.Errorf("%w", ErrUnsupportedBlockForLink)
	}
}

// SetBlockObjectLink turns a text block into a link block or updates a link block target, using BlockReplace (editor path).
func (s *Service) SetBlockObjectLink(
	ctx context.Context,
	spaceId, objectId, blockId string,
	req apimodel.SetBlockObjectLinkRequest,
) (*apimodel.SetBlockObjectLinkResponse, error) {
	targetObjectId := req.TargetObjectId
	if state.IsRequiredBlockId(blockId) {
		return nil, ErrRequiredBlock
	}

	srcView, err := objectShowOrErr(s, ctx, spaceId, objectId)
	if err != nil {
		return nil, err
	}
	cur := findBlockInView(srcView, blockId)
	if cur == nil {
		return nil, ErrBlockNotFound
	}

	_, err = objectShowOrErr(s, ctx, spaceId, targetObjectId)
	if err != nil {
		return nil, err
	}

	replacement, err := buildLinkReplacementBlock(cur, targetObjectId, req.LinkStyle, req.CardStyle)
	if err != nil {
		return nil, err
	}
	if sid := strings.TrimSpace(req.SyncLinkPresentationFromBlockId); sid != "" {
		ref := findBlockInView(srcView, sid)
		if ref == nil || ref.GetLink() == nil {
			return nil, util.ErrBadInput("sync_link_presentation_from_block_id must be a link block id on the same page")
		}
		if err := applySyncLinkPresentation(replacement.GetLink(), ref.GetLink(), req.LinkStyle, req.CardStyle); err != nil {
			return nil, err
		}
		applySyncBlockChrome(replacement, ref)
	}
	if req.BackgroundColor != nil {
		raw := strings.TrimSpace(*req.BackgroundColor)
		if raw == "" {
			replacement.BackgroundColor = ""
		} else {
			v, err := parseBlockHighlightColor(raw)
			if err != nil {
				return nil, err
			}
			replacement.BackgroundColor = v
		}
	}
	if s := strings.TrimSpace(req.IconSize); s != "" {
		v, err := parseIconSize(s)
		if err != nil {
			return nil, err
		}
		replacement.GetLink().IconSize = v
	}
	if s := strings.TrimSpace(req.LinkDescription); s != "" {
		v, err := parseLinkDescription(s)
		if err != nil {
			return nil, err
		}
		replacement.GetLink().Description = v
	}
	if req.Relations != nil {
		replacement.GetLink().Relations = slices.Clone(req.Relations)
	}
	if cur.GetLink() != nil {
		if blockLinkReplaceEqual(cur, replacement) {
			return &apimodel.SetBlockObjectLinkResponse{
				Object:   "block_object_link",
				BlockId:  blockId,
				ObjectId: objectId,
				SpaceId:  spaceId,
				TargetId: targetObjectId,
				Replaced: false,
			}, nil
		}
	}

	rep := s.mw.BlockReplace(ctx, &pb.RpcBlockReplaceRequest{
		ContextId: objectId,
		BlockId:   blockId,
		Block:     replacement,
	})
	if rep.Error == nil || rep.Error.Code == pb.RpcBlockReplaceResponseError_NULL {
		outId := rep.BlockId
		if outId == "" {
			outId = blockId
		}
		return &apimodel.SetBlockObjectLinkResponse{
			Object:   "block_object_link",
			BlockId:  outId,
			ObjectId: objectId,
			SpaceId:  spaceId,
			TargetId: targetObjectId,
			Replaced: true,
		}, nil
	}
	if rep.Error != nil && rep.Error.Code == pb.RpcBlockReplaceResponseError_BAD_INPUT {
		return nil, fmt.Errorf("%w: %s", ErrBlockReplaceFailed, rep.Error.Description)
	}
	return nil, ErrBlockReplaceFailed
}

// DeleteBlockObjectLink removes a link block (optional target_object_id must match when set).
func (s *Service) DeleteBlockObjectLink(ctx context.Context, spaceId, objectId, blockId, optionalTargetId string) error {
	if state.IsRequiredBlockId(blockId) {
		return ErrRequiredBlock
	}

	srcView, err := objectShowOrErr(s, ctx, spaceId, objectId)
	if err != nil {
		return err
	}
	cur := findBlockInView(srcView, blockId)
	if cur == nil {
		return ErrBlockNotFound
	}
	link := cur.GetLink()
	if link == nil {
		return ErrNotLinkBlock
	}
	if optionalTargetId != "" && link.TargetBlockId != optionalTargetId {
		return ErrTargetMismatch
	}

	del := s.mw.BlockListDelete(ctx, &pb.RpcBlockListDeleteRequest{
		ContextId: objectId,
		BlockIds:  []string{blockId},
	})
	if del.Error == nil || del.Error.Code == pb.RpcBlockListDeleteResponseError_NULL {
		return nil
	}
	if del.Error != nil && del.Error.Code == pb.RpcBlockListDeleteResponseError_BAD_INPUT {
		return fmt.Errorf("%w: %s", ErrBlockDeleteFailed, del.Error.Description)
	}
	return ErrBlockDeleteFailed
}
