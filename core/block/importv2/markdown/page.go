package markdown

import (
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/common/filetime"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
	"github.com/anyproto/anytype-heart/pkg/lib/schema/yaml"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	textutil "github.com/anyproto/anytype-heart/util/text"
)

// convertPage converts one markdown file and emits, in order: new property
// definitions, referenced file objects, then the page. Nothing about the
// page is retained afterwards.
func (c *Converter) convertPage(ctx context.Context, entry source.Entry, sink importv2.Sink) error {
	// The currentItem. The entry name is a path inside the user's own
	// tree — user content, so it travels as a DisplayText and reaches the
	// wire but never a log line.
	sink.Item(importv2.DisplayText(entry.Name))
	content, err := c.readEntry(ctx, entry.Name)
	if err != nil {
		sink.Issue(importv2.Issue{
			Severity: importv2.SeverityObjectError, Code: importv2.IssueObjectFailed, SourceKey: entry.Name,
			Message: "This file could not be read", Err: err,
		})
		return c.emitPlaceholderPage(ctx, entry, sink)
	}

	frontMatter, body, err := yaml.ExtractYAMLFrontMatter(content)
	if err != nil {
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, entry.Name,
			"The front matter of this file could not be read; it was imported as plain text"))
		body = content
	}
	var yamlDetails []domain.Detail
	var yamlLinks []*model.RelationLink
	var collectionRefs []string
	isCollection := false
	typeKey := bundle.TypeKeyPage.String()
	if len(frontMatter) > 0 {
		redirects := c.planRedirectsFor(path.Dir(entry.Name))
		c.resolver.setPlanRedirects(redirects)
		parsed, err := yaml.ParseYAMLFrontMatterWithResolverAndPath(frontMatter, c.resolver, path.Dir(entry.Name))
		c.resolver.setPlanRedirects(nil)
		if err != nil {
			sink.Issue(importv2.Issue{
				Severity: importv2.SeverityWarning, Code: importv2.IssueDataLoss, SourceKey: entry.Name,
				Message: "The front matter of this file could not be read; its properties were skipped", Err: err,
			})
		} else if parsed != nil {
			collectionRefs, parsed.Properties, isCollection = c.extractCollectionProperty(parsed.Properties)
			// The yaml parser yields properties in map order; sort BEFORE the
			// redirects so their propertyMapped issues are deterministic
			// (contract rule 5), and re-sort after — redirects rename.
			sort.SliceStable(parsed.Properties, func(i, j int) bool {
				return parsed.Properties[i].Name < parsed.Properties[j].Name
			})
			parsed.Properties = c.applyPlanRedirects(path.Dir(entry.Name), parsed.Properties, redirects, sink)
			sort.SliceStable(parsed.Properties, func(i, j int) bool {
				return parsed.Properties[i].Name < parsed.Properties[j].Name
			})
			yamlDetails, yamlLinks, typeKey, err = c.emitPropertyDefinitions(ctx, parsed.Properties, parsed.ObjectType, sink)
			if err != nil {
				return err
			}
		}
	}
	// A collection-title suggestion fills the default-Page gap only — an
	// explicit front-matter type always wins.
	if typeKey == bundle.TypeKeyPage.String() {
		if suggested, ok := c.suggestedDirTypes[path.Dir(entry.Name)]; ok {
			typeKey = suggested.String()
		}
	}

	blocks, _, err := anymark.MarkdownToBlocks(body, path.Dir(entry.Name), nil, c.flavour.Anymark...)
	if err != nil {
		sink.Issue(importv2.Issue{
			Severity: importv2.SeverityObjectError, Code: importv2.IssueObjectFailed, SourceKey: entry.Name,
			Message: "This markdown file could not be parsed", Err: err,
		})
		return c.emitPlaceholderPage(ctx, entry, sink)
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
	if isCollection {
		if members := c.resolveCollectionMembers(entry.Name, collectionRefs, sink); len(members) > 0 {
			object.Payload.Collections = collectionStore(members)
		}
	}
	return sink.Object(ctx, object)
}

// extractCollectionProperty pulls the collection-membership pseudo-property
// out of the front matter: the `_collection` key (Anytype's export
// convention, honored under every profile) or — with CollectionByName — a
// property named "Collection". It becomes the collection store, never a
// visible relation.
func (c *Converter) extractCollectionProperty(properties []yaml.Property) (refs []string, rest []yaml.Property, found bool) {
	for i, property := range properties {
		if property.Key != schema.CollectionPropertyKey &&
			!(c.flavour.CollectionByName && strings.EqualFold(property.Name, "Collection")) {
			continue
		}
		return property.Value.WrapToStringList(), append(properties[:i:i], properties[i+1:]...), true
	}
	return nil, properties, false
}

