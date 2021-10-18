package block

import (
	"context"
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/doc"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/schema"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

var ErrOptionUsedByOtherObjects = fmt.Errorf("option is used by other objects")

func (s *service) MarkArchived(id string, archived bool) (err error) {
	return s.Do(id, func(b smartblock.SmartBlock) error {
		return b.SetDetails(nil, []*pb.RpcBlockSetDetailsDetail{
			{
				Key:   "isArchived",
				Value: pbtypes.Bool(archived),
			},
		}, true)
	})
}

func (s *service) SetBreadcrumbs(ctx *state.Context, req pb.RpcBlockSetBreadcrumbsRequest) (err error) {
	return s.Do(req.BreadcrumbsId, func(b smartblock.SmartBlock) error {
		if breadcrumbs, ok := b.(*editor.Breadcrumbs); ok {
			return breadcrumbs.SetCrumbs(req.Ids)
		} else {
			return ErrUnexpectedBlockType
		}
	})
}

func (s *service) CreateBlock(ctx *state.Context, req pb.RpcBlockCreateRequest) (id string, err error) {
	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		id, err = b.Create(ctx, "", req)
		return err
	})
	return
}

func (s *service) DuplicateBlocks(ctx *state.Context, req pb.RpcBlockListDuplicateRequest) (newIds []string, err error) {
	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		newIds, err = b.Duplicate(ctx, req)
		return err
	})
	return
}

func (s *service) UnlinkBlock(ctx *state.Context, req pb.RpcBlockUnlinkRequest) (err error) {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.Unlink(ctx, req.BlockIds...)
	})
}

func (s *service) SetDivStyle(ctx *state.Context, contextId string, style model.BlockContentDivStyle, ids ...string) (err error) {
	return s.DoBasic(contextId, func(b basic.Basic) error {
		return b.SetDivStyle(ctx, style, ids...)
	})
}

func (s *service) SplitBlock(ctx *state.Context, req pb.RpcBlockSplitRequest) (blockId string, err error) {
	err = s.DoText(req.ContextId, func(b stext.Text) error {
		blockId, err = b.Split(ctx, req)
		return err
	})
	return
}

func (s *service) MergeBlock(ctx *state.Context, req pb.RpcBlockMergeRequest) (err error) {
	return s.DoText(req.ContextId, func(b stext.Text) error {
		return b.Merge(ctx, req.FirstBlockId, req.SecondBlockId)
	})
}

func (s *service) MoveBlocks(ctx *state.Context, req pb.RpcBlockListMoveRequest) (err error) {
	if req.ContextId == req.TargetContextId {
		return s.DoBasic(req.ContextId, func(b basic.Basic) error {
			return b.Move(ctx, req)
		})
	}
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return s.DoBasic(req.TargetContextId, func(tb basic.Basic) error {
			blocks, err := b.InternalCut(ctx, req)
			if err != nil {
				return err
			}
			return tb.InternalPaste(blocks)
		})
	})
}

func (s *service) TurnInto(ctx *state.Context, contextId string, style model.BlockContentTextStyle, ids ...string) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.TurnInto(ctx, style, ids...)
	})
}

func (s *service) SimplePaste(contextId string, anySlot []*model.Block) (err error) {
	var blocks []simple.Block

	for _, b := range anySlot {
		blocks = append(blocks, simple.New(b))
	}

	return s.DoBasic(contextId, func(b basic.Basic) error {
		return b.InternalPaste(blocks)
	})
}

func (s *service) MoveBlocksToNewPage(ctx *state.Context, req pb.RpcBlockListMoveToNewPageRequest) (linkId string, err error) {
	// 1. Create new page, link
	linkId, pageId, err := s.CreatePage(ctx, "", pb.RpcBlockCreatePageRequest{
		ContextId: req.ContextId,
		TargetId:  req.DropTargetId,
		Position:  req.Position,
		Details:   req.Details,
	})

	if err != nil {
		return linkId, err
	}

	// 2. Move blocks to new page
	err = s.MoveBlocks(nil, pb.RpcBlockListMoveRequest{
		ContextId:       req.ContextId,
		BlockIds:        req.BlockIds,
		TargetContextId: pageId,
		DropTargetId:    "",
		Position:        0,
	})

	if err != nil {
		return linkId, err
	}

	return linkId, err
}

