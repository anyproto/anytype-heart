package block

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor/clipboard"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/link"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var ErrOptionUsedByOtherObjects = fmt.Errorf("option is used by other objects")

type FileUploadRequest struct {
	pb.RpcFileUploadRequest
	ObjectOrigin         objectorigin.ObjectOrigin
	CustomEncryptionKeys map[string]string
}

type UploadRequest struct {
	pb.RpcBlockUploadRequest
	ObjectOrigin objectorigin.ObjectOrigin
	ImageKind    model.ImageKind
}

type BookmarkFetchRequest struct {
	pb.RpcBlockBookmarkFetchRequest
	ObjectOrigin objectorigin.ObjectOrigin
}

func (s *Service) CreateBlock(ctx session.Context, req pb.RpcBlockCreateRequest) (id string, err error) {
	err = cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, b basic.Creatable) error {
		id, err = b.CreateBlock(st, req)
		return err
	})
	return
}

func (s *Service) DuplicateBlocks(
	sctx session.Context,
	req pb.RpcBlockListDuplicateRequest,
) (newIds []string, err error) {
	if req.ContextId == req.TargetContextId || req.TargetContextId == "" {
		err = cache.DoStateCtx(s, sctx, req.ContextId, func(st *state.State, sb basic.Duplicatable) error {
			newIds, err = sb.Duplicate(st, st, req.TargetId, req.Position, req.BlockIds)
			return err
		})
		return
	}

	err = cache.DoStateCtx(s, sctx, req.ContextId, func(srcState *state.State, sb basic.Duplicatable) error {
		return cache.DoState(s, req.TargetContextId, func(targetState *state.State, tb basic.Creatable) error {
			newIds, err = sb.Duplicate(srcState, targetState, req.TargetId, req.Position, req.BlockIds)
			return err
		})
	})

	return
}

func (s *Service) UnlinkBlock(ctx session.Context, req pb.RpcBlockListDeleteRequest) (err error) {
	return cache.Do(s, req.ContextId, func(b basic.Unlinkable) error {
		return b.Unlink(ctx, req.BlockIds...)
	})
}

func (s *Service) SetDivStyle(
	ctx session.Context, contextId string, style model.BlockContentDivStyle, ids ...string,
) (err error) {
	return cache.Do(s, contextId, func(b basic.CommonOperations) error {
		return b.SetDivStyle(ctx, style, ids...)
	})
}

func (s *Service) SplitBlock(ctx session.Context, req pb.RpcBlockSplitRequest) (blockId string, err error) {
	err = cache.Do(s, req.ContextId, func(b stext.Text) error {
		blockId, err = b.Split(ctx, req)
		return err
	})
	return
}

func (s *Service) MergeBlock(ctx session.Context, req pb.RpcBlockMergeRequest) (err error) {
	return cache.Do(s, req.ContextId, func(b stext.Text) error {
		return b.Merge(ctx, req.FirstBlockId, req.SecondBlockId)
	})
}

func (s *Service) TurnInto(
	ctx session.Context, contextId string, style model.BlockContentTextStyle, ids ...string,
) error {
	return cache.Do(s, contextId, func(b stext.Text) error {
		return b.TurnInto(ctx, style, ids...)
	})
}

func (s *Service) ReplaceBlock(ctx session.Context, req pb.RpcBlockReplaceRequest) (newId string, err error) {
	err = cache.Do(s, req.ContextId, func(b basic.Replaceable) error {
		newId, err = b.Replace(ctx, req.BlockId, req.Block)
		return err
	})
	return
}

func (s *Service) SetFields(ctx session.Context, req pb.RpcBlockSetFieldsRequest) (err error) {
	return cache.Do(s, req.ContextId, func(b basic.CommonOperations) error {
		return b.SetFields(ctx, &pb.RpcBlockListSetFieldsRequestBlockField{
			BlockId: req.BlockId,
			Fields:  req.Fields,
		})
	})
}

