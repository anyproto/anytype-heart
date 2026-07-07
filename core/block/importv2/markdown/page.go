package markdown

import (
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/import/common/filetime"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema/yaml"
	textutil "github.com/anyproto/anytype-heart/util/text"
)

// convertPage converts one markdown file and emits, in order: new property
// definitions, referenced file objects, then the page. Nothing about the
// page is retained afterwards.
func (c *Converter) convertPage(ctx context.Context, entry source.Entry, sink importv2.Sink) error {
	content, err := c.readEntry(ctx, entry.Name)
	if err != nil {
		sink.Issue(importv2.ObjectError(importv2.IssueObjectFailed, entry.Name, fmt.Errorf("read: %w", err)))
		return nil
	}

	frontMatter, body, err := yaml.ExtractYAMLFrontMatter(content)
	if err != nil {
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, entry.Name, "front-matter unreadable, imported as plain text"))
		body = content
	}
	var yamlDetails []domain.Detail
	var yamlLinks []*model.RelationLink
	typeKey := bundle.TypeKeyPage.String()
	if len(frontMatter) > 0 {
		parsed, err := yaml.ParseYAMLFrontMatterWithResolverAndPath(frontMatter, c.resolver, path.Dir(entry.Name))
		if err != nil {
			sink.Issue(importv2.Warning(importv2.IssueDataLoss, entry.Name, fmt.Sprintf("front-matter skipped: %s", err)))
		} else if parsed != nil {
			// The yaml parser yields properties in map order; sort for
			// deterministic emission (contract rule 5).
			sort.SliceStable(parsed.Properties, func(i, j int) bool {
				return parsed.Properties[i].Name < parsed.Properties[j].Name
			})
			yamlDetails, yamlLinks, typeKey, err = c.emitPropertyDefinitions(ctx, parsed.Properties, parsed.ObjectType, sink)
			if err != nil {
				return err
			}
		}
	}

	blocks, _, err := anymark.MarkdownToBlocks(body, path.Dir(entry.Name), nil, c.flavour.Anymark...)
	if err != nil {
		sink.Issue(importv2.ObjectError(importv2.IssueObjectFailed, entry.Name, fmt.Errorf("parse markdown: %w", err)))
		return nil
	}

	title := pageTitleFromPath(entry.Name)
	iconEmoji := ""
	if extracted, rest := extractH1Title(blocks); extracted != "" {
		title, blocks = extracted, rest
		iconEmoji, title = splitEmojiTitle(title)
	}
	if c.flavour.ExtractMetadata != nil {
		page := &pageContext{Name: entry.Name, Blocks: blocks}
		c.flavour.ExtractMetadata(c, page)
		blocks = page.Blocks
	}
	blocks, err = c.rewriteBlocks(ctx, entry.Name, blocks, sink)
	if err != nil {
		return err
	}
	if c.propertiesAsBlockEnabled() {
		blocks = append(propertyBlocks(yamlDetails), blocks...)
	}

	object := &importv2.Object{
		SourceKey: entry.Name,
		SbType:    coresb.SmartBlockTypePage,
		Payload: &importv2.Snapshot{
			Blocks:        blocks,
			Details:       domain.NewDetails(),
			ObjectTypes:   []string{typeKey},
			RelationLinks: yamlLinks,
		},
		IsRootCandidate: c.dirs == nil && isTopLevel(entry.Name),
	}
	c.stampCommonDetails(object, entry, title)
	if iconEmoji != "" {
		object.Payload.Details.SetString(bundle.RelationKeyIconEmoji, iconEmoji)
	}
	for _, detail := range yamlDetails {
		object.Payload.Details.Set(detail.Key, detail.Value)
	}
	return sink.Object(ctx, object)
}

// splitEmojiTitle extracts a leading emoji from an H1 title into the page
// icon (v1 convention, approximate unicode ranges as before).
func splitEmojiTitle(title string) (emoji, rest string) {
	trimmed := strings.TrimSpace(title)
	runes := []rune(trimmed)
	if len(runes) == 0 || !isEmojiRune(runes[0]) {
		return "", title
	}
	return string(runes[0]), strings.TrimSpace(string(runes[1:]))
}