func (s *service) ConvertChildrenToPages(req pb.RpcBlockListConvertChildrenToPagesRequest) (linkIds []string, err error) {
	blocks := make(map[string]*model.Block)

	err = s.Do(req.ContextId, func(contextBlock smartblock.SmartBlock) error {
		for _, b := range contextBlock.Blocks() {
			blocks[b.Id] = b
		}
		return nil
	})

	if err != nil {
		return linkIds, err
	}

	for _, blockId := range req.BlockIds {
		if blocks[blockId] == nil || blocks[blockId].GetText() == nil {
			continue
		}

		fields := map[string]*types.Value{
			"name": pbtypes.String(blocks[blockId].GetText().Text),
		}

		if req.ObjectType != "" {
			fields[bundle.RelationKeyType.String()] = pbtypes.String(req.ObjectType)
		}

		children := s.AllDescendantIds(blockId, blocks)
		linkId, err := s.MoveBlocksToNewPage(nil, pb.RpcBlockListMoveToNewPageRequest{
			ContextId: req.ContextId,
			BlockIds:  children,
			Details: &types.Struct{
				Fields: fields,
			},
			DropTargetId: blockId,
			Position:     model.Block_Replace,
		})
		linkIds = append(linkIds, linkId)
		if err != nil {
			return linkIds, err
		}
	}

	return linkIds, err
}

func (s *service) UpdateBlockContent(ctx *state.Context, req pb.RpcBlockUpdateContentRequest) (err error) {
	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		var found bool
		err = b.Update(ctx, func(b simple.Block) error {
			found = true
			expectedType := fmt.Sprintf("%T", b.Model().GetContent())
			gotType := fmt.Sprintf("%T", req.GetBlock().Content)
			if gotType != expectedType {
				return fmt.Errorf("block content should have %s type, got %s instead", expectedType, gotType)
			}
			b.Model().Content = req.GetBlock().Content
			return nil
		}, req.BlockId)
		if err != nil {
			return err
		} else if !found {
			return smartblock.ErrSimpleBlockNotFound
		}

		return nil
	})
	return
}

func (s *service) ReplaceBlock(ctx *state.Context, req pb.RpcBlockReplaceRequest) (newId string, err error) {
	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		newId, err = b.Replace(ctx, req.BlockId, req.Block)
		return err
	})
	return
}

func (s *service) SetFields(ctx *state.Context, req pb.RpcBlockSetFieldsRequest) (err error) {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.SetFields(ctx, &pb.RpcBlockListSetFieldsRequestBlockField{
			BlockId: req.BlockId,
			Fields:  req.Fields,
		})
	})
}

func (s *service) SetDetails(ctx *state.Context, req pb.RpcBlockSetDetailsRequest) (err error) {
	return s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		return b.SetDetails(ctx, req.Details, true)
	})
}

func (s *service) SetFieldsList(ctx *state.Context, req pb.RpcBlockListSetFieldsRequest) (err error) {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.SetFields(ctx, req.BlockFields...)
	})
}

func (s *service) GetAggregatedRelations(req pb.RpcBlockDataviewRelationListAvailableRequest) (relations []*model.Relation, err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		relations, err = b.GetAggregatedRelations(req.BlockId)
		return err
	})

	return
}

func (s *service) UpdateDataviewView(ctx *state.Context, req pb.RpcBlockDataviewViewUpdateRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateView(ctx, req.BlockId, req.ViewId, *req.View, true)
	})
}

func (s *service) DeleteDataviewView(ctx *state.Context, req pb.RpcBlockDataviewViewDeleteRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.DeleteView(ctx, req.BlockId, req.ViewId, true)
	})
}

func (s *service) SetDataviewActiveView(ctx *state.Context, req pb.RpcBlockDataviewViewSetActiveRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.SetActiveView(ctx, req.BlockId, req.ViewId, int(req.Limit), int(req.Offset))
	})
}

