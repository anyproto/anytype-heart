package notion

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/types"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// mappedBlock is one anytype block plus its nested children — the tree is
// built by construction, so a child can never end up under two parents
// (v1's to_do flattening bug).
type mappedBlock struct {
	block    *model.Block
	children []*mappedBlock
}

type mapContext struct {
	pageId string
	// blockIds is the set of block ids fetched within this page's tree —
	// the ownership check for block-parented child entities.
	blockIds map[string]struct{}
}

func dashless(id string) string {
	return strings.ReplaceAll(id, "-", "")
}

// mapBlocks converts a fetched subtree. One notion block may yield several
// sibling anytype blocks (captions, extracted equations).
func (c *Converter) mapBlocks(ctx context.Context, mctx mapContext, blocks []notionBlock, sink importv2.Sink) ([]*mappedBlock, error) {
	var mapped []*mappedBlock
	for i := range blocks {
		nodes, err := c.mapBlock(ctx, mctx, &blocks[i], sink)
		if err != nil {
			return nil, err
		}
		mapped = append(mapped, nodes...)
	}
	return mapped, nil
}

type textPayload struct {
	RichText     []richText `json:"rich_text"`
	Checked      bool       `json:"checked"`
	IsToggleable bool       `json:"is_toggleable"`
	Color        string     `json:"color"`
	Language     string     `json:"language"`
	Caption      []richText `json:"caption"`
	Icon         *iconValue `json:"icon"`
	Expression   string     `json:"expression"`
	Url          string     `json:"url"`
}

func (c *Converter) mapBlock(ctx context.Context, mctx mapContext, block *notionBlock, sink importv2.Sink) ([]*mappedBlock, error) {
	switch block.Type {
	case "paragraph":
		return c.mapText(ctx, mctx, block, model.BlockContentText_Paragraph, sink)
	case "heading_1":
		return c.mapText(ctx, mctx, block, model.BlockContentText_Header1, sink)
	case "heading_2":
		return c.mapText(ctx, mctx, block, model.BlockContentText_Header2, sink)
	case "heading_3":
		return c.mapText(ctx, mctx, block, model.BlockContentText_Header3, sink)
	case "heading_4":
		return c.mapText(ctx, mctx, block, model.BlockContentText_Header4, sink)
	case "bulleted_list_item":
		return c.mapText(ctx, mctx, block, model.BlockContentText_Marked, sink)
	case "numbered_list_item":
		return c.mapText(ctx, mctx, block, model.BlockContentText_Numbered, sink)
	case "to_do":
		return c.mapText(ctx, mctx, block, model.BlockContentText_Checkbox, sink)
	case "toggle":
		return c.mapText(ctx, mctx, block, model.BlockContentText_Toggle, sink)
	case "quote":
		return c.mapText(ctx, mctx, block, model.BlockContentText_Quote, sink)
	case "callout":
		return c.mapText(ctx, mctx, block, model.BlockContentText_Callout, sink)
	case "code":
		return c.mapCode(ctx, mctx, block, sink)
	case "equation":
		var payload textPayload
		if err := block.decode(&payload); err != nil {
			return c.unsupported(block, sink), nil
		}
		return []*mappedBlock{latexBlock(block.Id, payload.Expression, model.BlockContentLatex_Latex)}, nil
	case "divider":
		return []*mappedBlock{{block: &model.Block{
			Id:      block.Id,
			Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{Style: model.BlockContentDiv_Line}},
		}}}, nil
	case "table_of_contents":
		return []*mappedBlock{{block: &model.Block{
			Id:      block.Id,
			Content: &model.BlockContentOfTableOfContents{TableOfContents: &model.BlockContentTableOfContents{}},
		}}}, nil
	case "column_list":
		return c.mapLayout(ctx, mctx, block, model.BlockContentLayout_Row, sink)
	case "column":
		return c.mapLayout(ctx, mctx, block, model.BlockContentLayout_Column, sink)
	case "table":
		return c.mapTable(ctx, block, sink)
	case "image":
		return c.mapFile(ctx, block, model.BlockContentFile_Image, sink)
	case "pdf":
		return c.mapFile(ctx, block, model.BlockContentFile_PDF, sink)
	case "file":
		return c.mapFile(ctx, block, model.BlockContentFile_File, sink)
	case "video":
		return c.mapMedia(ctx, block, model.BlockContentFile_Video, sink)
	case "audio":
		return c.mapMedia(ctx, block, model.BlockContentFile_Audio, sink)
	case "embed":
		return c.mapEmbed(block, sink)
	case "link_preview":
		var payload textPayload
		if err := block.decode(&payload); err != nil || payload.Url == "" {
			return c.unsupported(block, sink), nil
		}
		return []*mappedBlock{textLinkBlock(block.Id, payload.Url, payload.Url)}, nil
	case "bookmark":
		var payload textPayload
		if err := block.decode(&payload); err != nil {
			return c.unsupported(block, sink), nil
		}
		return []*mappedBlock{{block: &model.Block{
			Id: block.Id,
			Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{
				Url:   payload.Url,
				Title: plainText(payload.Caption),
			}},
		}}}, nil
	case "child_page":
		return c.mapChildEntity(ctx, mctx, block, false, sink)
	case "child_database":
		return c.mapChildEntity(ctx, mctx, block, true, sink)
	case "link_to_page":
		return c.mapLinkToPage(ctx, block, sink)
	case "synced_block":
		// Transparent container: the (already resolved) content hoists up.
		return c.mapBlocks(ctx, mctx, block.children, sink)
	default:
		return c.unsupported(block, sink), nil
	}
}

