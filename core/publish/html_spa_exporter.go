package publish

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	utf16 "github.com/anyproto/anytype-heart/util/text"
)

type SingleFileHtmlBuilder struct{}

func NewSingleFileHtmlBuilder() *SingleFileHtmlBuilder {
	return &SingleFileHtmlBuilder{}
}

type pageInfo struct {
	id       string
	title    string
	isRoot   bool
	snapshot *model.SmartBlockSnapshotBase
	sbType   model.SmartBlockType
}

func (b *SingleFileHtmlBuilder) BuildSPA(exportPath string, rootPageID string, inviteLink string) ([]byte, error) {
	objectsDir := filepath.Join(exportPath, "objects")
	dirEntries, err := os.ReadDir(objectsDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read objects dir: %w", err)
	}

	pages := make(map[string]*pageInfo)
	knownPages := make(map[string]bool)

	for _, entry := range dirEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pb") {
			continue
		}
		objID := strings.TrimSuffix(entry.Name(), ".pb")
		data, err := os.ReadFile(filepath.Join(objectsDir, entry.Name()))
		if err != nil {
			continue
		}

		snapshotWithType := pb.SnapshotWithType{}
		if err := proto.Unmarshal(data, &snapshotWithType); err != nil {
			continue
		}

		snap := snapshotWithType.GetSnapshot().GetData()
		if snap == nil {
			continue
		}

		sbType := snapshotWithType.GetSbType()
		// Process pages, profile pages, widgets, home, workspaces, etc.
		if sbType == model.SmartBlockType_Page ||
			sbType == model.SmartBlockType_ProfilePage ||
			sbType == model.SmartBlockType_Home ||
			sbType == model.SmartBlockType_Workspace ||
			objID == rootPageID {

			title := getPageTitle(snap, objID)
			pages[objID] = &pageInfo{
				id:       objID,
				title:    title,
				isRoot:   (objID == rootPageID),
				snapshot: snap,
				sbType:   sbType,
			}
			knownPages[objID] = true
		}
	}

	// Ensure root page is in knownPages even if sbType differed
	if rootInfo, ok := pages[rootPageID]; ok {
		rootInfo.isRoot = true
	} else if len(pages) == 0 {
		// Fallback empty root page
		pages[rootPageID] = &pageInfo{
			id:     rootPageID,
			title:  "Home",
			isRoot: true,
		}
		knownPages[rootPageID] = true
	}

	// Map of available file attachments in files/
	filesDir := filepath.Join(exportPath, "files")
	filesMap := make(map[string]string) // filename -> full path
	if entries, err := os.ReadDir(filesDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				filesMap[e.Name()] = filepath.Join(filesDir, e.Name())
			}
		}
	}

	var buf bytes.Buffer
	buf.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	buf.WriteString("  <meta charset=\"UTF-8\">\n")
	buf.WriteString("  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")

	rootTitle := "Published Channel"
	if rootInfo, ok := pages[rootPageID]; ok && rootInfo.title != "" {
		rootTitle = rootInfo.title
	}
	fmt.Fprintf(&buf, "  <title>%s</title>\n", html.EscapeString(rootTitle))
	buf.WriteString(getSPAStyles())
	buf.WriteString("</head>\n<body>\n")
	buf.WriteString("  <div class=\"top-bar\">\n")
	buf.WriteString("    <button class=\"theme-toggle\" onclick=\"toggleTheme()\">🌗 Theme</button>\n")
	buf.WriteString("  </div>\n")

	if inviteLink != "" {
		fmt.Fprintf(&buf, "  <div class=\"invite-banner\"><a href=\"%s\" target=\"_blank\">Join this Space</a></div>\n", html.EscapeString(inviteLink))
	}

	buf.WriteString("  <div class=\"container\">\n")

	// Sort page IDs for deterministic HTML generation
	pageIDs := make([]string, 0, len(pages))
	for id := range pages {
		pageIDs = append(pageIDs, id)
	}
	sort.Slice(pageIDs, func(i, j int) bool {
		if pageIDs[i] == rootPageID {
			return true
		}
		if pageIDs[j] == rootPageID {
			return false
		}
		return pageIDs[i] < pageIDs[j]
	})

	renderer := &pageRenderer{
		knownPages: knownPages,
		filesMap:   filesMap,
		filesDir:   filesDir,
	}

	for _, id := range pageIDs {
		p := pages[id]
		activeClass := ""
		dataPath := fmt.Sprintf("/%s", p.id)
		if p.isRoot {
			activeClass = " active"
			dataPath = "/"
		}

		fmt.Fprintf(&buf, "    <div id=\"route-%s\" data-path=\"%s\" class=\"page%s\">\n", p.id, dataPath, activeClass)
		if !p.isRoot {
			fmt.Fprintf(&buf, "      <div class=\"breadcrumb\"><a href=\"/\" onclick=\"navigate(event, '/')\">← Back to Home</a></div>\n")
		}
		fmt.Fprintf(&buf, "      <h1 class=\"page-title\">%s</h1>\n", html.EscapeString(p.title))

		if p.snapshot != nil {
			buf.WriteString(renderer.renderBlocks(p.snapshot))
		}
		buf.WriteString("    </div>\n")
	}

	buf.WriteString("  </div>\n")
	buf.WriteString(getSPAScript(rootPageID))
	buf.WriteString("</body>\n</html>")

	return buf.Bytes(), nil
}