func (s *service) SetDataviewViewPosition(ctx *state.Context, req pb.RpcBlockDataviewViewSetPositionRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.SetViewPosition(ctx, req.BlockId, req.ViewId, req.Position)
	})
}

func (s *service) CreateDataviewView(ctx *state.Context, req pb.RpcBlockDataviewViewCreateRequest) (id string, err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		if req.View == nil {
			req.View = &model.BlockContentDataviewView{}
		}
		view, err := b.CreateView(ctx, req.BlockId, *req.View)
		id = view.Id
		return err
	})

	return
}

func (s *service) CreateDataviewRecord(ctx *state.Context, req pb.RpcBlockDataviewRecordCreateRequest) (rec *types.Struct, err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		cr, err := b.CreateRecord(ctx, req.BlockId, model.ObjectDetails{Details: req.Record}, req.TemplateId)
		if err != nil {
			return err
		}
		rec = cr.Details
		return nil
	})

	return
}

func (s *service) UpdateDataviewRecord(ctx *state.Context, req pb.RpcBlockDataviewRecordUpdateRequest) (err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateRecord(ctx, req.BlockId, req.RecordId, model.ObjectDetails{Details: req.Record})
	})

	return
}

func (s *service) DeleteDataviewRecord(ctx *state.Context, req pb.RpcBlockDataviewRecordDeleteRequest) (err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.DeleteRecord(ctx, req.BlockId, req.RecordId)
	})

	return
}

func (s *service) UpdateDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationUpdateRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateRelation(ctx, req.BlockId, req.RelationKey, *req.Relation, true)
	})
}

func (s *service) AddDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationAddRequest) (relation *model.Relation, err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		var err error
		relation, err = b.AddRelation(ctx, req.BlockId, *req.Relation, true)
		if err != nil {
			return err
		}
		rels, err := b.GetDataviewRelations(req.BlockId)
		if err != nil {
			return err
		}

		relation = pbtypes.GetRelation(rels, relation.Key)
		if relation.Format == model.RelationFormat_status || relation.Format == model.RelationFormat_tag {
			err = b.FillAggregatedOptions(nil)
			if err != nil {
				log.Errorf("FillAggregatedOptions failed: %s", err.Error())
			}
		}
		return nil
	})

	return
}

func (s *service) DeleteDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationDeleteRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.DeleteRelation(ctx, req.BlockId, req.RelationKey, true)
	})
}

func (s *service) AddDataviewRecordRelationOption(ctx *state.Context, req pb.RpcBlockDataviewRecordRelationOptionAddRequest) (opt *model.RelationOption, err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		opt, err = b.AddRelationOption(ctx, req.BlockId, req.RecordId, req.RelationKey, *req.Option, true)
		if err != nil {
			return err
		}
		return nil
	})

	return
}

func (s *service) UpdateDataviewRecordRelationOption(ctx *state.Context, req pb.RpcBlockDataviewRecordRelationOptionUpdateRequest) error {
	err := s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		err := b.UpdateRelationOption(ctx, req.BlockId, req.RecordId, req.RelationKey, *req.Option, true)
		if err != nil {
			return err
		}
		return nil
	})

	return err
}

func (s *service) DeleteDataviewRecordRelationOption(ctx *state.Context, req pb.RpcBlockDataviewRecordRelationOptionDeleteRequest) error {
	err := s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		err := b.DeleteRelationOption(ctx, true, req.BlockId, req.RecordId, req.RelationKey, req.OptionId, true)
		if err != nil {
			return err
		}
		return nil
	})

	return err
}

func (s *service) SetDataviewSource(ctx *state.Context, contextId, blockId string, source []string) (err error) {
	return s.DoDataview(contextId, func(b dataview.Dataview) error {
		return b.SetSource(ctx, blockId, source)
	})
}

func (s *service) Copy(req pb.RpcBlockCopyRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		textSlot, htmlSlot, anySlot, err = cb.Copy(req)
		return err
	})

	return textSlot, htmlSlot, anySlot, err
}

func (s *service) Paste(ctx *state.Context, req pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.Paste(ctx, req, groupId)
		return err
	})

	return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
}