func (s *Service) SetFieldsList(ctx session.Context, req pb.RpcBlockListSetFieldsRequest) (err error) {
	return cache.Do(s, req.ContextId, func(b basic.CommonOperations) error {
		return b.SetFields(ctx, req.BlockFields...)
	})
}

func (s *Service) UpdateDataviewView(ctx session.Context, req pb.RpcBlockDataviewViewUpdateRequest) error {
	return cache.Do(s, req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateView(ctx, req.BlockId, req.ViewId, req.View, true)
	})
}

func (s *Service) UpdateDataviewGroupOrder(ctx session.Context, req pb.RpcBlockDataviewGroupOrderUpdateRequest) error {
	return cache.Do(s, req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateViewGroupOrder(ctx, req.BlockId, req.GroupOrder)
	})
}

func (s *Service) UpdateDataviewObjectOrder(
	ctx session.Context, req pb.RpcBlockDataviewObjectOrderUpdateRequest,
) error {
	return cache.Do(s, req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateViewObjectOrder(ctx, req.BlockId, req.ObjectOrders)
	})
}

func (s *Service) DataviewMoveObjectsInView(
	ctx session.Context, req *pb.RpcBlockDataviewObjectOrderMoveRequest,
) error {
	return cache.Do(s, req.ContextId, func(b dataview.Dataview) error {
		return b.DataviewMoveObjectsInView(ctx, req)
	})
}

func (s *Service) DeleteDataviewView(ctx session.Context, req pb.RpcBlockDataviewViewDeleteRequest) error {
	return cache.Do(s, req.ContextId, func(b dataview.Dataview) error {
		return b.DeleteView(ctx, req.BlockId, req.ViewId, true)
	})
}

func (s *Service) SetDataviewActiveView(ctx session.Context, req pb.RpcBlockDataviewViewSetActiveRequest) error {
	return cache.Do(s, req.ContextId, func(b dataview.Dataview) error {
		return b.SetActiveView(ctx, req.BlockId, req.ViewId)
	})
}

func (s *Service) SetDataviewViewPosition(ctx session.Context, req pb.RpcBlockDataviewViewSetPositionRequest) error {
	return cache.Do(s, req.ContextId, func(b dataview.Dataview) error {
		return b.SetViewPosition(ctx, req.BlockId, req.ViewId, req.Position)
	})
}

func (s *Service) CreateDataviewView(
	ctx session.Context, req pb.RpcBlockDataviewViewCreateRequest,
) (id string, err error) {
	err = cache.Do(s, req.ContextId, func(b dataview.Dataview) error {
		if req.View == nil {
			req.View = &model.BlockContentDataviewView{CardSize: model.BlockContentDataviewView_Medium}
		}
		view, e := b.CreateView(ctx, req.BlockId, *req.View, req.Source)
		if e != nil {
			return e
		}
		id = view.Id
		return nil
	})
	return
}

func (s *Service) AddDataviewRelation(ctx session.Context, req pb.RpcBlockDataviewRelationAddRequest) (err error) {
	err = cache.Do(s, req.ContextId, func(b dataview.Dataview) error {
		return b.AddRelations(ctx, req.BlockId, req.RelationKeys, true)
	})

	return
}

func (s *Service) DeleteDataviewRelation(ctx session.Context, req pb.RpcBlockDataviewRelationDeleteRequest) error {
	return cache.Do(s, req.ContextId, func(b dataview.Dataview) error {
		return b.DeleteRelations(ctx, req.BlockId, req.RelationKeys, true)
	})
}

func (s *Service) SetDataviewSource(ctx session.Context, contextId, blockId string, source []string) (err error) {
	return cache.Do(s, contextId, func(b dataview.Dataview) error {
		return b.SetSource(ctx, blockId, source)
	})
}

func (s *Service) Copy(
	ctx session.Context,
	req pb.RpcBlockCopyRequest,
) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	err = cache.Do(s, req.ContextId, func(cb clipboard.Clipboard) error {
		textSlot, htmlSlot, anySlot, err = cb.Copy(ctx, req)
		return err
	})

	return textSlot, htmlSlot, anySlot, err
}