// mapText renders a text-like block. Children (incl. toggleable headings —
// v1 flattened those to siblings) nest under the primary block. Inline
// equations split the text in place, keeping their position.
func (c *Converter) mapText(ctx context.Context, mctx mapContext, block *notionBlock, style model.BlockContentTextStyle, sink importv2.Sink) ([]*mappedBlock, error) {
	var payload textPayload
	if err := block.decode(&payload); err != nil {
		return c.unsupported(block, sink), nil
	}
	children, err := c.mapBlocks(ctx, mctx, block.children, sink)
	if err != nil {
		return nil, err
	}

	makeText := func(piece renderedPiece, primary bool, index int) *model.Block {
		text := &model.BlockContentText{
			Text:  piece.text,
			Style: model.BlockContentText_Paragraph,
			Marks: &model.BlockContentTextMarks{Marks: piece.marks},
		}
		modelBlock := &model.Block{Content: &model.BlockContentOfText{Text: text}}
		if primary {
			modelBlock.Id = block.Id
			text.Style = style
			text.Checked = payload.Checked
			if payload.Color != "" && payload.Color != "default" {
				if strings.Contains(payload.Color, "background") {
					modelBlock.BackgroundColor = anytypeColor(payload.Color)
				} else {
					text.Color = anytypeColor(payload.Color)
				}
			}
		} else {
			modelBlock.Id = fmt.Sprintf("%s-t%d", block.Id, index)
		}
		return modelBlock
	}

	pieces := c.renderRichTextPieces(payload.RichText)
	var nodes []*mappedBlock
	var primary *mappedBlock
	for i, piece := range pieces {
		if piece.equation != "" {
			nodes = append(nodes, latexBlock(fmt.Sprintf("%s-eq-%d", block.Id, i), piece.equation, model.BlockContentLatex_Latex))
			continue
		}
		node := &mappedBlock{block: makeText(piece, primary == nil, i)}
		if primary == nil {
			primary = node
		}
		nodes = append(nodes, node)
	}
	// Pure-equation blocks with no children render as latex only; anything
	// needing a host (children, callout icon, empty text) gets one.
	if primary == nil {
		if len(children) == 0 && len(nodes) > 0 && style != model.BlockContentText_Callout {
			return nodes, nil
		}
		primary = &mappedBlock{block: makeText(renderedPiece{}, true, 0)}
		nodes = append([]*mappedBlock{primary}, nodes...)
	}
	primary.children = children

	if style == model.BlockContentText_Callout && payload.Icon != nil {
		text := primary.block.GetText()
		if payload.Icon.Type == "emoji" {
			text.IconEmoji = payload.Icon.Emoji
		} else if iconUrl := payload.Icon.fileUrl(); iconUrl != "" {
			sourceKey, err := c.emitFileFromUrl(ctx, sink, iconUrl, "icon", payload.Icon.isExternal())
			if err != nil {
				return nil, err
			}
			text.IconImage = sourceKey
		}
	}
	return nodes, nil
}