type pageRenderer struct {
	knownPages map[string]bool
	filesMap   map[string]string
	filesDir   string
}

func (r *pageRenderer) renderBlocks(snap *model.SmartBlockSnapshotBase) string {
	if len(snap.Blocks) == 0 {
		return ""
	}

	blockMap := make(map[string]*model.Block)
	for _, b := range snap.Blocks {
		if b != nil {
			blockMap[b.Id] = b
		}
	}

	// Find root block
	var rootBlock *model.Block
	for _, b := range snap.Blocks {
		if b == nil {
			continue
		}
		if _, ok := b.Content.(*model.BlockContentOfSmartblock); ok {
			rootBlock = b
			break
		}
	}

	var buf bytes.Buffer
	if rootBlock != nil && len(rootBlock.ChildrenIds) > 0 {
		for _, childID := range rootBlock.ChildrenIds {
			if child, ok := blockMap[childID]; ok {
				r.renderBlock(&buf, child, blockMap)
			}
		}
	} else {
		for _, b := range snap.Blocks {
			if b != nil && b != rootBlock {
				r.renderBlock(&buf, b, blockMap)
			}
		}
	}

	return buf.String()
}

func (r *pageRenderer) renderBlock(buf *bytes.Buffer, b *model.Block, blockMap map[string]*model.Block) {
	if b == nil {
		return
	}

	switch content := b.Content.(type) {
	case *model.BlockContentOfText:
		r.renderTextBlock(buf, b, content.Text, blockMap)
	case *model.BlockContentOfFile:
		r.renderFileBlock(buf, content.File)
	case *model.BlockContentOfBookmark:
		r.renderBookmarkBlock(buf, content.Bookmark)
	case *model.BlockContentOfDiv:
		buf.WriteString("      <hr class=\"divider\" />\n")
	case *model.BlockContentOfLayout:
		r.renderLayoutBlock(buf, b, content.Layout, blockMap)
	case *model.BlockContentOfTable:
		r.renderTableBlock(buf, b, blockMap)
	default:
		r.renderChildren(buf, b, blockMap)
	}
}

func (r *pageRenderer) renderChildren(buf *bytes.Buffer, b *model.Block, blockMap map[string]*model.Block) {
	for _, childID := range b.ChildrenIds {
		if child, ok := blockMap[childID]; ok {
			r.renderBlock(buf, child, blockMap)
		}
	}
}

