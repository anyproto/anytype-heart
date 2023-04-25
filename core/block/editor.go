package block

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"

	"github.com/anytypeio/any-sync/app/ocache"
	"github.com/anytypeio/go-anytype-middleware/core/block/doc"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/table"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/widget"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

var ErrOptionUsedByOtherObjects = fmt.Errorf("option is used by other objects")

func (s *Service) MarkArchived(id string, archived bool) (err error) {
	return s.Do(id, func(b smartblock.SmartBlock) error {
		return b.SetDetails(nil, []*pb.RpcObjectSetDetailsDetail{
			{
				Key:   "isArchived",
				Value: pbtypes.Bool(archived),
			},
		}, true)
	})
}

func (s *Service) SetBreadcrumbs(ctx *session.Context, req pb.RpcObjectSetBreadcrumbsRequest) (err error) {
	return s.Do(req.BreadcrumbsId, func(b smartblock.SmartBlock) error {
		if breadcrumbs, ok := b.(*editor.Breadcrumbs); ok {
			return breadcrumbs.SetCrumbs(req.Ids)
		} else {
			return ErrUnexpectedBlockType
		}
	})
}

func (s *Service) CreateBlock(ctx *session.Context, req pb.RpcBlockCreateRequest) (id string, err error) {
	err = DoState(s, req.ContextId, func(st *state.State, b basic.Creatable) error {
		id, err = b.CreateBlock(st, req)
		return err
	})
	return
}

func (s *Service) DuplicateBlocks(
	ctx *session.Context,
	req pb.RpcBlockListDuplicateRequest,
) (newIds []string, err error) {
	if req.ContextId == req.TargetContextId || req.TargetContextId == "" {
		err = DoStateCtx(s, ctx, req.ContextId, func(st *state.State, sb basic.Duplicatable) error {
			newIds, err = sb.Duplicate(st, st, req.TargetId, req.Position, req.BlockIds)
			return err
		})
		return
	}

	err = DoStateCtx(s, ctx, req.ContextId, func(srcState *state.State, sb basic.Duplicatable) error {
		return DoState(s, req.TargetContextId, func(targetState *state.State, tb basic.Creatable) error {
			newIds, err = sb.Duplicate(srcState, targetState, req.TargetId, req.Position, req.BlockIds)
			return err
		})
	})

	return
}

func (s *Service) UnlinkBlock(ctx *session.Context, req pb.RpcBlockListDeleteRequest) (err error) {
	return Do(s, req.ContextId, func(b basic.Unlinkable) error {
		return b.Unlink(ctx, req.BlockIds...)
	})
}

func (s *Service) SetDivStyle(
	ctx *session.Context, contextId string, style model.BlockContentDivStyle, ids ...string,
) (err error) {
	return Do(s, contextId, func(b basic.CommonOperations) error {
		return b.SetDivStyle(ctx, style, ids...)
	})
}

func (s *Service) SplitBlock(ctx *session.Context, req pb.RpcBlockSplitRequest) (blockId string, err error) {
	err = s.DoText(req.ContextId, func(b stext.Text) error {
		blockId, err = b.Split(ctx, req)
		return err
	})
	return
}

func (s *Service) MergeBlock(ctx *session.Context, req pb.RpcBlockMergeRequest) (err error) {
	return s.DoText(req.ContextId, func(b stext.Text) error {
		return b.Merge(ctx, req.FirstBlockId, req.SecondBlockId)
	})
}

func (s *Service) TurnInto(
	ctx *session.Context, contextId string, style model.BlockContentTextStyle, ids ...string,
) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.TurnInto(ctx, style, ids...)
	})
}

func (s *Service) SimplePaste(contextId string, anySlot []*model.Block) (err error) {
	var blocks []simple.Block

	for _, b := range anySlot {
		blocks = append(blocks, simple.New(b))
	}

	return DoState(s, contextId, func(s *state.State, b basic.CommonOperations) error {
		return b.PasteBlocks(s, "", model.Block_Inner, blocks)
	})
}