func (s *service) Cut(ctx *state.Context, req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		textSlot, htmlSlot, anySlot, err = cb.Cut(ctx, req)
		return err
	})
	return textSlot, htmlSlot, anySlot, err
}

func (s *service) Export(req pb.RpcBlockExportRequest) (path string, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		path, err = cb.Export(req)
		return err
	})
	return path, err
}

func (s *service) ImportMarkdown(ctx *state.Context, req pb.RpcBlockImportMarkdownRequest) (rootLinkIds []string, err error) {
	var rootLinks []*model.Block
	err = s.DoImport(req.ContextId, func(imp _import.Import) error {
		rootLinks, err = imp.ImportMarkdown(ctx, req)
		return err
	})
	if err != nil {
		return rootLinkIds, err
	}

	if len(rootLinks) == 1 {
		err = s.SimplePaste(req.ContextId, rootLinks)

		if err != nil {
			return rootLinkIds, err
		}
	} else {
		_, pageId, err := s.CreatePage(ctx, "", pb.RpcBlockCreatePageRequest{
			ContextId: req.ContextId,
			Details: &types.Struct{Fields: map[string]*types.Value{
				"name":      pbtypes.String("Import from Notion"),
				"iconEmoji": pbtypes.String("📁"),
			}},
		})

		if err != nil {
			return rootLinkIds, err
		}

		err = s.SimplePaste(pageId, rootLinks)
	}

	for _, r := range rootLinks {
		rootLinkIds = append(rootLinkIds, r.Id)
	}

	return rootLinkIds, err
}

func (s *service) SetTextText(ctx *state.Context, req pb.RpcBlockSetTextTextRequest) error {
	return s.DoText(req.ContextId, func(b stext.Text) error {
		return b.SetText(req)
	})
}

func (s *service) SetLatexText(ctx *state.Context, req pb.RpcBlockSetLatexTextRequest) error {
	return s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		return b.(basic.Basic).SetLatexText(ctx, req)
	})
}

func (s *service) SetTextStyle(ctx *state.Context, contextId string, style model.BlockContentTextStyle, blockIds ...string) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			t.SetStyle(style)
			return nil
		})
	})
}

func (s *service) SetTextChecked(ctx *state.Context, req pb.RpcBlockSetTextCheckedRequest) error {
	return s.DoText(req.ContextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(ctx, []string{req.BlockId}, true, func(t text.Block) error {
			t.SetChecked(req.Checked)
			return nil
		})
	})
}

func (s *service) SetTextColor(ctx *state.Context, contextId string, color string, blockIds ...string) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			t.SetTextColor(color)
			return nil
		})
	})
}

func (s *service) SetTextMark(ctx *state.Context, contextId string, mark *model.BlockContentTextMark, blockIds ...string) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.SetMark(ctx, mark, blockIds...)
	})
}

func (s *service) SetBackgroundColor(ctx *state.Context, contextId string, color string, blockIds ...string) (err error) {
	return s.DoBasic(contextId, func(b basic.Basic) error {
		return b.Update(ctx, func(b simple.Block) error {
			b.Model().BackgroundColor = color
			return nil
		}, blockIds...)
	})
}

func (s *service) SetAlign(ctx *state.Context, contextId string, align model.BlockAlign, blockIds ...string) (err error) {
	return s.DoBasic(contextId, func(b basic.Basic) error {
		return b.SetAlign(ctx, align, blockIds...)
	})
}

func (s *service) SetLayout(ctx *state.Context, contextId string, layout model.ObjectTypeLayout) (err error) {
	return s.DoBasic(contextId, func(b basic.Basic) error {
		return b.SetLayout(ctx, layout)
	})
}

func (s *service) FeaturedRelationAdd(ctx *state.Context, contextId string, relations ...string) error {
	return s.DoBasic(contextId, func(b basic.Basic) error {
		return b.FeaturedRelationAdd(ctx, relations...)
	})
}

func (s *service) FeaturedRelationRemove(ctx *state.Context, contextId string, relations ...string) error {
	return s.DoBasic(contextId, func(b basic.Basic) error {
		return b.FeaturedRelationRemove(ctx, relations...)
	})
}