func isEmojiRune(r rune) bool {
	return (r >= 0x1F000 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF)
}

// systemPropertyKeys are excluded from properties-as-blocks (v1 list).
var systemPropertyKeys = map[string]struct{}{
	bundle.RelationKeyName.String():             {},
	bundle.RelationKeyDescription.String():      {},
	bundle.RelationKeyType.String():             {},
	bundle.RelationKeyCreatedDate.String():      {},
	bundle.RelationKeyLastModifiedDate.String(): {},
	bundle.RelationKeyCreator.String():          {},
	bundle.RelationKeyLastModifiedBy.String():   {},
	bundle.RelationKeyId.String():               {},
	bundle.RelationKeyIconEmoji.String():        {},
	bundle.RelationKeyIconImage.String():        {},
	bundle.RelationKeyCoverId.String():          {},
	bundle.RelationKeyLayout.String():           {},
}

// propertyBlocks renders front-matter properties as relation blocks at the
// top of the page (the includePropertiesAsBlock option).
func propertyBlocks(details []domain.Detail) []*model.Block {
	blocks := make([]*model.Block, 0, len(details))
	for i, detail := range details {
		if _, system := systemPropertyKeys[string(detail.Key)]; system {
			continue
		}
		blocks = append(blocks, &model.Block{
			Id: fmt.Sprintf("property%d", i),
			Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{
				Key: string(detail.Key),
			}},
		})
	}
	return blocks
}

// readEntry buffers one file — bounded by a single document, never the set.
func (c *Converter) readEntry(ctx context.Context, name string) ([]byte, error) {
	reader, err := c.source.Open(ctx, name)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (c *Converter) stampCommonDetails(object *importv2.Object, entry source.Entry, title string) {
	details := object.Payload.Details
	if details == nil {
		details = domain.NewDetails()
		object.Payload.Details = details
	}
	details.SetString(bundle.RelationKeyName, title)
	details.SetString(bundle.RelationKeySourceFilePath, sourcePathHash(entry.Name))
	created, modified := entry.ModTime.Unix(), entry.ModTime.Unix()
	if entry.FSPath != "" {
		if fsCreated, fsModified := filetime.ExtractFileTimes(entry.FSPath); fsCreated != 0 || fsModified != 0 {
			created, modified = fsCreated, fsModified
		}
	}
	details.SetInt64(bundle.RelationKeyCreatedDate, created)
	details.SetInt64(bundle.RelationKeyLastModifiedDate, modified)
}

// extractH1Title returns the text of a leading Header1 block and the blocks
// without it (the v1 title convention; content loss is deliberate and
// documented).
func extractH1Title(blocks []*model.Block) (string, []*model.Block) {
	if len(blocks) == 0 {
		return "", blocks
	}
	text := blocks[0].GetText()
	if text == nil || text.Style != model.BlockContentText_Header1 {
		return "", blocks
	}
	return text.Text, blocks[1:]
}

// rewriteBlocks converts anymark output to source-key references: markdown
// link marks become mentions or page links, referenced local files become
// file blocks backed by emitted file objects, bare URL lines become
// bookmarks.
func (c *Converter) rewriteBlocks(ctx context.Context, pageName string, blocks []*model.Block, sink importv2.Sink) ([]*model.Block, error) {
	result := make([]*model.Block, 0, len(blocks))
	for _, block := range blocks {
		if file := block.GetFile(); file != nil && file.Name != "" {
			if err := c.rewriteFileBlock(ctx, pageName, block, sink); err != nil {
				return nil, err
			}
			result = append(result, block)
			continue
		}
		text := block.GetText()
		if text == nil {
			result = append(result, block)
			continue
		}
		replacement, err := c.rewriteTextBlock(ctx, pageName, block, sink)
		if err != nil {
			return nil, err
		}
		result = append(result, replacement)
	}
	return result, nil
}

func (c *Converter) rewriteFileBlock(ctx context.Context, pageName string, block *model.Block, sink importv2.Sink) error {
	file := block.GetFile()
	if anymark.IsUrl(file.Name) {
		return nil // remote media stays a url-backed block
	}
	entryName, found := c.lookupEntry(file.Name)
	if !found {
		sink.Issue(importv2.Warning(importv2.IssueMissingTarget, pageName,
			fmt.Sprintf("file %q referenced but not present in the source", file.Name)))
		file.Name = ""
		return nil
	}
	if err := c.emitFileObject(ctx, entryName, sink); err != nil {
		return err
	}
	file.TargetObjectId = entryName
	file.Name = ""
	return nil
}

func (c *Converter) rewriteTextBlock(ctx context.Context, pageName string, block *model.Block, sink importv2.Sink) (*model.Block, error) {
	text := block.GetText()
	marks := text.GetMarks().GetMarks()

	// A block that is exactly one whole-line link becomes a dedicated block.
	if len(marks) == 1 && marks[0].Type == model.BlockContentTextMark_Link && isWholeLine(text.Text, marks[0]) {
		if replacement, err, handled := c.convertWholeLineLink(ctx, pageName, block, marks[0].Param, sink); handled {
			return replacement, err
		}
	}

	for _, mark := range marks {
		if mark.Type != model.BlockContentTextMark_Link {
			continue
		}
		if anymark.IsUrl(mark.Param) {
			continue
		}
		entryName, found := c.lookupEntry(mark.Param)
		if !found {
			continue // not a source file: leave the mark untouched
		}
		if c.isPageEntry(entryName) {
			mark.Type = model.BlockContentTextMark_Mention
			mark.Param = entryName
		}
	}
	return block, nil
}

// convertWholeLineLink turns a line that is a single link into a page link,
// file block or bookmark. handled=false leaves the block to mark rewriting.
func (c *Converter) convertWholeLineLink(ctx context.Context, pageName string, block *model.Block, target string, sink importv2.Sink) (*model.Block, error, bool) {
	if anymark.IsUrl(target) {
		return &model.Block{
			Id: block.Id,
			Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{
				Url: target,
			}},
		}, nil, true
	}
	entryName, found := c.lookupEntry(target)
	if !found {
		return block, nil, false
	}
	if c.isPageEntry(entryName) {
		return &model.Block{
			Id: block.Id,
			Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
				TargetBlockId: entryName,
				Style:         model.BlockContentLink_Page,
			}},
		}, nil, true
	}
	if err := c.emitFileObject(ctx, entryName, sink); err != nil {
		return nil, err, true
	}
	fileContent := anymark.ConvertTextToFile(entryName)
	fileContent.File.Name = ""
	fileContent.File.TargetObjectId = entryName
	return &model.Block{Id: block.Id, Content: fileContent}, nil, true
}