func (r *pageRenderer) renderTextBlock(buf *bytes.Buffer, b *model.Block, text *model.BlockContentText, blockMap map[string]*model.Block) {
	if text == nil {
		r.renderChildren(buf, b, blockMap)
		return
	}

	formattedText := r.formatTextWithMarks(text)

	switch text.Style {
	case model.BlockContentText_Header1:
		fmt.Fprintf(buf, "      <h2>%s</h2>\n", formattedText)
	case model.BlockContentText_Header2:
		fmt.Fprintf(buf, "      <h3>%s</h3>\n", formattedText)
	case model.BlockContentText_Header3:
		fmt.Fprintf(buf, "      <h4>%s</h4>\n", formattedText)
	case model.BlockContentText_Marked:
		fmt.Fprintf(buf, "      <ul><li>%s", formattedText)
		r.renderChildren(buf, b, blockMap)
		buf.WriteString("</li></ul>\n")
		return
	case model.BlockContentText_Numbered:
		fmt.Fprintf(buf, "      <ol><li>%s", formattedText)
		r.renderChildren(buf, b, blockMap)
		buf.WriteString("</li></ol>\n")
		return
	case model.BlockContentText_Callout:
		icon := ""
		if text.IconEmoji != "" {
			icon = fmt.Sprintf("<span class=\"callout-icon\">%s</span> ", html.EscapeString(text.IconEmoji))
		}
		fmt.Fprintf(buf, "      <div class=\"callout\">%s%s", icon, formattedText)
		r.renderChildren(buf, b, blockMap)
		buf.WriteString("</div>\n")
		return
	case model.BlockContentText_Quote:
		fmt.Fprintf(buf, "      <blockquote>%s", formattedText)
		r.renderChildren(buf, b, blockMap)
		buf.WriteString("</blockquote>\n")
		return
	case model.BlockContentText_Code:
		fmt.Fprintf(buf, "      <pre><code>%s</code></pre>\n", formattedText)
	default:
		if formattedText != "" {
			fmt.Fprintf(buf, "      <p>%s</p>\n", formattedText)
		}
	}

	r.renderChildren(buf, b, blockMap)
}

func (r *pageRenderer) formatTextWithMarks(text *model.BlockContentText) string {
	if text == nil || text.Text == "" {
		return ""
	}

	textRunes := []rune(text.Text)
	textLen := utf16.UTF16RuneCountString(text.Text)

	if text.Marks == nil || len(text.Marks.Marks) == 0 {
		return html.EscapeString(text.Text)
	}

	type markEvent struct {
		isStart bool
		mark    *model.BlockContentTextMark
	}

	events := make(map[int][]markEvent)
	for _, m := range text.Marks.Marks {
		if m == nil {
			continue
		}
		from := int(m.Range.From)
		to := int(m.Range.To)
		if from < 0 {
			from = 0
		}
		if to > textLen {
			to = textLen
		}
		if from >= to {
			continue
		}
		events[from] = append(events[from], markEvent{isStart: true, mark: m})
		events[to] = append(events[to], markEvent{isStart: false, mark: m})
	}

	var out bytes.Buffer
	for i := 0; i <= textLen; i++ {
		if evs, ok := events[i]; ok {
			// Close marks first
			for _, ev := range evs {
				if !ev.isStart {
					r.renderMarkTag(&out, ev.mark, false)
				}
			}
			// Open marks second
			for _, ev := range evs {
				if ev.isStart {
					r.renderMarkTag(&out, ev.mark, true)
				}
			}
		}
		if i < len(textRunes) {
			out.WriteString(html.EscapeString(string(textRunes[i])))
		}
	}

	return out.String()
}