func (c *Converter) mapCode(ctx context.Context, mctx mapContext, block *notionBlock, sink importv2.Sink) ([]*mappedBlock, error) {
	var payload textPayload
	if err := block.decode(&payload); err != nil {
		return c.unsupported(block, sink), nil
	}
	if payload.Language == "mermaid" {
		return []*mappedBlock{latexBlock(block.Id, plainText(payload.RichText), model.BlockContentLatex_Mermaid)}, nil
	}
	rendered := c.renderRichText(payload.RichText)
	codeBlock := &model.Block{
		Id:     block.Id,
		Fields: fieldsWith("lang", payload.Language),
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:  rendered.text,
			Style: model.BlockContentText_Code,
		}},
	}
	nodes := []*mappedBlock{{block: codeBlock}}
	nodes = append(nodes, c.captionBlocks(block.Id, payload.Caption)...)
	return nodes, nil
}

// captionBlocks keeps captions as text under the captioned block (approved
// decision — v1 dropped every caption except bookmark titles).
func (c *Converter) captionBlocks(parentId string, caption []richText) []*mappedBlock {
	text := plainText(caption)
	if text == "" {
		return nil
	}
	return []*mappedBlock{{block: &model.Block{
		Id: parentId + "-caption",
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:  text,
			Style: model.BlockContentText_Paragraph,
			Color: "grey",
		}},
	}}}
}

type filePayload struct {
	fileValue
	Caption []richText `json:"caption"`
}

// emptyMedia handles a media/embed block whose URL is empty or inaccessible
// (the API returns "" when the file has no content or the integration lacks
// access) — an accurate warning, not a false "unsupported block type", and
// any caption is preserved.
func (c *Converter) emptyMedia(block *notionBlock, caption []richText, sink importv2.Sink) []*mappedBlock {
	sink.Issue(importv2.Warning(importv2.IssueDataLoss, block.Id,
		fmt.Sprintf("%s block has no accessible URL (empty or not shared with the integration); skipped", block.Type)))
	return c.captionBlocks(block.Id, caption)
}

func (c *Converter) mapFile(ctx context.Context, block *notionBlock, fileType model.BlockContentFileType, sink importv2.Sink) ([]*mappedBlock, error) {
	var payload filePayload
	if err := block.decode(&payload); err != nil {
		return c.unsupported(block, sink), nil
	}
	if payload.url() == "" {
		return c.emptyMedia(block, payload.Caption, sink), nil
	}
	sourceKey, err := c.emitFileFromUrl(ctx, sink, payload.url(), payload.Name, payload.isExternal())
	if err != nil {
		return nil, err
	}
	nodes := []*mappedBlock{{block: &model.Block{
		Id: block.Id,
		Content: &model.BlockContentOfFile{File: &model.BlockContentFile{
			Type:           fileType,
			TargetObjectId: sourceKey,
			State:          model.BlockContentFile_Done,
		}},
	}}}
	nodes = append(nodes, c.captionBlocks(block.Id, payload.Caption)...)
	return nodes, nil
}