func (s *Service) ReplaceBlock(ctx *session.Context, req pb.RpcBlockReplaceRequest) (newId string, err error) {
	err = Do(s, req.ContextId, func(b basic.Replaceable) error {
		newId, err = b.Replace(ctx, req.BlockId, req.Block)
		return err
	})
	return
}

func (s *Service) SetFields(ctx *session.Context, req pb.RpcBlockSetFieldsRequest) (err error) {
	return Do(s, req.ContextId, func(b basic.CommonOperations) error {
		return b.SetFields(ctx, &pb.RpcBlockListSetFieldsRequestBlockField{
			BlockId: req.BlockId,
			Fields:  req.Fields,
		})
	})
}

func (s *Service) SetDetails(ctx *session.Context, req pb.RpcObjectSetDetailsRequest) (err error) {
	return s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		return b.SetDetails(ctx, req.Details, true)
	})
}

func (s *Service) SetFieldsList(ctx *session.Context, req pb.RpcBlockListSetFieldsRequest) (err error) {
	return Do(s, req.ContextId, func(b basic.CommonOperations) error {
		return b.SetFields(ctx, req.BlockFields...)
	})
}

func (s *Service) GetAggregatedRelations(
	req pb.RpcBlockDataviewRelationListAvailableRequest,
) (relations []*model.Relation, err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		// todo: remove or replace
		// relations, err = b.GetAggregatedRelations(req.BlockId)
		return err
	})

	return
}

func (s *Service) UpdateDataviewView(ctx *session.Context, req pb.RpcBlockDataviewViewUpdateRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateView(ctx, req.BlockId, req.ViewId, req.View, true)
	})
}

func (s *Service) UpdateDataviewGroupOrder(ctx *session.Context, req pb.RpcBlockDataviewGroupOrderUpdateRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateViewGroupOrder(ctx, req.BlockId, req.GroupOrder)
	})
}

func (s *Service) UpdateDataviewObjectOrder(
	ctx *session.Context, req pb.RpcBlockDataviewObjectOrderUpdateRequest,
) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateViewObjectOrder(ctx, req.BlockId, req.ObjectOrders)
	})
}

func (s *Service) DeleteDataviewView(ctx *session.Context, req pb.RpcBlockDataviewViewDeleteRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.DeleteView(ctx, req.BlockId, req.ViewId, true)
	})
}

func (s *Service) SetDataviewActiveView(ctx *session.Context, req pb.RpcBlockDataviewViewSetActiveRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.SetActiveView(ctx, req.BlockId, req.ViewId, int(req.Limit), int(req.Offset))
	})
}

func (s *Service) SetDataviewViewPosition(ctx *session.Context, req pb.RpcBlockDataviewViewSetPositionRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.SetViewPosition(ctx, req.BlockId, req.ViewId, req.Position)
	})
}

func (s *Service) CreateDataviewView(
	ctx *session.Context, req pb.RpcBlockDataviewViewCreateRequest,
) (id string, err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		if req.View == nil {
			req.View = &model.BlockContentDataviewView{}
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

func (s *Service) AddDataviewRelation(ctx *session.Context, req pb.RpcBlockDataviewRelationAddRequest) (err error) {
	err = s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.AddRelations(ctx, req.BlockId, req.RelationKeys, true)
	})

	return
}

func (s *Service) DeleteDataviewRelation(ctx *session.Context, req pb.RpcBlockDataviewRelationDeleteRequest) error {
	return s.DoDataview(req.ContextId, func(b dataview.Dataview) error {
		return b.DeleteRelations(ctx, req.BlockId, req.RelationKeys, true)
	})
}

func (s *Service) SetDataviewSource(ctx *session.Context, contextId, blockId string, source []string) (err error) {
	return s.DoDataview(contextId, func(b dataview.Dataview) error {
		return b.SetSource(ctx, blockId, source)
	})
}

func (s *Service) Copy(
	req pb.RpcBlockCopyRequest,
) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		textSlot, htmlSlot, anySlot, err = cb.Copy(req)
		return err
	})

	return textSlot, htmlSlot, anySlot, err
}