func (r *pageRenderer) renderMarkTag(out *bytes.Buffer, m *model.BlockContentTextMark, isStart bool) {
	switch m.Type {
	case model.BlockContentTextMark_Bold:
		if isStart {
			out.WriteString("<b>")
		} else {
			out.WriteString("</b>")
		}
	case model.BlockContentTextMark_Italic:
		if isStart {
			out.WriteString("<i>")
		} else {
			out.WriteString("</i>")
		}
	case model.BlockContentTextMark_Strikethrough:
		if isStart {
			out.WriteString("<s>")
		} else {
			out.WriteString("</s>")
		}
	case model.BlockContentTextMark_Underscored:
		if isStart {
			out.WriteString("<u>")
		} else {
			out.WriteString("</u>")
		}
	case model.BlockContentTextMark_Keyboard:
		if isStart {
			out.WriteString("<kbd>")
		} else {
			out.WriteString("</kbd>")
		}
	case model.BlockContentTextMark_Link:
		if isStart {
			targetID := r.extractObjectID(m.Param)
			if targetID != "" && r.knownPages[targetID] {
				fmt.Fprintf(out, "<a href=\"/%s\" onclick=\"navigate(event, '/%s')\">", targetID, targetID)
			} else {
				fmt.Fprintf(out, "<a href=\"%s\" target=\"_blank\" rel=\"noopener noreferrer\">", html.EscapeString(m.Param))
			}
		} else {
			out.WriteString("</a>")
		}
	case model.BlockContentTextMark_TextColor:
		if isStart {
			fmt.Fprintf(out, "<span style=\"color:%s\">", html.EscapeString(m.Param))
		} else {
			out.WriteString("</span>")
		}
	case model.BlockContentTextMark_BackgroundColor:
		if isStart {
			fmt.Fprintf(out, "<span style=\"background-color:%s\">", html.EscapeString(m.Param))
		} else {
			out.WriteString("</span>")
		}
	}
}

func (r *pageRenderer) extractObjectID(link string) string {
	clean := strings.TrimPrefix(link, "anytype://")
	parts := strings.Split(clean, "/")
	candidate := parts[len(parts)-1]
	if candidate != "" && r.knownPages[candidate] {
		return candidate
	}
	return ""
}

func (r *pageRenderer) renderFileBlock(buf *bytes.Buffer, file *model.BlockContentFile) {
	if file == nil {
		return
	}

	src := r.getFileSrc(file)

	switch file.Type {
	case model.BlockContentFile_Image:
		fmt.Fprintf(buf, "      <div class=\"media-container\"><img src=\"%s\" alt=\"%s\" class=\"media-img\" /></div>\n", html.EscapeString(src), html.EscapeString(file.Name))
	case model.BlockContentFile_Video:
		fmt.Fprintf(buf, "      <div class=\"media-container\"><video controls src=\"%s\" class=\"media-video\"></video></div>\n", html.EscapeString(src))
	case model.BlockContentFile_Audio:
		fmt.Fprintf(buf, "      <div class=\"media-container\"><audio controls src=\"%s\" class=\"media-audio\"></audio></div>\n", html.EscapeString(src))
	default:
		fmt.Fprintf(buf, "      <div class=\"file-card\"><a href=\"%s\" download><span class=\"icon\">📄</span> %s</a></div>\n", html.EscapeString(src), html.EscapeString(file.Name))
	}
}

func (r *pageRenderer) getFileSrc(file *model.BlockContentFile) string {
	if file == nil {
		return "#"
	}
	// Try matching by targetObjectId or name in filesMap
	for fname, fullPath := range r.filesMap {
		if strings.Contains(fname, file.TargetObjectId) || fname == file.Name {
			// Check if small image for base64 inline embedding (< 32KB)
			if file.Type == model.BlockContentFile_Image {
				if info, err := os.Stat(fullPath); err == nil && info.Size() < 32*1024 {
					if data, err := os.ReadFile(fullPath); err == nil {
						mime := "image/png"
						if strings.HasSuffix(fname, ".jpg") || strings.HasSuffix(fname, ".jpeg") {
							mime = "image/jpeg"
						} else if strings.HasSuffix(fname, ".svg") {
							mime = "image/svg+xml"
						} else if strings.HasSuffix(fname, ".webp") {
							mime = "image/webp"
						}
						return fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))
					}
				}
			}
			return fmt.Sprintf("files/%s", fname)
		}
	}

	if file.Name != "" {
		return fmt.Sprintf("files/%s", file.Name)
	}
	if file.TargetObjectId != "" {
		return fmt.Sprintf("files/%s", file.TargetObjectId)
	}
	return "#"
}