// emitFileObject streams the file object backing a reference, once per file.
func (c *Converter) emitFileObject(ctx context.Context, entryName string, sink importv2.Sink) error {
	if c.emittedFiles[entryName] {
		return nil
	}
	c.emittedFiles[entryName] = true
	entry, _ := c.source.Stat(entryName)
	object := &importv2.Object{
		SourceKey: entryName,
		SbType:    coresb.SmartBlockTypeFileObject,
		Payload:   &importv2.Snapshot{Details: domain.NewDetails()},
		File: &importv2.FileSource{
			Name: path.Base(entryName),
			Open: c.openEntry(entryName),
		},
	}
	if entry.FSPath != "" {
		object.File.Path = entry.FSPath
		object.File.Open = nil
	}
	return sink.Object(ctx, object)
}

// isPageEntry reports whether an entry converts to an object of its own
// (markdown page, or csv collection under a csv-collections profile) rather
// than a file object. Must agree with what Convert actually emits, or link
// rewriting produces dangling references.
func (c *Converter) isPageEntry(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".md":
		return true
	case ".csv":
		return c.flavour.CSVCollections
	default:
		return false
	}
}

// isWholeLine reports whether the mark spans the entire text in UTF-16
// units (the mark range convention).
func isWholeLine(text string, mark *model.BlockContentTextMark) bool {
	if mark.Range == nil {
		return false
	}
	return mark.Range.From == 0 && int(mark.Range.To) >= textutil.UTF16RuneCountString(text)
}