// mapMedia embeds recognized streaming services as latex processors and
// imports everything else as a file.
func (c *Converter) mapMedia(ctx context.Context, block *notionBlock, fileType model.BlockContentFileType, sink importv2.Sink) ([]*mappedBlock, error) {
	var payload filePayload
	if err := block.decode(&payload); err != nil {
		return c.unsupported(block, sink), nil
	}
	if payload.url() == "" {
		return c.emptyMedia(block, payload.Caption, sink), nil
	}
	mediaUrl := payload.url()
	if processor, ok := embedProcessorOf(mediaUrl); ok {
		nodes := []*mappedBlock{latexBlock(block.Id, mediaUrl, processor)}
		return append(nodes, c.captionBlocks(block.Id, payload.Caption)...), nil
	}
	return c.mapFile(ctx, block, fileType, sink)
}

func (c *Converter) mapEmbed(block *notionBlock, sink importv2.Sink) ([]*mappedBlock, error) {
	var payload textPayload
	if err := block.decode(&payload); err != nil {
		return c.unsupported(block, sink), nil
	}
	if payload.Url == "" {
		return c.emptyMedia(block, payload.Caption, sink), nil
	}
	if processor, ok := embedProcessorOf(payload.Url); ok {
		nodes := []*mappedBlock{latexBlock(block.Id, payload.Url, processor)}
		return append(nodes, c.captionBlocks(block.Id, payload.Caption)...), nil
	}
	nodes := []*mappedBlock{textLinkBlock(block.Id, payload.Url, payload.Url)}
	return append(nodes, c.captionBlocks(block.Id, payload.Caption)...), nil
}

func embedProcessorOf(rawUrl string) (model.BlockContentLatexProcessor, bool) {
	switch {
	case strings.Contains(rawUrl, "youtube.com") || strings.Contains(rawUrl, "youtu.be"):
		return model.BlockContentLatex_Youtube, true
	case strings.Contains(rawUrl, "vimeo.com"):
		return model.BlockContentLatex_Vimeo, true
	case strings.Contains(rawUrl, "soundcloud.com"):
		return model.BlockContentLatex_Soundcloud, true
	case strings.Contains(rawUrl, "google.com/maps"):
		return model.BlockContentLatex_GoogleMaps, true
	case strings.Contains(rawUrl, "miro.com"):
		return model.BlockContentLatex_Miro, true
	case strings.Contains(rawUrl, "gist.github.com"):
		return model.BlockContentLatex_GithubGist, true
	case strings.Contains(rawUrl, "codepen.io"):
		return model.BlockContentLatex_Codepen, true
	default:
		return 0, false
	}
}

func (c *Converter) mapLayout(ctx context.Context, mctx mapContext, block *notionBlock, style model.BlockContentLayoutStyle, sink importv2.Sink) ([]*mappedBlock, error) {
	children, err := c.mapBlocks(ctx, mctx, block.children, sink)
	if err != nil {
		return nil, err
	}
	return []*mappedBlock{{
		block: &model.Block{
			Id:      block.Id,
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: style}},
		},
		children: children,
	}}, nil
}

type tablePayload struct {
	TableWidth      int  `json:"table_width"`
	HasColumnHeader bool `json:"has_column_header"`
	HasRowHeader    bool `json:"has_row_header"`
}

type tableRowPayload struct {
	Cells [][]richText `json:"cells"`
}