func (r *pageRenderer) renderBookmarkBlock(buf *bytes.Buffer, bm *model.BlockContentBookmark) {
	if bm == nil {
		return
	}
	title := bm.Title
	if title == "" {
		title = bm.Url
	}
	fmt.Fprintf(buf, "      <div class=\"bookmark-card\"><a href=\"%s\" target=\"_blank\" rel=\"noopener noreferrer\"><strong>%s</strong></a>", html.EscapeString(bm.Url), html.EscapeString(title))
	if bm.Description != "" {
		fmt.Fprintf(buf, "<p>%s</p>", html.EscapeString(bm.Description))
	}
	buf.WriteString("</div>\n")
}

func (r *pageRenderer) renderLayoutBlock(buf *bytes.Buffer, b *model.Block, layout *model.BlockContentLayout, blockMap map[string]*model.Block) {
	style := model.BlockContentLayoutStyle(-1)
	if layout != nil {
		style = layout.Style
	}

	switch style {
	case model.BlockContentLayout_Column:
		buf.WriteString("      <div class=\"column\">\n")
		r.renderChildren(buf, b, blockMap)
		buf.WriteString("      </div>\n")
	case model.BlockContentLayout_Row:
		buf.WriteString("      <div class=\"row\">\n")
		r.renderChildren(buf, b, blockMap)
		buf.WriteString("      </div>\n")
	default:
		r.renderChildren(buf, b, blockMap)
	}
}

func (r *pageRenderer) renderTableBlock(buf *bytes.Buffer, b *model.Block, blockMap map[string]*model.Block) {
	buf.WriteString("      <table class=\"table\">\n")
	for _, rowID := range b.ChildrenIds {
		if rowBlock, ok := blockMap[rowID]; ok {
			buf.WriteString("        <tr>\n")
			for _, cellID := range rowBlock.ChildrenIds {
				if cellBlock, ok := blockMap[cellID]; ok {
					buf.WriteString("          <td>")
					r.renderChildren(buf, cellBlock, blockMap)
					buf.WriteString("</td>\n")
				}
			}
			buf.WriteString("        </tr>\n")
		}
	}
	buf.WriteString("      </table>\n")
}

func getPageTitle(snap *model.SmartBlockSnapshotBase, defaultID string) string {
	if snap == nil {
		return defaultID
	}
	if snap.Details != nil && snap.Details.Fields != nil {
		if val, ok := snap.Details.Fields["name"]; ok && val.GetStringValue() != "" {
			return val.GetStringValue()
		}
	}
	for _, b := range snap.Blocks {
		if b == nil {
			continue
		}
		if textContent, ok := b.Content.(*model.BlockContentOfText); ok && textContent.Text != nil {
			if textContent.Text.Text != "" {
				return textContent.Text.Text
			}
		}
	}
	return defaultID
}