func (s *Service) Paste(
	ctx *session.Context, req pb.RpcBlockPasteRequest, groupId string,
) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.Paste(ctx, &req, groupId)
		return err
	})

	return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
}

func (s *Service) Cut(
	ctx *session.Context, req pb.RpcBlockCutRequest,
) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		textSlot, htmlSlot, anySlot, err = cb.Cut(ctx, req)
		return err
	})
	return textSlot, htmlSlot, anySlot, err
}

func (s *Service) Export(req pb.RpcBlockExportRequest) (path string, err error) {
	err = s.DoClipboard(req.ContextId, func(cb clipboard.Clipboard) error {
		path, err = cb.Export(req)
		return err
	})
	return path, err
}

func (s *Service) SetTextText(ctx *session.Context, req pb.RpcBlockTextSetTextRequest) error {
	return s.DoText(req.ContextId, func(b stext.Text) error {
		return b.SetText(ctx, req)
	})
}

func (s *Service) SetLatexText(ctx *session.Context, req pb.RpcBlockLatexSetTextRequest) error {
	return Do(s, req.ContextId, func(b basic.CommonOperations) error {
		return b.SetLatexText(ctx, req)
	})
}

func (s *Service) SetTextStyle(
	ctx *session.Context, contextId string, style model.BlockContentTextStyle, blockIds ...string,
) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			t.SetStyle(style)
			return nil
		})
	})
}

func (s *Service) SetTextChecked(ctx *session.Context, req pb.RpcBlockTextSetCheckedRequest) error {
	return s.DoText(req.ContextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(ctx, []string{req.BlockId}, true, func(t text.Block) error {
			t.SetChecked(req.Checked)
			return nil
		})
	})
}

func (s *Service) SetTextColor(ctx *session.Context, contextId string, color string, blockIds ...string) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			t.SetTextColor(color)
			return nil
		})
	})
}

func (s *Service) ClearTextStyle(ctx *session.Context, contextId string, blockIds ...string) error {
	return s.DoText(contextId, func(b stext.Text) error {
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

func (s *Service) ClearTextContent(ctx *session.Context, contextId string, blockIds ...string) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.UpdateTextBlocks(ctx, blockIds, true, func(t text.Block) error {
			return t.SetText("", nil)
		})
	})
}

func (s *Service) SetTextMark(
	ctx *session.Context, contextId string, mark *model.BlockContentTextMark, blockIds ...string,
) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.SetMark(ctx, mark, blockIds...)
	})
}

func (s *Service) SetTextIcon(ctx *session.Context, contextId, image, emoji string, blockIds ...string) error {
	return s.DoText(contextId, func(b stext.Text) error {
		return b.SetIcon(ctx, image, emoji, blockIds...)
	})
}

func (s *Service) SetBackgroundColor(
	ctx *session.Context, contextId string, color string, blockIds ...string,
) (err error) {
	return Do(s, contextId, func(b basic.Updatable) error {
		return b.Update(ctx, func(b simple.Block) error {
			b.Model().BackgroundColor = color
			return nil
		}, blockIds...)
	})
}