func (s *service) UploadBlockFile(ctx *state.Context, req pb.RpcBlockUploadRequest, groupId string) (err error) {
	return s.DoFile(req.ContextId, func(b file.File) error {
		err = b.Upload(ctx, req.BlockId, file.FileSource{
			Path:    req.FilePath,
			Url:     req.Url,
			GroupId: groupId,
		}, false)
		return err
	})
}

func (s *service) UploadBlockFileSync(ctx *state.Context, req pb.RpcBlockUploadRequest) (err error) {
	return s.DoFile(req.ContextId, func(b file.File) error {
		err = b.Upload(ctx, req.BlockId, file.FileSource{
			Path: req.FilePath,
			Url:  req.Url,
		}, true)
		return err
	})
}

func (s *service) CreateAndUploadFile(ctx *state.Context, req pb.RpcBlockFileCreateAndUploadRequest) (id string, err error) {
	err = s.DoFile(req.ContextId, func(b file.File) error {
		id, err = b.CreateAndUpload(ctx, req)
		return err
	})
	return
}

func (s *service) UploadFile(req pb.RpcUploadFileRequest) (hash string, err error) {
	upl := file.NewUploader(s)
	if req.DisableEncryption {
		upl.AddOptions(files.WithPlaintext(true))
	}
	if req.Type != model.BlockContentFile_None {
		upl.SetType(req.Type)
	} else {
		upl.AutoType(true)
	}
	res := upl.SetFile(req.LocalPath).Upload(context.TODO())
	if res.Err != nil {
		return "", res.Err
	}
	return res.Hash, nil
}

func (s *service) DropFiles(req pb.RpcExternalDropFilesRequest) (err error) {
	return s.DoFileNonLock(req.ContextId, func(b file.File) error {
		return b.DropFiles(req)
	})
}

func (s *service) Undo(ctx *state.Context, req pb.RpcBlockUndoRequest) (counters pb.RpcBlockUndoRedoCounter, err error) {
	err = s.DoHistory(req.ContextId, func(b basic.IHistory) error {
		counters, err = b.Undo(ctx)
		return err
	})
	return
}

func (s *service) Redo(ctx *state.Context, req pb.RpcBlockRedoRequest) (counters pb.RpcBlockUndoRedoCounter, err error) {
	err = s.DoHistory(req.ContextId, func(b basic.IHistory) error {
		counters, err = b.Redo(ctx)
		return err
	})
	return
}

func (s *service) BookmarkFetch(ctx *state.Context, req pb.RpcBlockBookmarkFetchRequest) (err error) {
	return s.DoBookmark(req.ContextId, func(b bookmark.Bookmark) error {
		return b.Fetch(ctx, req.BlockId, req.Url, false)
	})
}

func (s *service) BookmarkFetchSync(ctx *state.Context, req pb.RpcBlockBookmarkFetchRequest) (err error) {
	return s.DoBookmark(req.ContextId, func(b bookmark.Bookmark) error {
		return b.Fetch(ctx, req.BlockId, req.Url, true)
	})
}

func (s *service) BookmarkCreateAndFetch(ctx *state.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (id string, err error) {
	err = s.DoBookmark(req.ContextId, func(b bookmark.Bookmark) error {
		id, err = b.CreateAndFetch(ctx, req)
		return err
	})
	return
}

func (s *service) SetRelationKey(ctx *state.Context, req pb.RpcBlockRelationSetKeyRequest) error {
	return s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		rels := b.Relations()
		rel := pbtypes.GetRelation(rels, req.Key)
		if rel == nil {
			var err error
			rels, err = s.Anytype().ObjectStore().ListRelations("")
			if err != nil {
				return err
			}
			rel = pbtypes.GetRelation(rels, req.Key)
			if rel == nil {
				return fmt.Errorf("relation with provided key not found")
			}
		}

		return b.(basic.Basic).AddRelationAndSet(ctx, pb.RpcBlockRelationAddRequest{Relation: rel, BlockId: req.BlockId, ContextId: req.ContextId})
	})
}

func (s *service) AddRelationBlock(ctx *state.Context, req pb.RpcBlockRelationAddRequest) error {
	return s.DoBasic(req.ContextId, func(b basic.Basic) error {
		return b.AddRelationAndSet(ctx, req)
	})
}