func (s *Service) Paste(
	ctx session.Context, req pb.RpcBlockPasteRequest, groupId string,
) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	err = cache.Do(s, req.ContextId, func(cb clipboard.Clipboard) error {
		blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.Paste(ctx, &req, groupId)
		return err
	})

	return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
}

func (s *Service) Cut(
	ctx session.Context, req pb.RpcBlockCutRequest,
) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	err = cache.Do(s, req.ContextId, func(cb clipboard.Clipboard) error {
		textSlot, htmlSlot, anySlot, err = cb.Cut(ctx, req)
		return err
	})
	return textSlot, htmlSlot, anySlot, err
}

func (s *Service) Export(req pb.RpcBlockExportRequest) (path string, err error) {
	err = cache.Do(s, req.ContextId, func(cb clipboard.Clipboard) error {
		path, err = cb.Export(req)
		return err
	})
	return path, err
}

func (s *Service) SetTextText(ctx session.Context, req pb.RpcBlockTextSetTextRequest) error {
	return cache.Do(s, req.ContextId, func(b stext.Text) error {
		return b.SetText(ctx, req)
	})
}

func (s *Service) SetLatexText(ctx session.Context, req pb.RpcBlockLatexSetTextRequest) error {
	return cache.Do(s, req.ContextId, func(b basic.CommonOperations) error {
		return b.SetLatexText(ctx, req)
	})
}

func (s *Service) SetLatexProcessor(ctx session.Context, req pb.RpcBlockLatexSetTextRequest) error {
	return cache.Do(s, req.ContextId, func(b basic.CommonOperations) error {
		return b.SetLatexText(ctx, req)
	})
}

func (s *Service) SetTextStyle(
	ctx session.Context, contextId string, style model.BlockContentTextStyle, blockIds ...string,
) error {
	return cache.Do(s, contextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			t.SetStyle(style)
			return nil
		})
	})
}

func (s *Service) SetTextChecked(ctx session.Context, req pb.RpcBlockTextSetCheckedRequest) error {
	return cache.Do(s, req.ContextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(ctx, []string{req.BlockId}, true, func(t text.Block) error {
			t.SetChecked(req.Checked)
			return nil
		})
	})
}

func (s *Service) SetTextColor(ctx session.Context, contextId string, color string, blockIds ...string) error {
	return cache.Do(s, contextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			t.SetTextColor(color)
			return nil
		})
	})
}

func (s *Service) ClearTextStyle(ctx session.Context, contextId string, blockIds ...string) error {
	return cache.Do(s, contextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			t.Model().BackgroundColor = ""
			t.Model().Align = model.Block_AlignLeft
			t.Model().VerticalAlign = model.Block_VerticalAlignTop
			t.SetTextColor("")
			t.SetStyle(model.BlockContentText_Paragraph)

			marks := t.Model().GetText().Marks.Marks[:0]
			for _, m := range t.Model().GetText().Marks.Marks {
				switch m.Type {
				case model.BlockContentTextMark_Strikethrough,
					model.BlockContentTextMark_Keyboard,
					model.BlockContentTextMark_Italic,
					model.BlockContentTextMark_Bold,
					model.BlockContentTextMark_Underscored,
					model.BlockContentTextMark_TextColor,
					model.BlockContentTextMark_BackgroundColor:
				default:
					marks = append(marks, m)
				}
			}
			t.Model().GetText().Marks.Marks = marks

			return nil
		})
	})
}

func (s *Service) ClearTextContent(ctx session.Context, contextId string, blockIds ...string) error {
	return cache.Do(s, contextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			t.SetText("", nil)
			return nil
		})
	})
}

func (s *Service) SetTextMark(
	ctx session.Context, contextId string, mark *model.BlockContentTextMark, blockIds ...string,
) error {
	return cache.Do(s, contextId, func(b stext.Text) error {
		return b.SetMark(ctx, mark, blockIds...)
	})
}