func (s *Service) SetLinkAppearance(ctx *session.Context, req pb.RpcBlockLinkListSetAppearanceRequest) (err error) {
	return Do(s, req.ContextId, func(b basic.Updatable) error {
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
	ctx *session.Context, contextId string, align model.BlockAlign, blockIds ...string,
) (err error) {
	return s.Do(contextId, func(sb smartblock.SmartBlock) error {
		return sb.SetAlign(ctx, align, blockIds...)
	})
}

func (s *Service) SetVerticalAlign(
	ctx *session.Context, contextId string, align model.BlockVerticalAlign, blockIds ...string,
) (err error) {
	return s.Do(contextId, func(sb smartblock.SmartBlock) error {
		return sb.SetVerticalAlign(ctx, align, blockIds...)
	})
}

func (s *Service) SetLayout(ctx *session.Context, contextId string, layout model.ObjectTypeLayout) (err error) {
	return s.Do(contextId, func(sb smartblock.SmartBlock) error {
		return sb.SetLayout(ctx, layout)
	})
}

func (s *Service) FeaturedRelationAdd(ctx *session.Context, contextId string, relations ...string) error {
	return Do(s, contextId, func(b basic.CommonOperations) error {
		return b.FeaturedRelationAdd(ctx, relations...)
	})
}

func (s *Service) FeaturedRelationRemove(ctx *session.Context, contextId string, relations ...string) error {
	return Do(s, contextId, func(b basic.CommonOperations) error {
		return b.FeaturedRelationRemove(ctx, relations...)
	})
}

func (s *Service) UploadBlockFile(ctx *session.Context, req pb.RpcBlockUploadRequest, groupId string) (err error) {
	return s.DoFile(req.ContextId, func(b file.File) error {
		err = b.Upload(ctx, req.BlockId, file.FileSource{
			Path:    req.FilePath,
			Url:     req.Url,
			GroupId: groupId,
		}, false)
		return err
	})
}

func (s *Service) UploadBlockFileSync(ctx *session.Context, req pb.RpcBlockUploadRequest) (err error) {
	return s.DoFile(req.ContextId, func(b file.File) error {
		err = b.Upload(ctx, req.BlockId, file.FileSource{
			Path: req.FilePath,
			Url:  req.Url,
		}, true)
		return err
	})
}

func (s *Service) CreateAndUploadFile(
	ctx *session.Context, req pb.RpcBlockFileCreateAndUploadRequest,
) (id string, err error) {
	err = s.DoFile(req.ContextId, func(b file.File) error {
		id, err = b.CreateAndUpload(ctx, req)
		return err
	})
	return
}

func (s *Service) UploadFile(req pb.RpcFileUploadRequest) (hash string, err error) {
	upl := file.NewUploader(s)
	if req.DisableEncryption {
		log.Errorf("DisableEncryption is deprecated and has no effect")
	}

	upl.SetStyle(req.Style)
	if req.Type != model.BlockContentFile_None {
		upl.SetType(req.Type)
	} else {
		upl.AutoType(true)
	}
	if req.LocalPath != "" {
		upl.SetFile(req.LocalPath)
	} else if req.Url != "" {
		upl.SetUrl(req.Url)
	}
	res := upl.Upload(context.TODO())
	if res.Err != nil {
		return "", res.Err
	}
	return res.Hash, nil
}

func (s *Service) DropFiles(req pb.RpcFileDropRequest) (err error) {
	return s.DoFileNonLock(req.ContextId, func(b file.File) error {
		return b.DropFiles(req)
	})
}

func (s *Service) SetFileStyle(
	ctx *session.Context, contextId string, style model.BlockContentFileStyle, blockIds ...string,
) error {
	return s.DoFile(contextId, func(b file.File) error {
		return b.SetFileStyle(ctx, style, blockIds...)
	})
}

func (s *Service) UploadFileBlockWithHash(
	ctx *session.Context, contextId string, req pb.RpcBlockUploadRequest,
) (hash string, err error) {
	err = s.DoFile(contextId, func(b file.File) error {
		res, err := b.UploadFileWithHash(req.BlockId, file.FileSource{
			Path:    req.FilePath,
			Url:     req.Url,
			GroupId: "",
		})
		if err != nil {
			return err
		}
		hash = res.Hash
		return nil
	})

	return hash, err
}

func (s *Service) Undo(
	ctx *session.Context, req pb.RpcObjectUndoRequest,
) (counters pb.RpcObjectUndoRedoCounter, err error) {
	err = s.DoHistory(req.ContextId, func(b basic.IHistory) error {
		counters, err = b.Undo(ctx)
		return err
	})
	return
}

func (s *Service) Redo(
	ctx *session.Context, req pb.RpcObjectRedoRequest,
) (counters pb.RpcObjectUndoRedoCounter, err error) {
	err = s.DoHistory(req.ContextId, func(b basic.IHistory) error {
		counters, err = b.Redo(ctx)
		return err
	})
	return
}

func (s *Service) BookmarkFetch(ctx *session.Context, req pb.RpcBlockBookmarkFetchRequest) (err error) {
	return s.DoBookmark(req.ContextId, func(b bookmark.Bookmark) error {
		return b.Fetch(ctx, req.BlockId, req.Url, false)
	})
}

func (s *Service) BookmarkFetchSync(ctx *session.Context, req pb.RpcBlockBookmarkFetchRequest) (err error) {
	return s.DoBookmark(req.ContextId, func(b bookmark.Bookmark) error {
		return b.Fetch(ctx, req.BlockId, req.Url, true)
	})
}

func (s *Service) BookmarkCreateAndFetch(
	ctx *session.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest,
) (id string, err error) {
	err = s.DoBookmark(req.ContextId, func(b bookmark.Bookmark) error {
		id, err = b.CreateAndFetch(ctx, req)
		return err
	})
	return
}

func (s *Service) SetRelationKey(ctx *session.Context, req pb.RpcBlockRelationSetKeyRequest) error {
	return Do(s, req.ContextId, func(b basic.CommonOperations) error {
		rel, err := s.relationService.FetchKey(req.Key)
		if err != nil {
			return err
		}
		return b.AddRelationAndSet(ctx, s.relationService, pb.RpcBlockRelationAddRequest{
			RelationKey: rel.Key, BlockId: req.BlockId, ContextId: req.ContextId,
		})
	})
}

func (s *Service) AddRelationBlock(ctx *session.Context, req pb.RpcBlockRelationAddRequest) error {
	return Do(s, req.ContextId, func(b basic.CommonOperations) error {
		return b.AddRelationAndSet(ctx, s.relationService, req)
	})
}

func (s *Service) GetDocInfo(ctx context.Context, id string) (info doc.DocInfo, err error) {
	if err = s.DoWithContext(ctx, id, func(b smartblock.SmartBlock) error {
		info, err = b.GetDocInfo()
		return err
	}); err != nil {
		return
	}
	return
}

func (s *Service) Wakeup(id string) (err error) {
	return s.Do(id, func(b smartblock.SmartBlock) error {
		return nil
	})
}

func (s *Service) GetRelations(objectId string) (relations []*model.Relation, err error) {
	err = s.Do(objectId, func(b smartblock.SmartBlock) error {
		relations = b.Relations(nil).Models()
		return nil
	})
	return
}

// ModifyDetails performs details get and update under the sb lock to make sure no modifications are done in the middle
func (s *Service) ModifyDetails(
	objectId string, modifier func(current *types.Struct) (*types.Struct, error),
) (err error) {
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

// ModifyLocalDetails modifies local details of the object in cache,
// and if it is not found, sets pending details in object store
func (s *Service) ModifyLocalDetails(
	objectId string, modifier func(current *types.Struct) (*types.Struct, error),
) (err error) {
	if modifier == nil {
		return fmt.Errorf("modifier is nil")
	}
	// we set pending details if object is not in cache
	// we do this under lock to prevent races if the object is created in parallel
	// because in that case we can lose changes
	err = s.cache.ObjectCache().DoLockedIfNotExists(objectId, func() error {
		objectDetails, err := s.objectStore.GetPendingLocalDetails(objectId)
		if err != nil && err != ds.ErrNotFound {
			return err
		}
		var details *types.Struct
		if objectDetails != nil {
			details = objectDetails.GetDetails()
		}
		modifiedDetails, err := modifier(details)
		if err != nil {
			return err
		}
		return s.objectStore.UpdatePendingLocalDetails(objectId, modifiedDetails)
	})
	if err != nil && err != ocache.ErrExists {
		return err
	}
	err = s.Do(objectId, func(b smartblock.SmartBlock) error {
		// we just need to invoke the smartblock so it reads from pending details
		// no need to call modify twice
		if err == nil {
			return nil
		}

		dets, err := modifier(b.CombinedDetails())
		if err != nil {
			return err
		}

		return b.Apply(b.NewState().SetDetails(dets))
	})
	// that means that we will apply the change later as soon as the block is loaded by thread queue
	if err == source.ErrObjectNotFound {
		return nil
	}
	return err
}

func (s *Service) AddExtraRelations(ctx *session.Context, objectId string, relationIds []string) (err error) {
	return s.Do(objectId, func(b smartblock.SmartBlock) error {
		return b.AddRelationLinks(ctx, relationIds...)
	})
}

func (s *Service) SetObjectTypes(ctx *session.Context, objectId string, objectTypes []string) (err error) {
	return s.Do(objectId, func(b smartblock.SmartBlock) error {
		return b.SetObjectTypes(ctx, objectTypes)
	})
}

func (s *Service) DeleteObjectFromWorkspace(workspaceId string, objectId string) error {
	return s.Do(workspaceId, func(b smartblock.SmartBlock) error {
		workspace, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("incorrect object with workspace id")
		}

		st, err := coresb.SmartBlockTypeFromID(objectId)
		if err != nil {
			return err
		}
		if st == coresb.SmartBlockTypeSubObject {
			return workspace.DeleteSubObject(objectId)
		}

		return workspace.DeleteObject(objectId)
	})
}

func (s *Service) RemoveExtraRelations(ctx *session.Context, objectTypeId string, relationKeys []string) (err error) {
	return s.Do(objectTypeId, func(b smartblock.SmartBlock) error {
		return b.RemoveExtraRelations(ctx, relationKeys)
	})
}

func (s *Service) ListAvailableRelations(objectId string) (aggregatedRelations []*model.Relation, err error) {
	err = s.Do(objectId, func(b smartblock.SmartBlock) error {
		// TODO: not implemented
		return nil
	})
	return
}

func (s *Service) ListConvertToObjects(
	ctx *session.Context, req pb.RpcBlockListConvertToObjectsRequest,
) (linkIds []string, err error) {
	err = Do(s, req.ContextId, func(b basic.CommonOperations) error {
		linkIds, err = b.ExtractBlocksToObjects(ctx, s.objectCreator, req)
		return err
	})
	return
}

func (s *Service) MoveBlocksToNewPage(
	ctx *session.Context, req pb.RpcBlockListMoveToNewObjectRequest,
) (linkID string, err error) {
	// 1. Create new page, link
	linkID, objectID, err := s.CreateLinkToTheNewObject(ctx, &pb.RpcBlockLinkCreateWithObjectRequest{
		ContextId: req.ContextId,
		TargetId:  req.DropTargetId,
		Position:  req.Position,
		Details:   req.Details,
	})
	if err != nil {
		return
	}

	// 2. Move blocks to new page
	err = DoState(s, req.ContextId, func(srcState *state.State, sb basic.Movable) error {
		return DoState(s, objectID, func(destState *state.State, tb basic.Movable) error {
			return sb.Move(srcState, destState, "", model.Block_Inner, req.BlockIds)
		})
	})
	if err != nil {
		return
	}
	return linkID, err
}

func (s *Service) MoveBlocks(ctx *session.Context, req pb.RpcBlockListMoveToExistingObjectRequest) error {
	return DoState2(s, req.ContextId, req.TargetContextId, func(srcState, destState *state.State, sb, tb basic.Movable) error {
		return sb.Move(srcState, destState, req.DropTargetId, req.Position, req.BlockIds)
	})
}

func (s *Service) CreateTableBlock(ctx *session.Context, req pb.RpcBlockTableCreateRequest) (id string, err error) {
	err = DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		id, err = e.TableCreate(st, req)
		return err
	})
	return
}