func getSPAStyles() string {
	return `  <style>
    :root {
      --bg: #ffffff;
      --fg: #1c1c1e;
      --card-bg: #f2f2f7;
      --border: #e5e5ea;
      --link: #007aff;
      --callout-bg: #eef6ff;
    }
    body.dark-mode {
      --bg: #1c1c1e;
      --fg: #f2f2f7;
      --card-bg: #2c2c2e;
      --border: #3a3a3c;
      --link: #0a84ff;
      --callout-bg: #1e293b;
    }
    body.light-mode {
      --bg: #ffffff;
      --fg: #1c1c1e;
      --card-bg: #f2f2f7;
      --border: #e5e5ea;
      --link: #007aff;
      --callout-bg: #eef6ff;
    }
    body {
      background-color: var(--bg);
      color: var(--fg);
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      margin: 0;
      padding: 0;
      line-height: 1.6;
    }
    .top-bar {
      display: flex;
      justify-content: flex-end;
      padding: 12px 24px;
      border-bottom: 1px solid var(--border);
    }
    .theme-toggle {
      background: var(--card-bg);
      color: var(--fg);
      border: 1px solid var(--border);
      padding: 6px 14px;
      border-radius: 20px;
      cursor: pointer;
      font-size: 14px;
    }
    .invite-banner {
      background: var(--link);
      color: white;
      text-align: center;
      padding: 8px;
    }
    .invite-banner a {
      color: white;
      font-weight: bold;
      text-decoration: underline;
    }
    .container {
      max-width: 800px;
      margin: 30px auto;
      padding: 0 20px;
    }
    .page {
      display: none;
    }
    .page.active {
      display: block;
    }
    .breadcrumb {
      margin-bottom: 16px;
    }
    .breadcrumb a {
      color: var(--link);
      text-decoration: none;
      font-size: 14px;
    }
    .page-title {
      font-size: 32px;
      margin-bottom: 24px;
    }
    a {
      color: var(--link);
      text-decoration: none;
    }
    a:hover {
      text-decoration: underline;
    }
    .callout {
      background: var(--callout-bg);
      border-left: 4px solid var(--link);
      padding: 12px 16px;
      margin: 16px 0;
      border-radius: 4px;
    }
    .callout-icon {
      font-size: 18px;
    }
    .bookmark-card, .file-card {
      background: var(--card-bg);
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 12px 16px;
      margin: 12px 0;
    }
    .media-container {
      margin: 16px 0;
    }
    .media-img, .media-video {
      max-width: 100%;
      height: auto;
      border-radius: 8px;
    }
    .row {
      display: flex;
      gap: 16px;
    }
    .column {
      flex: 1;
    }
    .table {
      width: 100%;
      border-collapse: collapse;
      margin: 16px 0;
    }
    .table td {
      border: 1px solid var(--border);
      padding: 8px 12px;
    }
    pre {
      background: var(--card-bg);
      padding: 12px;
      border-radius: 6px;
      overflow-x: auto;
    }
  </style>
`
}

func getSPAScript(rootPageID string) string {
	return `  <script>
    function navigate(e, path) {
      if (e) e.preventDefault();
      window.history.pushState({}, '', path);
      renderRoute(path);
    }
    function renderRoute(path) {
      var cleanPath = path || window.location.pathname;
      if (cleanPath.endsWith('/') && cleanPath.length > 1) {
        cleanPath = cleanPath.substring(0, cleanPath.length - 1);
      }
      var pages = document.querySelectorAll('.page');
      var matched = false;
      pages.forEach(function(p) {
        var pagePath = p.getAttribute('data-path');
        if (pagePath === cleanPath || (cleanPath === '' && pagePath === '/') || (cleanPath === '/' && pagePath === '/')) {
          p.classList.add('active');
          matched = true;
        } else {
          p.classList.remove('active');
        }
      });
      if (!matched && pages.length > 0) {
        var root = document.querySelector('.page[data-path="/"]') || pages[0];
        if (root) root.classList.add('active');
      }
      window.scrollTo(0, 0);
    }
    window.addEventListener('popstate', function() {
      renderRoute(window.location.pathname);
    });
    document.addEventListener('DOMContentLoaded', function() {
      renderRoute(window.location.pathname);
    });
    function toggleTheme() {
      var body = document.body;
      if (body.classList.contains('dark-mode')) {
        body.classList.remove('dark-mode');
        body.classList.add('light-mode');
      } else {
        body.classList.remove('light-mode');
        body.classList.add('dark-mode');
      }
    }
  </script>
`
}