func (s *service) GetDocInfo(ctx context.Context, id string) (info doc.DocInfo, err error) {
	if err = s.Do(id, func(b smartblock.SmartBlock) error {
		info, err = b.GetDocInfo()
		return err
	}); err != nil {
		return
	}
	return
}

func (s *service) Wakeup(id string) (err error) {
	return s.Do(id, func(b smartblock.SmartBlock) error {
		return nil
	})
}

func (s *service) GetRelations(objectId string) (relations []*model.Relation, err error) {
	err = s.Do(objectId, func(b smartblock.SmartBlock) error {
		relations = b.Relations()
		return nil
	})
	return
}

// ModifyExtraRelations gets and updates extra relations under the sb lock to make sure no modifications are done in the middle
func (s *service) ModifyExtraRelations(ctx *state.Context, objectId string, modifier func(current []*model.Relation) ([]*model.Relation, error)) (err error) {
	if modifier == nil {
		return fmt.Errorf("modifier is nil")
	}
	return s.Do(objectId, func(b smartblock.SmartBlock) error {
		st := b.NewStateCtx(ctx)
		rels, err := modifier(st.ExtraRelations())
		if err != nil {
			return err
		}

		return b.UpdateExtraRelations(st.Context(), rels, true)
	})
}

// ModifyDetails performs details get and update under the sb lock to make sure no modifications are done in the middle
func (s *service) ModifyDetails(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error) {
	if modifier == nil {
		return fmt.Errorf("modifier is nil")
	}
	return s.Do(objectId, func(b smartblock.SmartBlock) error {
		dets, err := modifier(b.CombinedDetails())
		if err != nil {
			return err
		}

		return b.Apply(b.NewState().SetDetails(dets))
	})
}

func (s *service) UpdateExtraRelations(ctx *state.Context, objectId string, relations []*model.Relation, createIfMissing bool) (err error) {
	return s.Do(objectId, func(b smartblock.SmartBlock) error {
		return b.UpdateExtraRelations(ctx, relations, createIfMissing)
	})
}

func (s *service) AddExtraRelations(ctx *state.Context, objectId string, relations []*model.Relation) (relationsWithKeys []*model.Relation, err error) {
	err = s.Do(objectId, func(b smartblock.SmartBlock) error {
		var err2 error
		relationsWithKeys, err2 = b.AddExtraRelations(ctx, relations)
		if err2 != nil {
			return err2
		}
		return nil
	})

	return
}

func (s *service) AddExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionAddRequest) (opt *model.RelationOption, err error) {
	err = s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		opt, err = b.AddExtraRelationOption(ctx, req.RelationKey, *req.Option, true)
		if err != nil {
			return err
		}
		return nil
	})

	return
}

func (s *service) UpdateExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionUpdateRequest) error {
	return s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		err := b.UpdateExtraRelationOption(ctx, req.RelationKey, *req.Option, true)
		if err != nil {
			return err
		}
		return nil
	})
}

func (s *service) DeleteExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionDeleteRequest) error {
	objIds, err := s.anytype.ObjectStore().AggregateObjectIdsForOptionAndRelation(req.RelationKey, req.OptionId)
	if err != nil {
		return err
	}

	if !req.ConfirmRemoveAllValuesInRecords {
		for _, objId := range objIds {
			if objId != req.ContextId {
				return ErrOptionUsedByOtherObjects
			}
		}
	} else {
		for _, objId := range objIds {
			err = s.Do(objId, func(b smartblock.SmartBlock) error {
				err := b.DeleteExtraRelationOption(ctx, req.RelationKey, req.OptionId, true)
				if err != nil {
					return err
				}
				return nil
			})
			if err != nil && err != smartblock.ErrRelationOptionNotFound {
				return err
			}
		}
	}
	return nil
}

func (s *service) SetObjectTypes(ctx *state.Context, objectId string, objectTypes []string) (err error) {
	return s.Do(objectId, func(b smartblock.SmartBlock) error {
		return b.SetObjectTypes(ctx, objectTypes)
	})
}