func (s *Service) TableRowCreate(ctx *session.Context, req pb.RpcBlockTableRowCreateRequest) error {
	return DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		_, err := e.RowCreate(st, req)
		return err
	})
}

func (s *Service) TableColumnCreate(ctx *session.Context, req pb.RpcBlockTableColumnCreateRequest) error {
	return DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		_, err := e.ColumnCreate(st, req)
		return err
	})
}

func (s *Service) TableRowDelete(ctx *session.Context, req pb.RpcBlockTableRowDeleteRequest) (err error) {
	err = DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.RowDelete(st, req)
	})
	return
}

func (s *Service) TableColumnDelete(ctx *session.Context, req pb.RpcBlockTableColumnDeleteRequest) (err error) {
	err = DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.ColumnDelete(st, req)
	})
	return
}

func (s *Service) TableColumnMove(ctx *session.Context, req pb.RpcBlockTableColumnMoveRequest) (err error) {
	err = DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.ColumnMove(st, req)
	})
	return
}

func (s *Service) TableRowDuplicate(ctx *session.Context, req pb.RpcBlockTableRowDuplicateRequest) error {
	return DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		_, err := e.RowDuplicate(st, req)
		return err
	})
}

func (s *Service) TableColumnDuplicate(
	ctx *session.Context, req pb.RpcBlockTableColumnDuplicateRequest,
) (id string, err error) {
	err = DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		id, err = e.ColumnDuplicate(st, req)
		return err
	})
	return id, err
}

