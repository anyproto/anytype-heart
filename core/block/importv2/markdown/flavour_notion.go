package markdown

import (
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	textutil "github.com/anyproto/anytype-heart/util/text"
)

// notionIdToken matches a bare Notion export id (the 32-hex suffix Notion
// appends to every exported file name).
var notionIdToken = regexp.MustCompile(`^[0-9a-f]{32}$`)

// notionIdOf extracts the trailing Notion id from a file base name
// ("Title <32-hex>.ext").
func notionIdOf(base string) (string, bool) {
	name := strings.TrimSuffix(base, path.Ext(base))
	idx := strings.LastIndex(name, " ")
	if idx < 0 || !notionIdToken.MatchString(name[idx+1:]) {
		return "", false
	}
	return name[idx+1:], true
}

// notionPageTitle derives a display title from an entry name with the
// Notion id suffix stripped ("My Page 0123….md" → "My Page").
func notionPageTitle(entryName string) string {
	title := pageTitleFromPath(entryName)
	if idx := strings.LastIndex(title, " "); idx > 0 && notionIdToken.MatchString(title[idx+1:]) {
		return title[:idx]
	}
	return title
}

// notionResolveTarget is the notion-export ResolveTarget hook: a link whose
// path missed every generic strategy still resolves when exactly one page
// carries the same trailing Notion id — export links survive directory
// moves and renames that way (v1's field-block id matching, generalized).
func notionResolveTarget(c *Converter, target string) (string, bool) {
	id, ok := notionIdOf(path.Base(target))
	if !ok {
		return "", false
	}
	match := ""
	for _, entry := range c.mdEntries {
		candidate, ok := notionIdOf(path.Base(entry.Name))
		if !ok || candidate != id {
			continue
		}
		if match != "" {
			return "", false // ambiguous: two pages share the id
		}
		match = entry.Name
	}
	return match, match != ""
}

// notionExtractMetadata is the notion-export ExtractMetadata hook. Notion's
// markdown exports carry database-row properties as plain first-paragraph
// lines ("Related: Other%20Page%20<id>.md"); v1 rewrote the page references
// into mentions (processFieldBlockIfItIs) and v2 dropped them entirely. The
// hook rewrites each resolvable reference to the target's display title
// with a Mention mark (UTF-16 ranges — v1 measured bytes).
func notionExtractMetadata(c *Converter, page *pageContext) {
	if len(page.Blocks) == 0 {
		return
	}
	text := page.Blocks[0].GetText()
	if text == nil || text.Text == "" {
		return
	}
	if len(text.GetMarks().GetMarks()) > 0 {
		return // property lines never carry markup (v1 rule)
	}
	rewritten, marks := c.rewriteNotionPropertyLines(text.Text)
	if len(marks) == 0 {
		return
	}
	text.Text = rewritten
	text.Marks = &model.BlockContentTextMarks{Marks: marks}
}

// rewriteNotionPropertyLines rewrites "Key: ref.md[, ref.md…]" lines,
// leaving every other line verbatim.
func (c *Converter) rewriteNotionPropertyLines(raw string) (string, []*model.BlockContentTextMark) {
	var out strings.Builder
	var marks []*model.BlockContentTextMark
	for i, line := range strings.Split(raw, "\n") {
		if i > 0 {
			out.WriteString("\n")
		}
		key, refs, ok := splitNotionPropertyLine(line)
		if !ok {
			out.WriteString(line)
			continue
		}
		out.WriteString(key)
		out.WriteString(": ")
		for j, ref := range refs {
			if j > 0 {
				out.WriteString(", ")
			}
			entryName, found := c.lookupNotionRef(ref)
			if !found {
				out.WriteString(strings.TrimSpace(ref))
				continue
			}
			title := notionPageTitle(entryName)
			from := textutil.UTF16RuneCountString(out.String())
			out.WriteString(title)
			marks = append(marks, &model.BlockContentTextMark{
				Range: &model.Range{From: int32(from), To: int32(from + textutil.UTF16RuneCountString(title))},
				Type:  model.BlockContentTextMark_Mention,
				Param: entryName,
			})
		}
	}
	return out.String(), marks
}

// splitNotionPropertyLine recognizes a property line by v1's shape: it must
// end in ".md" and contain a "Key: value" split; values are comma-separated.
func splitNotionPropertyLine(line string) (key string, refs []string, ok bool) {
	if !strings.HasSuffix(line, ".md") {
		return "", nil, false
	}
	key, values, found := strings.Cut(line, ":")
	if !found || strings.TrimSpace(values) == "" {
		return "", nil, false
	}
	return key, strings.Split(values, ","), true
}

// lookupNotionRef resolves one field-block reference: unescape and unquote,
// then the generic chain (which ends in the notion id fallback).
func (c *Converter) lookupNotionRef(ref string) (string, bool) {
	cleaned := strings.TrimSpace(strings.ReplaceAll(ref, `"`, ""))
	if unescaped, err := url.PathUnescape(cleaned); err == nil {
		cleaned = unescaped
	}
	if !strings.HasSuffix(strings.ToLower(cleaned), ".md") {
		return "", false
	}
	entryName, found := c.lookupEntry(cleaned)
	if !found || !c.isPageEntry(entryName) {
		return "", false
	}
	return entryName, true
}