// resolveCollectionMembers maps front-matter member references to page
// source keys: `.md` appended when missing, the generic lookup chain, then
// relative to the collection page's directory (v1's matching strategies).
func (c *Converter) resolveCollectionMembers(pageName string, refs []string, sink importv2.Sink) []string {
	members := make([]string, 0, len(refs))
	for _, ref := range refs {
		refPath := ref
		if !strings.HasSuffix(strings.ToLower(refPath), ".md") {
			refPath += ".md"
		}
		if entryName, ok := c.lookupEntry(refPath); ok && c.isPageEntry(entryName) {
			members = append(members, entryName)
			continue
		}
		if entryName, ok := c.lookupEntry(path.Join(path.Dir(pageName), refPath)); ok && c.isPageEntry(entryName) {
			members = append(members, entryName)
			continue
		}
		sink.Issue(importv2.Warning(importv2.IssueMissingTarget, pageName,
			"A collection listed a file that is not part of this import").About(ref))
	}
	return members
}

func collectionStore(members []string) *types.Struct {
	return &types.Struct{Fields: map[string]*types.Value{
		template.CollectionStoreKey: pbtypes.StringList(members),
	}}
}

// splitEmojiTitle extracts a leading emoji from an H1 title into the page
// icon. The v1 rule: the icon is the whole first space-delimited token, so
// multi-code-point emoji (flags, ZWJ families, skin tones) survive intact;
// an emoji-only title stays the name — a page must not lose its title to
// its icon.
func splitEmojiTitle(title string) (emoji, rest string) {
	first, remainder, found := strings.Cut(strings.TrimSpace(title), " ")
	if !found {
		return "", title
	}
	runes := []rune(first)
	if len(runes) == 0 || !isEmojiRune(runes[0]) {
		return "", title
	}
	return first, strings.TrimSpace(remainder)
}

// isEmojiRune approximates emoji detection, as v1 did: 2194–329F covers the
// BMP symbol blocks (arrows, clocks ⏰, stars ⭐, enclosed CJK), 1F000–1FAFF
// the emoji planes (extended past v1's 1FADF for the Unicode 15 additions).
func isEmojiRune(r rune) bool {
	return (r >= 0x2194 && r <= 0x329F) || (r >= 0x1F000 && r <= 0x1FAFF)
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
	bundle.RelationKeyCoverType.String():        {},
	bundle.RelationKeyCoverX.String():           {},
	bundle.RelationKeyCoverY.String():           {},
	bundle.RelationKeyCoverScale.String():       {},
	bundle.RelationKeyLayout.String():           {},
	bundle.RelationKeyLayoutAlign.String():      {},
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

// emitPlaceholderPage keeps a claimed identity real when its content is
// unreadable: pass-1 minted an id that other pages may already reference, so
// an empty page (filename title) must exist — v1 behavior. The accompanying
// ObjectError still aborts all-or-nothing runs.
func (c *Converter) emitPlaceholderPage(ctx context.Context, entry source.Entry, sink importv2.Sink) error {
	object := &importv2.Object{
		SourceKey: entry.Name,
		SbType:    coresb.SmartBlockTypePage,
		Payload: &importv2.Snapshot{
			Details:     domain.NewDetails(),
			ObjectTypes: []string{bundle.TypeKeyPage.String()},
		},
		IsRootCandidate: c.dirs == nil && isTopLevel(entry.Name),
	}
	c.stampCommonDetails(object, entry, pageTitleFromPath(entry.Name))
	return sink.Object(ctx, object)
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
			"A file referenced by this page is not in the source").About(file.Name))
		file.Name = ""
		return nil
	}
	if c.isPageEntry(entryName) {
		// An image/file embed of a page or csv collection: emitting a file
		// object here would collide with the page's identity under the same
		// source key. Link to the page instead (v1's csv rule).
		block.Content = &model.BlockContentOfLink{Link: &model.BlockContentLink{
			TargetBlockId: entryName,
			Style:         model.BlockContentLink_Page,
		}}
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
		if !c.isPageEntry(entryName) {
			// An inline link to a local file: import the file and mention
			// it — keeps the sentence intact (v1 replaced the whole block
			// with a file block, losing the text).
			if err := c.emitFileObject(ctx, entryName, sink); err != nil {
				return nil, err
			}
		}
		mark.Type = model.BlockContentTextMark_Mention
		mark.Param = entryName
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