func (s *Service) TableExpand(ctx *session.Context, req pb.RpcBlockTableExpandRequest) (err error) {
	err = DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.Expand(st, req)
	})
	return err
}

func (s *Service) TableRowListFill(ctx *session.Context, req pb.RpcBlockTableRowListFillRequest) (err error) {
	err = DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.RowListFill(st, req)
	})
	return err
}

func (s *Service) TableRowListClean(ctx *session.Context, req pb.RpcBlockTableRowListCleanRequest) (err error) {
	err = DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.RowListClean(st, req)
	})
	return err
}

func (s *Service) TableRowSetHeader(ctx *session.Context, req pb.RpcBlockTableRowSetHeaderRequest) (err error) {
	err = DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.RowSetHeader(st, req)
	})
	return err
}

func (s *Service) TableSort(ctx *session.Context, req pb.RpcBlockTableSortRequest) (err error) {
	err = DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.Sort(st, req)
	})
	return err
}

func (s *Service) TableColumnListFill(ctx *session.Context, req pb.RpcBlockTableColumnListFillRequest) (err error) {
	err = DoStateCtx(s, ctx, req.ContextId, func(st *state.State, e table.TableEditor) error {
		return e.ColumnListFill(st, req)
	})
	return err
}

func (s *Service) CreateWidgetBlock(ctx *session.Context, req *pb.RpcBlockCreateWidgetRequest) (string, error) {
	var id string
	err := DoStateCtx(s, ctx, req.ContextId, func(st *state.State, w widget.Widget) error {
		var err error
		id, err = w.CreateBlock(st, req)
		return err
	})
	return id, err
}

func (s *Service) CopyDataviewToBlock(ctx *session.Context,
	req *pb.RpcBlockDataviewCreateFromExistingObjectRequest) ([]*model.BlockContentDataviewView, error) {

	var targetDvContent *model.BlockContentDataview

	err := s.DoDataview(req.TargetObjectId, func(d dataview.Dataview) error {
		var err error
		targetDvContent, err = d.GetDataview(template.DataviewBlockId)
		return err
	})
	if err != nil {
		return nil, err
	}

	err = s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
		st := b.NewStateCtx(ctx)
		block := st.Get(req.BlockId)

		dvContent, ok := block.Model().Content.(*model.BlockContentOfDataview)
		if !ok {
			return fmt.Errorf("block must contain dataView content")
		}

		dvContent.Dataview.Views = targetDvContent.Views
		dvContent.Dataview.RelationLinks = targetDvContent.RelationLinks
		dvContent.Dataview.GroupOrders = targetDvContent.GroupOrders
		dvContent.Dataview.ObjectOrders = targetDvContent.ObjectOrders
		dvContent.Dataview.TargetObjectId = req.TargetObjectId

		return b.Apply(st)
	})
	if err != nil {
		return nil, err
	}

	return targetDvContent.Views, err
}