func (s *service) CreateSet(ctx *state.Context, req pb.RpcBlockCreateSetRequest) (linkId string, setId string, err error) {
	var dvContent model.BlockContentOfDataview
	var dvSchema schema.Schema
	if len(req.Source) != 0 {
		if dvContent, dvSchema, err = dataview.DataviewBlockBySource(s.anytype.ObjectStore(), req.Source); err != nil {
			return
		}
	}

	csm, err := s.anytype.CreateBlock(coresb.SmartBlockTypeSet)
	if err != nil {
		err = fmt.Errorf("anytype.CreateBlock error: %v", err)
		return
	}
	setId = csm.ID()

	state := state.NewDoc(csm.ID(), nil).NewState()
	workspaceId, err := s.anytype.ObjectStore().GetCurrentWorkspaceId()
	if err == nil {
		state.SetDetailAndBundledRelation(bundle.RelationKeyWorkspaceId, pbtypes.String(workspaceId))
	}

	sb, err := s.newSmartBlock(setId, &smartblock.InitContext{
		State: state,
	})
	if err != nil {
		return "", "", err
	}
	set, ok := sb.(*editor.Set)
	if !ok {
		return "", setId, fmt.Errorf("unexpected set block type: %T", sb)
	}

	name := pbtypes.GetString(req.Details, bundle.RelationKeyName.String())
	icon := pbtypes.GetString(req.Details, bundle.RelationKeyIconEmoji.String())

	if name == "" && dvSchema != nil {
		name = dvSchema.Description() + " set"
	}
	if dvSchema != nil {
		err = set.InitDataview(&dvContent, name, icon)
	} else {
		err = set.InitDataview(nil, name, icon)
	}
	if err != nil {
		return "", setId, err
	}

	if req.ContextId == "" && req.TargetId == "" {
		// do not create a link
		return "", setId, nil
	}

	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		linkId, err = b.Create(ctx, "", pb.RpcBlockCreateRequest{
			TargetId: req.TargetId,
			Block: &model.Block{
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: setId,
						Style:         model.BlockContentLink_Dataview,
					},
				},
			},
			Position: req.Position,
		})
		if err != nil {
			err = fmt.Errorf("link create error: %v", err)
		}
		return err
	})

	return linkId, setId, nil
}

func (s *service) ObjectToSet(id string, source []string) (newId string, err error) {
	var details *types.Struct
	if err = s.Do(id, func(b smartblock.SmartBlock) error {
		details = pbtypes.CopyStruct(b.Details())
		return nil
	}); err != nil {
		return
	}

	_, newId, err = s.CreateSet(nil, pb.RpcBlockCreateSetRequest{
		Source:  source,
		Details: details,
	})
	if err != nil {
		return
	}

	oStore := s.app.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	res, err := oStore.GetWithLinksInfoByID(id)
	if err != nil {
		return
	}
	for _, il := range res.Links.Inbound {
		if err = s.replaceLink(il.Id, id, newId); err != nil {
			return
		}
	}
	err = s.DeleteObject(id)
	if err != nil {
		// intentionally do not return error here
		log.Errorf("failed to delete object after conversion to set: %s", err.Error())
	}

	return
}

func (s *service) RemoveExtraRelations(ctx *state.Context, objectTypeId string, relationKeys []string) (err error) {
	return s.Do(objectTypeId, func(b smartblock.SmartBlock) error {
		return b.RemoveExtraRelations(ctx, relationKeys)
	})
}

func (s *service) ListAvailableRelations(objectId string) (aggregatedRelations []*model.Relation, err error) {
	err = s.Do(objectId, func(b smartblock.SmartBlock) error {
		objType := b.ObjectType()
		aggregatedRelations = b.Relations()

		agRels, err := s.Anytype().ObjectStore().ListRelations(objType)
		if err != nil {
			return err
		}

		for _, rel := range agRels {
			if pbtypes.HasRelation(aggregatedRelations, rel.Key) {
				continue
			}
			aggregatedRelations = append(aggregatedRelations, pbtypes.CopyRelation(rel))
		}
		return nil
	})

	return
}