func (s *Service) SetTextIcon(ctx session.Context, contextId, image, emoji string, blockIds ...string) error {
	return cache.Do(s, contextId, func(b stext.Text) error {
		return b.SetIcon(ctx, image, emoji, blockIds...)
	})
}

func (s *Service) SetBackgroundColor(
	ctx session.Context, contextId string, color string, blockIds ...string,
) (err error) {
	return cache.Do(s, contextId, func(b basic.Updatable) error {
		return b.Update(ctx, func(b simple.Block) error {
			b.Model().BackgroundColor = color
			return nil
		}, blockIds...)
	})
}

func (s *Service) SetLinkAppearance(ctx session.Context, req pb.RpcBlockLinkListSetAppearanceRequest) (err error) {
	return cache.Do(s, req.ContextId, func(b basic.Updatable) error {
		return b.Update(ctx, func(b simple.Block) error {
			if linkBlock, ok := b.(link.Block); ok {
				return linkBlock.SetAppearance(&model.BlockContentLink{
					IconSize:    req.IconSize,
					CardStyle:   req.CardStyle,
					Description: req.Description,
					Relations:   req.Relations,
				})
			}
			return nil
		}, req.BlockIds...)
	})
}

func (s *Service) SetAlign(
	ctx session.Context, contextId string, align model.BlockAlign, blockIds ...string,
) (err error) {
	return cache.DoStateCtx(s, ctx, contextId, func(st *state.State, sb smartblock.SmartBlock) error {
		return st.SetAlign(align, blockIds...)
	})
}

func (s *Service) SetVerticalAlign(
	ctx session.Context, contextId string, align model.BlockVerticalAlign, blockIds ...string,
) (err error) {
	return cache.Do(s, contextId, func(sb smartblock.SmartBlock) error {
		return sb.SetVerticalAlign(ctx, align, blockIds...)
	})
}

func (s *Service) SetLayout(ctx session.Context, contextId string, layout model.ObjectTypeLayout) (err error) {
	return cache.Do(s, contextId, func(sb basic.CommonOperations) error {
		return sb.SetLayout(ctx, layout)
	})
}

func (s *Service) FeaturedRelationAdd(ctx session.Context, contextId string, relations ...string) error {
	return cache.Do(s, contextId, func(b basic.CommonOperations) error {
		return b.FeaturedRelationAdd(ctx, relations...)
	})
}

func (s *Service) FeaturedRelationRemove(ctx session.Context, contextId string, relations ...string) error {
	return cache.Do(s, contextId, func(b basic.CommonOperations) error {
		return b.FeaturedRelationRemove(ctx, relations...)
	})
}

func (s *Service) UploadBlockFile(
	ctx session.Context, req UploadRequest, groupID string, isSync bool,
) (fileObjectId string, err error) {
	err = cache.Do(s, req.ContextId, func(b file.File) error {
		fileObjectId, err = b.Upload(ctx, req.BlockId, file.FileSource{
			Path:      req.FilePath,
			Url:       req.Url,
			Bytes:     req.Bytes,
			GroupID:   groupID,
			Origin:    req.ObjectOrigin,
			ImageKind: req.ImageKind,
		}, isSync)
		return err
	})
	return fileObjectId, err
}

func (s *Service) CreateAndUploadFile(
	ctx session.Context, req pb.RpcBlockFileCreateAndUploadRequest,
) (id string, err error) {
	err = cache.Do(s, req.ContextId, func(b file.File) error {
		id, err = b.CreateAndUpload(ctx, req)
		return err
	})
	return
}