// mapTable builds the anytype table subtree. has_column_header marks the
// first row as header (v1 applied has_row_header there — inverted);
// column headers have no anytype counterpart and degrade with a warning.
//
// Anytype cell ids are `rowID-colID` with exactly one dash (ParseCellID
// splits on the first dash; IsTableCell rejects multi-dash ids), so row and
// column ids derive from the dashed Notion UUIDs with the dashes stripped.
func (c *Converter) mapTable(ctx context.Context, block *notionBlock, sink importv2.Sink) ([]*mappedBlock, error) {
	var payload tablePayload
	if err := block.decode(&payload); err != nil {
		return c.unsupported(block, sink), nil
	}
	if payload.HasRowHeader {
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, block.Id,
			"table row headers (first column) have no anytype counterpart; imported as regular cells"))
	}

	columns := make([]*mappedBlock, 0, payload.TableWidth)
	columnIds := make([]string, 0, payload.TableWidth)
	for i := 0; i < payload.TableWidth; i++ {
		columnId := fmt.Sprintf("c%s%d", dashless(block.Id), i)
		columnIds = append(columnIds, columnId)
		columns = append(columns, &mappedBlock{block: &model.Block{
			Id:      columnId,
			Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}},
		}})
	}

	var rows []*mappedBlock
	for rowIndex := range block.children {
		child := &block.children[rowIndex]
		if child.Type != "table_row" {
			continue
		}
		var rowPayload tableRowPayload
		if err := child.decode(&rowPayload); err != nil {
			continue
		}
		rowId := "r" + dashless(child.Id)
		var cells []*mappedBlock
		for cellIndex, cellRuns := range rowPayload.Cells {
			if cellIndex >= len(columnIds) {
				break
			}
			rendered := c.renderRichText(cellRuns)
			cells = append(cells, &mappedBlock{block: &model.Block{
				Id: rowId + "-" + columnIds[cellIndex],
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Text:  rendered.text,
					Style: model.BlockContentText_Paragraph,
					Marks: &model.BlockContentTextMarks{Marks: rendered.marks},
				}},
			}})
		}
		rows = append(rows, &mappedBlock{
			block: &model.Block{
				Id: rowId,
				Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{
					IsHeader: payload.HasColumnHeader && rowIndex == 0,
				}},
			},
			children: cells,
		})
	}

	return []*mappedBlock{{
		block: &model.Block{Id: block.Id, Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}},
		children: []*mappedBlock{
			{
				block: &model.Block{
					Id:      block.Id + "-columns",
					Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}},
				},
				children: columns,
			},
			{
				block: &model.Block{
					Id:      block.Id + "-rows",
					Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}},
				},
				children: rows,
			},
		},
	}}, nil
}

type childEntityPayload struct {
	Title string `json:"title"`
}

// mapChildEntity resolves child_page/child_database (the API gives a title,
// not an id) against the pass-1 hierarchy: children of this page first,
// then a globally unique title. Ambiguity degrades explicitly. A
// block-parented entity counts as "within this page" only when its parent
// block was actually fetched in this page's tree — an unrelated same-titled
// subpage nested in another page's column/toggle must not match.
func (c *Converter) mapChildEntity(ctx context.Context, mctx mapContext, block *notionBlock, wantDatabase bool, sink importv2.Sink) ([]*mappedBlock, error) {
	var payload childEntityPayload
	if err := block.decode(&payload); err != nil {
		return c.unsupported(block, sink), nil
	}

	// A child_page block's own id IS the child page's id; a child_database
	// block's id is the owning database's id. Resolve directly — this is
	// exact, unlike title matching which collides on "Untitled" siblings.
	// Hoisted synced-block content carries suffixed ids, so resolve via the
	// real Notion id.
	targetId := ""
	if wantDatabase {
		if id, ok := c.resolveDatabaseRef(block.notionId()); ok {
			targetId = id
		}
	} else if _, ok := c.entityById[block.notionId()]; ok {
		targetId = block.notionId()
	}

	// Fallback: title matching within the page (kept for entities the block
	// id can't resolve — e.g. an id shape mismatch).
	if targetId == "" {
		targetId = c.resolveChildByTitle(mctx, payload.Title, wantDatabase)
	}
	// Second chance (§16 item 3): the block id is the child's fetchable id —
	// a child /search omitted (eventual consistency) is claimed and imported
	// instead of degrading to a dead placeholder.
	if targetId == "" {
		if wantDatabase {
			targetId, _ = c.discoverDatabase(ctx, block.notionId(), sink)
		} else {
			targetId, _ = c.discoverPage(ctx, block.notionId(), sink)
		}
	}
	if targetId == "" {
		sink.Issue(importv2.Warning(importv2.IssueMissingTarget, mctx.pageId,
			fmt.Sprintf("child %q could not be resolved", payload.Title)))
		return []*mappedBlock{textBlock(block.Id, fmt.Sprintf("Unresolved link: %s", payload.Title))}, nil
	}
	return []*mappedBlock{{block: &model.Block{
		Id: block.Id,
		Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
			TargetBlockId: targetId,
			Style:         model.BlockContentLink_Page,
		}},
	}}}, nil
}