func (s *Service) UploadFile(ctx context.Context, spaceId string, req FileUploadRequest) (objectId string, details *types.Struct, err error) {
	upl := s.fileUploaderService.NewUploader(spaceId, req.ObjectOrigin)
	if req.DisableEncryption {
		log.Errorf("DisableEncryption is deprecated and has no effect")
	}

	if req.CustomEncryptionKeys != nil {
		upl.SetCustomEncryptionKeys(req.CustomEncryptionKeys)
	}
	upl.SetStyle(req.Style)
	upl.SetAdditionalDetails(req.Details)
	if req.Type != model.BlockContentFile_None {
		upl.SetType(req.Type)
	}
	if req.LocalPath != "" {
		upl.SetFile(req.LocalPath)
	} else if req.Url != "" {
		upl.SetUrl(req.Url)
	}
	if req.ImageKind != model.ImageKind_Basic {
		upl.SetImageKind(req.ImageKind)
	}
	res := upl.Upload(ctx)
	if res.Err != nil {
		return "", nil, res.Err
	}
	return res.FileObjectId, res.FileObjectDetails, nil
}

func (s *Service) DropFiles(req pb.RpcFileDropRequest) (err error) {
	return s.DoFileNonLock(req.ContextId, func(b file.File) error {
		return b.DropFiles(req)
	})
}

func (s *Service) SetFileTargetObjectId(ctx session.Context, contextId string, blockId, targetObjectId string) error {
	return cache.Do(s, contextId, func(b file.File) error {
		return b.SetFileTargetObjectId(ctx, blockId, targetObjectId)
	})
}

func (s *Service) SetFileStyle(
	ctx session.Context, contextId string, style model.BlockContentFileStyle, blockIds ...string,
) error {
	return cache.Do(s, contextId, func(b file.File) error {
		return b.SetFileStyle(ctx, style, blockIds...)
	})
}

func (s *Service) Undo(
	ctx session.Context, req pb.RpcObjectUndoRequest,
) (info basic.HistoryInfo, err error) {
	err = cache.Do(s, req.ContextId, func(b basic.IHistory) error {
		info, err = b.Undo(ctx)
		return err
	})
	return
}

func (s *Service) Redo(
	ctx session.Context, req pb.RpcObjectRedoRequest,
) (info basic.HistoryInfo, err error) {
	err = cache.Do(s, req.ContextId, func(b basic.IHistory) error {
		info, err = b.Redo(ctx)
		return err
	})
	return
}

func (s *Service) BookmarkFetch(ctx session.Context, req BookmarkFetchRequest) (err error) {
	return cache.Do(s, req.ContextId, func(b bookmark.Bookmark) error {
		return b.Fetch(ctx, req.BlockId, req.Url, req.ObjectOrigin)
	})
}

func (s *Service) BookmarkCreateAndFetch(ctx session.Context, req bookmark.CreateAndFetchRequest) (id string, err error) {
	err = cache.Do(s, req.ContextId, func(b bookmark.Bookmark) error {
		id, err = b.CreateAndFetch(ctx, req)
		return err
	})
	return
}

func (s *Service) SetRelationKey(ctx session.Context, req pb.RpcBlockRelationSetKeyRequest) error {
	return cache.Do(s, req.ContextId, func(b basic.CommonOperations) error {
		return b.AddRelationAndSet(ctx, pb.RpcBlockRelationAddRequest{
			RelationKey: req.Key, BlockId: req.BlockId, ContextId: req.ContextId,
		})
	})
}

func (s *Service) AddRelationBlock(ctx session.Context, req pb.RpcBlockRelationAddRequest) error {
	return cache.Do(s, req.ContextId, func(b basic.CommonOperations) error {
		return b.AddRelationAndSet(ctx, req)
	})
}

func (s *Service) GetRelations(ctx session.Context, objectId string) (relations []*model.Relation, err error) {
	err = cache.Do(s, objectId, func(b smartblock.SmartBlock) error {
		relations = b.Relations(nil).Models()
		return nil
	})
	return
}

func (s *Service) AddExtraRelations(ctx session.Context, objectId string, relationIds []string) (err error) {
	if len(relationIds) == 0 {
		return nil
	}
	return cache.Do(s, objectId, func(b smartblock.SmartBlock) error { // TODO RQ: check if empty
		return b.AddRelationLinks(ctx, relationIds...)
	})
}

func (s *Service) SetObjectTypes(ctx session.Context, objectId string, objectTypeUniqueKeys []string) (err error) {
	return cache.Do(s, objectId, func(b basic.CommonOperations) error {
		objectTypeKeys := make([]domain.TypeKey, 0, len(objectTypeUniqueKeys))
		for _, rawUniqueKey := range objectTypeUniqueKeys {
			objectTypeKey, err := domain.GetTypeKeyFromRawUniqueKey(rawUniqueKey)
			if err != nil {
				return fmt.Errorf("get type key from raw unique key: %w", err)
			}
			objectTypeKeys = append(objectTypeKeys, objectTypeKey)
		}
		return b.SetObjectTypes(ctx, objectTypeKeys, false)
	})
}

func (s *Service) RemoveExtraRelations(ctx session.Context, objectTypeId string, relationKeys []string) (err error) {
	return cache.Do(s, objectTypeId, func(b smartblock.SmartBlock) error {
		return b.RemoveExtraRelations(ctx, relationKeys)
	})
}

func (s *Service) ListAvailableRelations(ctx session.Context, objectId string) (aggregatedRelations []*model.Relation, err error) {
	err = cache.Do(s, objectId, func(b smartblock.SmartBlock) error {
		// TODO: not implemented
		return nil
	})
	return
}

func (s *Service) ListConvertToObjects(
	ctx session.Context, req pb.RpcBlockListConvertToObjectsRequest,
) (linkIds []string, err error) {
	err = cache.Do(s, req.ContextId, func(b basic.CommonOperations) error {
		linkIds, err = b.ExtractBlocksToObjects(ctx, s.objectCreator, s.templateService, req)
		return err
	})
	return
}

func (s *Service) MoveBlocksToNewPage(
	ctx context.Context,
	sctx session.Context,
	req pb.RpcBlockListMoveToNewObjectRequest,
) (linkID string, err error) {
	// 1. Create new page, link
	linkID, objectID, _, err := s.CreateLinkToTheNewObject(ctx, sctx, &pb.RpcBlockLinkCreateWithObjectRequest{
		ContextId:           req.ContextId,
		TargetId:            req.DropTargetId,
		ObjectTypeUniqueKey: bundle.TypeKeyPage.URL(),
		Position:            req.Position,
		Details:             req.Details,
	})
	if err != nil {
		return
	}

	// 2. Move blocks to new page
	// TODO Use DoState2
	err = cache.DoState(s, req.ContextId, func(srcState *state.State, sb basic.Movable) error {
		return cache.DoState(s, objectID, func(destState *state.State, tb basic.Movable) error {
			return sb.Move(srcState, destState, "", model.Block_Inner, req.BlockIds)
		})
	})
	if err != nil {
		return
	}
	return linkID, err
}

type Movable interface {
	basic.Movable
	basic.Restrictionable
}

func (s *Service) MoveBlocks(req pb.RpcBlockListMoveToExistingObjectRequest) error {
	return cache.DoState2(s, req.ContextId, req.TargetContextId, func(srcState, destState *state.State, sb, tb Movable) error {
		if err := sb.Restrictions().Object.Check(model.Restrictions_Blocks); err != nil {
			return restriction.ErrRestricted
		}
		if err := tb.Restrictions().Object.Check(model.Restrictions_Blocks); err != nil {
			return restriction.ErrRestricted
		}
		return sb.Move(srcState, destState, req.DropTargetId, req.Position, req.BlockIds)
	})
}

func (s *Service) CreateTableBlock(ctx session.Context, req pb.RpcBlockTableCreateRequest) (id string, err error) {
	err = cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		id, err = e.TableCreate(st, req)
		return err
	})
	return
}

func (s *Service) TableRowCreate(ctx session.Context, req pb.RpcBlockTableRowCreateRequest) error {
	return cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		_, err := e.RowCreate(st, req)
		return err
	})
}

func (s *Service) TableColumnCreate(ctx session.Context, req pb.RpcBlockTableColumnCreateRequest) error {
	return cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		_, err := e.ColumnCreate(st, req)
		return err
	})
}