func (c *Converter) resolveChildByTitle(mctx mapContext, title string, wantDatabase bool) string {
	var withinPage, global []string
	for _, entity := range c.entityById {
		if entity.isCollectionLike() != wantDatabase || entity.Title != title {
			continue
		}
		global = append(global, entity.Id)
		parentedHere := entity.Parent.Type == "page_id" && entity.Parent.PageId == mctx.pageId
		if !parentedHere && entity.Parent.Type == "block_id" {
			_, parentedHere = mctx.blockIds[entity.Parent.BlockId]
		}
		if parentedHere {
			withinPage = append(withinPage, entity.Id)
		}
	}
	if len(withinPage) == 1 {
		return withinPage[0]
	}
	if len(withinPage) == 0 && len(global) == 1 {
		return global[0]
	}
	return ""
}

type linkToPagePayload struct {
	Type       string `json:"type"`
	PageId     string `json:"page_id"`
	DatabaseId string `json:"database_id"`
}

func (c *Converter) mapLinkToPage(ctx context.Context, block *notionBlock, sink importv2.Sink) ([]*mappedBlock, error) {
	var payload linkToPagePayload
	if err := block.decode(&payload); err != nil {
		return c.unsupported(block, sink), nil
	}
	targetId, resolved := "", false
	switch payload.Type {
	case "database_id":
		// Blocks still reference the database, not its data sources.
		targetId, resolved = c.resolveDatabaseRef(payload.DatabaseId)
		if !resolved {
			targetId, resolved = c.discoverDatabase(ctx, payload.DatabaseId, sink)
		}
	default:
		targetId = payload.PageId
		if _, resolved = c.entityById[targetId]; !resolved {
			targetId, resolved = c.discoverPage(ctx, payload.PageId, sink)
		}
	}
	if !resolved || targetId == "" {
		sink.Issue(importv2.Warning(importv2.IssueMissingTarget, block.Id,
			"link_to_page target was not part of the import"))
		return []*mappedBlock{textBlock(block.Id, "Unresolved link")}, nil
	}
	return []*mappedBlock{{block: &model.Block{
		Id: block.Id,
		Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
			TargetBlockId: targetId,
			Style:         model.BlockContentLink_Page,
		}},
	}}}, nil
}

// unsupported is the uniform placeholder: nothing is silently dropped.
func (c *Converter) unsupported(block *notionBlock, sink importv2.Sink) []*mappedBlock {
	sink.Issue(importv2.Warning(importv2.IssueUnsupportedBlock, block.Id,
		fmt.Sprintf("block type %q has no anytype counterpart; placeholder inserted", block.Type)))
	return []*mappedBlock{textBlock(block.Id, fmt.Sprintf("Unsupported block (%s)", block.Type))}
}

func textBlock(id, text string) *mappedBlock {
	return &mappedBlock{block: &model.Block{
		Id: id,
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:  text,
			Style: model.BlockContentText_Paragraph,
		}},
	}}
}

func textLinkBlock(id, text, url string) *mappedBlock {
	length := int32(len([]rune(text))) // ascii urls: rune==utf16 count
	return &mappedBlock{block: &model.Block{
		Id: id,
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:  text,
			Style: model.BlockContentText_Paragraph,
			Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{{
				Type:  model.BlockContentTextMark_Link,
				Param: url,
				Range: &model.Range{From: 0, To: length},
			}}},
		}},
	}}
}

func latexBlock(id, text string, processor model.BlockContentLatexProcessor) *mappedBlock {
	return &mappedBlock{block: &model.Block{
		Id:      id,
		Content: &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{Text: text, Processor: processor}},
	}}
}

func fieldsWith(key, value string) *types.Struct {
	return &types.Struct{Fields: map[string]*types.Value{
		key: {Kind: &types.Value_StringValue{StringValue: value}},
	}}
}