func (s *Service) TableRowDelete(ctx session.Context, req pb.RpcBlockTableRowDeleteRequest) (err error) {
	err = cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.RowDelete(st, req)
	})
	return
}

func (s *Service) TableColumnDelete(ctx session.Context, req pb.RpcBlockTableColumnDeleteRequest) (err error) {
	err = cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.ColumnDelete(st, req)
	})
	return
}

func (s *Service) TableColumnMove(ctx session.Context, req pb.RpcBlockTableColumnMoveRequest) (err error) {
	err = cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.ColumnMove(st, req)
	})
	return
}

func (s *Service) TableRowDuplicate(ctx session.Context, req pb.RpcBlockTableRowDuplicateRequest) error {
	return cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		_, err := e.RowDuplicate(st, req)
		return err
	})
}

func (s *Service) TableColumnDuplicate(
	ctx session.Context, req pb.RpcBlockTableColumnDuplicateRequest,
) (id string, err error) {
	err = cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		id, err = e.ColumnDuplicate(st, req)
		return err
	})
	return id, err
}

func (s *Service) TableExpand(ctx session.Context, req pb.RpcBlockTableExpandRequest) (err error) {
	err = cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.Expand(st, req)
	})
	return err
}

func (s *Service) TableRowListFill(ctx session.Context, req pb.RpcBlockTableRowListFillRequest) (err error) {
	err = cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.RowListFill(st, req)
	})
	return err
}

func (s *Service) TableRowListClean(ctx session.Context, req pb.RpcBlockTableRowListCleanRequest) (err error) {
	err = cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.RowListClean(st, req)
	})
	return err
}

func (s *Service) TableRowSetHeader(ctx session.Context, req pb.RpcBlockTableRowSetHeaderRequest) (err error) {
	err = cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.RowSetHeader(st, req)
	})
	return err
}

func (s *Service) TableSort(ctx session.Context, req pb.RpcBlockTableSortRequest) (err error) {
	err = cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.Sort(st, req)
	})
	return err
}

func (s *Service) TableColumnListFill(ctx session.Context, req pb.RpcBlockTableColumnListFillRequest) (err error) {
	err = cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.ColumnListFill(st, req)
	})
	return err
}

func (s *Service) CreateWidgetBlock(ctx session.Context, req *pb.RpcBlockCreateWidgetRequest) (string, error) {
	var id string
	err := cache.DoStateCtx(s, ctx, req.ContextId, func(st *state.State, w widget.Widget) error {
		var err error
		id, err = w.CreateBlock(st, req)
		return err
	})
	return id, err
}

func (s *Service) CopyDataviewToBlock(
	ctx session.Context,
	req *pb.RpcBlockDataviewCreateFromExistingObjectRequest,
) ([]*model.BlockContentDataviewView, error) {

	var targetDvContent *model.BlockContentDataview

	err := cache.Do(s, req.TargetObjectId, func(d dataview.Dataview) error {
		var err error
		targetDvContent, err = d.GetDataview(template.DataviewBlockId)
		return err
	})
	if err != nil {
		return nil, err
	}

	err = cache.Do(s, req.ContextId, func(b smartblock.SmartBlock) error {
		st := b.NewStateCtx(ctx)
		block := st.Get(req.BlockId)
		if block == nil {
			return fmt.Errorf("block is not found")
		}

		dvContent, ok := block.Model().Content.(*model.BlockContentOfDataview)
		if !ok {
			return fmt.Errorf("block must contain dataView content")
		}

		dvContent.Dataview.Views = targetDvContent.Views
		dvContent.Dataview.RelationLinks = targetDvContent.RelationLinks
		dvContent.Dataview.GroupOrders = targetDvContent.GroupOrders
		dvContent.Dataview.ObjectOrders = targetDvContent.ObjectOrders
		dvContent.Dataview.TargetObjectId = req.TargetObjectId
		dvContent.Dataview.IsCollection = targetDvContent.IsCollection

		return b.Apply(st)
	})
	if err != nil {
		return nil, err
	}

	return targetDvContent.Views, err
}
