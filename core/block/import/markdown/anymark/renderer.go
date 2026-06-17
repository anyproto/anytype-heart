package anymark

import (
	"bytes"
	"html"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/gogo/protobuf/types"
	"github.com/yuin/goldmark/ast"
	ext "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/wikilink"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/text"
)

var log = logging.Logger("anytype-anymark")

// BlockLengthSoftLimit is the soft limit for the length of a text block.
// In case text block length exceeds this limit and the soft line break found(e.g. single \n) the new text block will be started.
const TextBlockLengthSoftLimit = 1024

type Renderer struct {
	*blocksRenderer
}

// NewRenderer returns a new Renderer with given options.
func NewRenderer(br *blocksRenderer) *Renderer {
	return &Renderer{
		blocksRenderer: br,
	}
}

// RegisterFuncs implements NodeRenderer.RegisterFuncs .
func (r *Renderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	// blocks

	reg.Register(ast.KindDocument, r.renderDocument)
	reg.Register(ast.KindHeading, r.renderHeading)
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
	reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
	reg.Register(ast.KindList, r.renderList)
	reg.Register(ast.KindListItem, r.renderListItem)
	reg.Register(ast.KindParagraph, r.renderParagraph)
	reg.Register(ast.KindTextBlock, r.renderTextBlock)
	reg.Register(ast.KindThematicBreak, r.renderThematicBreak)

	// inlines

	reg.Register(ast.KindAutoLink, r.renderAutoLink)
	reg.Register(ast.KindCodeSpan, r.renderCodeSpan)
	reg.Register(ast.KindEmphasis, r.renderEmphasis)
	reg.Register(ast.KindImage, r.renderImage)
	reg.Register(ast.KindLink, r.renderLink)
	reg.Register(ast.KindRawHTML, r.renderRawHTML)
	reg.Register(ast.KindText, r.renderText)
	reg.Register(ast.KindString, r.renderString)
	reg.Register(ext.KindStrikethrough, r.renderStrikethrough)
	reg.Register(wikilink.Kind, r.renderWikiLink)
}

func (r *Renderer) writeLines(source []byte, n ast.Node) {
	l := n.Lines().Len()
	for i := 0; i < l; i++ {
		line := n.Lines().At(i)
		s := Unescape(string(line.Value(source)))
		r.AddTextToBuffer(s)
	}
}

func (r *Renderer) renderDocument(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	// nothing to do
	return ast.WalkContinue, nil
}

func (r *Renderer) renderHeading(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Heading)

	var style model.BlockContentTextStyle

	switch n.Level {
	case 1:
		style = model.BlockContentText_Header1
	case 2:
		style = model.BlockContentText_Header2
	case 3:
		style = model.BlockContentText_Header3
	case 4:
		style = model.BlockContentText_Header3
	case 5:
		style = model.BlockContentText_Header3
	case 6:
		style = model.BlockContentText_Header3
	}

	r.openTextBlockWithStyle(entering, style, nil)

	return ast.WalkContinue, nil
}

func (r *Renderer) renderBlockquote(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	r.openTextBlockWithStyle(entering, model.BlockContentText_Quote, nil)
	return ast.WalkContinue, nil
}

func (r *Renderer) renderCodeBlock(_ util.BufWriter,
	source []byte,
	n ast.Node,
	entering bool) (ast.WalkStatus, error) {
	r.openTextBlockWithStyle(entering, model.BlockContentText_Code, nil)
	if entering {
		r.writeLines(source, n)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderFencedCodeBlock(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.FencedCodeBlock)
	language := string(n.Language(source))
	var fields *types.Struct
	if language != "" {
		fields = &types.Struct{Fields: map[string]*types.Value{"lang": pbtypes.String(language)}}
	}
	if entering {
		r.openTextBlockWithStyle(entering, model.BlockContentText_Code, fields)
		r.writeLines(source, n)
	} else {
		r.openTextBlockWithStyle(entering, model.BlockContentText_Code, nil)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderHTMLBlock(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	raw := rawHTMLBlockText(source, node)

	// A self-contained <details>...</details> block (no blank line between the
	// <summary> and the children) is emitted by goldmark as a SINGLE HTMLBlock
	// containing the whole element. Parse it fully here: open a Toggle from the
	// <summary>, parse the inner content (between </summary> and the matching
	// </details>) as Markdown into child blocks, nest them, then close the Toggle.
	// Without this the block would be silently dropped (M3).
	if summary, inner, remainder, ok := parseSelfContainedDetails(raw); ok {
		r.OpenToggleBlock(summary)
		inner = strings.TrimSpace(inner)
		if inner != "" {
			children, _, err := MarkdownToBlocks([]byte(inner), r.GetBaseFilepath(), r.GetAllFileShortPaths())
			if err != nil {
				log.Errorf("failed to parse inner content of self-contained <details>: %v", err)
			} else {
				r.AppendChildBlocks(children)
			}
		}
		r.CloseToggleBlock()
		// goldmark's HTML block (type 6) ends at a blank line, NOT at the matched
		// </details>. So any content after the matched close lives in this SAME
		// HTMLBlock and must be emitted as SIBLINGS at this level (not children of
		// the toggle we just closed), otherwise it is silently lost (Major 1). The
		// remainder may itself contain another tight <details>, which is handled by
		// routing it back through the normal MarkdownToBlocks parse.
		if remainder = strings.TrimSpace(remainder); remainder != "" {
			siblings, _, err := MarkdownToBlocks([]byte(remainder), r.GetBaseFilepath(), r.GetAllFileShortPaths())
			if err != nil {
				log.Errorf("failed to parse trailing content after self-contained <details>: %v", err)
			} else {
				r.AppendSiblingBlocks(siblings)
			}
		}
		return ast.WalkContinue, nil
	}

	// A <details> element (with a <summary>) is the export encoding for a Toggle
	// block. We open a Toggle here so the blocks parsed between this opening
	// <details> and the closing </details> become its children, then close it on
	// </details>. This mirrors the md exporter's <details>/<summary> output.
	if summary, ok := parseDetailsSummary(raw); ok {
		r.OpenToggleBlock(summary)
		// goldmark folds a tight first line of children into the opener HTMLBlock
		// (e.g. "<details>\n<summary>S</summary>\nFirst\n"). That trailing content
		// after </summary> would otherwise be lost, so parse it as children here.
		// Subsequent loosely-separated blocks still nest via the open Toggle and
		// the closing </details> still closes it.
		if inner := strings.TrimSpace(detailsOpenerTrailingContent(raw)); inner != "" {
			children, _, err := MarkdownToBlocks([]byte(inner), r.GetBaseFilepath(), r.GetAllFileShortPaths())
			if err != nil {
				log.Errorf("failed to parse trailing content of <details> opener: %v", err)
			} else {
				r.AppendChildBlocks(children)
			}
		}
		return ast.WalkContinue, nil
	}
	if isDetailsClose(raw) {
		r.CloseToggleBlock()
		return ast.WalkContinue, nil
	}

	// Other HTML blocks are not rendered.
	return ast.WalkContinue, nil
}

func rawHTMLBlockText(source []byte, node ast.Node) string {
	var buf bytes.Buffer
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		buf.Write(line.Value(source))
	}
	return buf.String()
}

var (
	reDetailsOpen = regexp.MustCompile(`(?is)<details\b[^>]*>`)
	reSummary     = regexp.MustCompile(`(?is)<summary\b[^>]*>(.*?)</summary>`)
	reDetailsEnd  = regexp.MustCompile(`(?is)</details\s*>`)
)

// parseDetailsSummary reports whether raw opens a (not self-closed) <details>
// element, and if so returns the plain text of its <summary>, if present. A
// block that both opens and closes <details> is not treated as an opener,
// because there would be no following children to nest and no separate close.
func parseDetailsSummary(raw string) (string, bool) {
	if !reDetailsOpen.MatchString(raw) || reDetailsEnd.MatchString(raw) {
		return "", false
	}
	summary := ""
	if m := reSummary.FindStringSubmatch(raw); len(m) > 1 {
		// The exporter HTML-escapes the summary text, so the captured content is
		// the escaped plain text of the toggle title (e.g. "&lt;/details&gt;" for
		// a literal "</details>"). Decode HTML entities to recover the original
		// text exactly. We intentionally do NOT strip tag-shaped substrings here:
		// since the content is escaped, any '<'/'>' belong to the user's title and
		// stripping them would silently drop text.
		summary = strings.TrimSpace(html.UnescapeString(m[1]))
	}
	return summary, true
}

func isDetailsClose(raw string) bool {
	return reDetailsEnd.MatchString(raw) && !reDetailsOpen.MatchString(raw)
}

// parseSelfContainedDetails reports whether raw is a single self-contained
// <details>...</details> block (it both opens and closes <details>). If so it
// returns the decoded <summary> text, the inner content between the </summary>
// and the </details> that closes the OUTER <details> (depth-aware, so a nested
// <details> inside the content is kept intact for recursive parsing), and the
// remainder of the block AFTER that matched </details>. The remainder is
// non-empty when goldmark folded trailing content into the same HTML block (its
// block ends at a blank line, not at </details>); the caller emits it as
// siblings so it is not lost (Major 1).
func parseSelfContainedDetails(raw string) (summary string, inner string, remainder string, ok bool) {
	openLoc := reDetailsOpen.FindStringIndex(raw)
	if openLoc == nil || !reDetailsEnd.MatchString(raw) {
		return "", "", "", false
	}

	// Decode the (HTML-escaped) summary, mirroring parseDetailsSummary.
	if m := reSummary.FindStringSubmatch(raw); len(m) > 1 {
		summary = strings.TrimSpace(html.UnescapeString(m[1]))
	}

	// Locate the body start: just after the first </summary> if present,
	// otherwise just after the opening <details ...> tag.
	bodyStart := openLoc[1]
	if sumLoc := reSummary.FindStringIndex(raw); sumLoc != nil {
		bodyStart = sumLoc[1]
	}

	// Find the </details> that closes the OUTER <details> by tracking depth over
	// the remaining open/close tags after the body start.
	body := raw[bodyStart:]
	closeRel, closeEnd := matchingDetailsClose(body)
	if closeRel < 0 {
		// Shouldn't happen given reDetailsEnd matched, but be safe.
		return summary, strings.TrimSpace(body), "", true
	}
	// inner and remainder are sliced from the ORIGINAL body using the matched
	// offsets, so masking (used only to find the true close) never reaches the
	// returned text.
	return summary, body[:closeRel], body[closeEnd:], true
}

// detailsOpenerTrailingContent returns any content that appears after the
// </summary> in a <details> opener HTMLBlock that does NOT contain a </details>.
// goldmark folds a tight first child line into this opener block; that content
// would otherwise be lost.
func detailsOpenerTrailingContent(raw string) string {
	sumLoc := reSummary.FindStringIndex(raw)
	if sumLoc == nil {
		return ""
	}
	return raw[sumLoc[1]:]
}

// matchingDetailsClose returns the byte offsets (within s) of the </details>
// tag that closes the outer <details>, accounting for nested <details> elements
// AND ignoring any <details>/</details> tags that fall inside fenced code blocks
// (``` / ~~~) or inline code spans (backtick runs). It scans a masked copy of s
// (code regions replaced with a neutral byte of EQUAL length, so all offsets map
// 1:1 back to the original) for openers/closers, incrementing/decrementing
// depth; the close at depth 0 is the match. It returns (start, end) where start
// is the offset of the matched </details> and end is the offset just after it
// (so the caller can slice inner = s[:start] and remainder = s[end:] from the
// ORIGINAL text). Returns (-1, -1) if no matching close is found.
func matchingDetailsClose(s string) (start int, end int) {
	masked := maskCodeRegions(s)

	type tok struct {
		idx   int
		end   int
		isEnd bool
	}
	var toks []tok
	for _, loc := range reDetailsOpen.FindAllStringIndex(masked, -1) {
		toks = append(toks, tok{idx: loc[0], end: loc[1], isEnd: false})
	}
	for _, loc := range reDetailsEnd.FindAllStringIndex(masked, -1) {
		toks = append(toks, tok{idx: loc[0], end: loc[1], isEnd: true})
	}
	// Sort tokens by position (simple insertion sort; lists are tiny).
	for i := 1; i < len(toks); i++ {
		for j := i; j > 0 && toks[j-1].idx > toks[j].idx; j-- {
			toks[j-1], toks[j] = toks[j], toks[j-1]
		}
	}
	depth := 0
	for _, tk := range toks {
		if tk.isEnd {
			if depth == 0 {
				return tk.idx, tk.end
			}
			depth--
		} else {
			depth++
		}
	}
	return -1, -1
}

// maskCodeRegions returns a copy of s in which the bytes belonging to fenced
// code blocks (``` or ~~~) and inline code spans (backtick runs) are replaced
// with a neutral space, preserving the original byte length so that offsets into
// the returned string map 1:1 onto the original. This lets the </details> close
// scan ignore tags that live inside code, which goldmark would render verbatim
// (Major 2). Only the FENCE/DELIMITER-driven structure is considered; real HTML
// tags outside code stay intact.
//
// Fences: a line whose first non-space run is >= 3 of the same fence char
// (` or ~) opens a fenced block; the block closes at the next line whose fence
// run is the same char and at least as long (an opening info string is allowed,
// a closing fence must not have one). Goldmark allows up to 3 leading spaces of
// indentation on a fence line; we mirror that.
//
// Inline spans: a run of N backticks opens an inline code span that closes at
// the next run of exactly N backticks on the same logical text. Code spans are
// only scanned outside fenced blocks.
func maskCodeRegions(s string) string {
	out := []byte(s)
	n := len(out)

	maskRange := func(from, to int) {
		for i := from; i < to && i < n; i++ {
			if out[i] != '\n' {
				out[i] = ' '
			}
		}
	}

	i := 0
	atLineStart := true
	for i < n {
		if atLineStart {
			// Detect a fence opener at the start of this line (allow up to 3 spaces).
			lineStart := i
			j := i
			spaces := 0
			for j < n && out[j] == ' ' && spaces < 4 {
				j++
				spaces++
			}
			if spaces < 4 && j < n && (out[j] == '`' || out[j] == '~') {
				fenceChar := out[j]
				runLen := 0
				k := j
				for k < n && out[k] == fenceChar {
					k++
					runLen++
				}
				if runLen >= 3 {
					// Find the end of this fenced block: scan subsequent lines for a
					// closing fence (same char, run >= runLen, no trailing non-space
					// for backtick fences with info strings is not required to close).
					// First, advance to end of the opening fence line.
					lineEnd := indexByteFrom(out, '\n', k)
					blockStart := lineStart
					var blockEnd int
					if lineEnd < 0 {
						blockEnd = n
					} else {
						blockEnd = lineEnd + 1
						// Now scan following lines.
						closed := false
						for blockEnd < n {
							ls := blockEnd
							le := indexByteFrom(out, '\n', ls)
							var lineLimit int
							if le < 0 {
								lineLimit = n
							} else {
								lineLimit = le
							}
							// Inspect this line for a closing fence.
							p := ls
							sp := 0
							for p < lineLimit && out[p] == ' ' && sp < 4 {
								p++
								sp++
							}
							clRun := 0
							for p < lineLimit && out[p] == fenceChar {
								p++
								clRun++
							}
							// A closing fence: same char, run >= opening run, and only
							// spaces after it on the line.
							onlySpaces := true
							for q := p; q < lineLimit; q++ {
								if out[q] != ' ' && out[q] != '\t' {
									onlySpaces = false
									break
								}
							}
							if sp < 4 && clRun >= runLen && onlySpaces {
								if le < 0 {
									blockEnd = n
								} else {
									blockEnd = le + 1
								}
								closed = true
								break
							}
							if le < 0 {
								blockEnd = n
								break
							}
							blockEnd = le + 1
						}
						if !closed {
							blockEnd = n
						}
					}
					maskRange(blockStart, blockEnd)
					i = blockEnd
					atLineStart = true
					continue
				}
			}
		}

		switch out[i] {
		case '\n':
			atLineStart = true
			i++
		case '`':
			// Inline code span: run of N backticks .. matching run of N backticks.
			runLen := 0
			start := i
			for i < n && out[i] == '`' {
				i++
				runLen++
			}
			// Find a closing run of exactly runLen backticks.
			closeAt := -1
			for p := i; p < n; {
				if out[p] == '`' {
					cl := 0
					q := p
					for q < n && out[q] == '`' {
						q++
						cl++
					}
					if cl == runLen {
						closeAt = q
						break
					}
					p = q
				} else {
					p++
				}
			}
			if closeAt >= 0 {
				maskRange(start, closeAt)
				i = closeAt
			}
			// If unterminated, leave as-is (goldmark treats it as literal backticks).
			atLineStart = false
		default:
			atLineStart = false
			i++
		}
	}
	return string(out)
}

func indexByteFrom(b []byte, c byte, from int) int {
	for i := from; i < len(b); i++ {
		if b[i] == c {
			return i
		}
	}
	return -1
}

func (r *Renderer) renderList(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.List)

	r.SetListState(entering, n.IsOrdered())

	return ast.WalkContinue, nil
}

func (r *Renderer) renderListItem(_ util.BufWriter,
	source []byte,
	n ast.Node,
	entering bool) (ast.WalkStatus, error) {
	tag := model.BlockContentText_Marked

	if r.GetIsNumberedList() {
		tag = model.BlockContentText_Numbered
	}

	r.openTextBlockWithStyle(entering, tag, nil)
	return ast.WalkContinue, nil
}

func (r *Renderer) renderParagraph(_ util.BufWriter,
	source []byte,
	n ast.Node,
	entering bool) (ast.WalkStatus, error) {
	r.openTextBlockWithStyle(entering, model.BlockContentText_Paragraph, nil)
	return ast.WalkContinue, nil
}

func (r *Renderer) renderTextBlock(_ util.BufWriter,
	source []byte,
	n ast.Node,
	entering bool) (ast.WalkStatus, error) {
	if !entering {
		// TODO: check it
		// r.CloseTextBlock(model.BlockContentText_Paragraph)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderThematicBreak(_ util.BufWriter,
	source []byte,
	n ast.Node,
	entering bool) (ast.WalkStatus, error) {
	if r.inTable {
		return ast.WalkContinue, nil
	}
	if entering {
		r.ForceCloseTextBlock()
	} else {
		r.AddDivider()
	}

	return ast.WalkContinue, nil
}

func (r *Renderer) renderAutoLink(_ util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.AutoLink)
	if !entering {
		return ast.WalkContinue, nil
	}

	label := n.Label(source)
	r.SetMarkStart()

	start := int32(text.UTF16RuneCountString(r.GetText()))
	labelLength := int32(text.UTF16RuneCount(label))

	linkPath, err := url.PathUnescape(string(label))
	if err != nil {
		log.Errorf("failed to unescape label %s", err)
		linkPath = string(label)
	}

	if !IsUrl(linkPath) {
		// Treat as a file path if no URL scheme
		linkPath = filepath.Join(r.GetBaseFilepath(), linkPath)
		linkPath = cleanLinkSection(linkPath)
	}

	r.AddMark(model.BlockContentTextMark{
		Range: &model.Range{From: start, To: start + labelLength},
		Type:  model.BlockContentTextMark_Link,
		Param: linkPath,
	})
	r.AddTextToBuffer(string(util.EscapeHTML(label)))
	return ast.WalkContinue, nil
}

func IsUrl(raw string) bool {
	colon := strings.IndexByte(raw, ':')

	if colon > 0 {
		scheme := raw[:colon]
		if isASCIIAlpha(scheme) {
			return true
		}
	}

	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && len(u.Scheme) > 1 {
		return true
	}

	return false
}

func isASCIIAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) || r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func (r *Renderer) renderCodeSpan(_ util.BufWriter,
	source []byte,
	n ast.Node,
	entering bool) (ast.WalkStatus, error) {
	if entering {
		r.SetMarkStart()

		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			segment := c.(*ast.Text).Segment
			value := segment.Value(source)
			s := Unescape(string(value))
			if bytes.HasSuffix(value, []byte("\n")) {
				r.AddTextToBuffer(s[:len(s)-1])
				if c != n.LastChild() {
					r.AddTextToBuffer(" ")
				}
			} else {
				r.AddTextToBuffer(s)
			}
		}
		return ast.WalkSkipChildren, nil
	}
	to := int32(text.UTF16RuneCountString(r.GetText()))

	r.AddMark(model.BlockContentTextMark{
		Range: &model.Range{From: int32(r.GetMarkStart()), To: to},
		Type:  model.BlockContentTextMark_Keyboard,
	})

	return ast.WalkContinue, nil
}

func (r *Renderer) renderEmphasis(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Emphasis)
	tag := model.BlockContentTextMark_Italic
	if n.Level == 2 {
		tag = model.BlockContentTextMark_Bold
	}

	if entering {
		r.SetMarkStart()
	} else {
		to := int32(text.UTF16RuneCountString(r.GetText()))

		r.AddMark(model.BlockContentTextMark{
			Range: &model.Range{From: int32(r.GetMarkStart()), To: to},
			Type:  tag,
		})
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderLink(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Link)

	destination := n.Destination

	if entering {
		r.SetMarkStart()
	} else {

		linkPath, err := url.PathUnescape(string(destination))
		if err != nil {
			log.Errorf("failed to unescape destination %s", err)
			linkPath = string(destination)
		}

		if !IsUrl(linkPath) {
			// Treat as a file path if no URL scheme
			linkPath = filepath.Join(r.GetBaseFilepath(), linkPath)
			ext := filepath.Ext(linkPath)
			// if empty or contains spaces
			linkPath = cleanLinkSection(linkPath)

			// todo: should be improved
			if ext == "" || strings.Contains(ext, " ") {
				linkPath += ".md" // Default to .md if no extension is provided
			}
		}

		to := int32(text.UTF16RuneCountString(r.GetText()))

		r.AddMark(model.BlockContentTextMark{
			Range: &model.Range{From: int32(r.GetMarkStart()), To: to},
			Type:  model.BlockContentTextMark_Link,
			Param: linkPath,
		})
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderImage(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	n := node.(*ast.Image)
	if r.inTable {
		return ast.WalkSkipChildren, nil
	}
	r.AddImageBlock(string(n.Destination))

	return ast.WalkSkipChildren, nil
}

func (r *Renderer) renderRawHTML(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {
	n, ok := node.(*ast.RawHTML)
	if !ok {
		return ast.WalkSkipChildren, nil
	}
	for i := 0; i < n.Segments.Len(); i++ {
		segment := n.Segments.At(i)
		tag := segment.Value(source)
		switch string(tag) {
		case "<u>":
			if !entering {
				r.SetMarkStart()
			}
		case "</u>":
			if entering {
				tag := model.BlockContentTextMark_Underscored
				to := int32(text.UTF16RuneCountString(r.GetText()))
				r.AddMark(model.BlockContentTextMark{
					Range: &model.Range{From: int32(r.GetMarkStart()), To: to},
					Type:  tag,
				})
			}
		case "<br>", "<br/>", "<br />":
			if entering {
				r.AddTextToBuffer("\n")
			}
		default:
			return ast.WalkSkipChildren, nil
		}
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderText(_ util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool) (ast.WalkStatus, error) {

	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Text)
	segment := n.Segment
	s := string(segment.Value(source))
	s = Unescape(s)
	r.AddTextToBuffer(s)

	if n.HardLineBreak() || n.SoftLineBreak() && r.TextBufferLen() > TextBlockLengthSoftLimit {
		r.openTextBlockWithStyle(false, model.BlockContentText_Paragraph, nil)

	} else if n.SoftLineBreak() {
		r.AddTextToBuffer("\n")
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderString(_ util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.String)
	s := string(n.Value)
	s = Unescape(s)
	r.AddTextToBuffer(s)

	return ast.WalkContinue, nil
}

func (r *Renderer) renderStrikethrough(_ util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	tag := model.BlockContentTextMark_Strikethrough
	if entering {
		r.SetMarkStart()
	} else {
		to := int32(text.UTF16RuneCountString(r.GetText()))
		r.AddMark(model.BlockContentTextMark{
			Range: &model.Range{From: int32(r.GetMarkStart()), To: to},
			Type:  tag,
		})
	}
	return ast.WalkContinue, nil
}

func cleanLinkSection(linkPath string) string {
	// Remove any section markers from the link path.
	for _, char := range []string{"|", "#", "^"} {
		if idx := strings.LastIndex(linkPath, char); idx != -1 {
			linkPath = linkPath[:idx]
		}
	}
	return linkPath
}

func (r *Renderer) renderWikiLink(_ util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*wikilink.Node)
	linkPath := string(n.Target)

	// For embed syntax ![[]], check if it's an image
	if n.Embed && entering {
		// Check if destination has image extension
		lowerPath := strings.ToLower(linkPath)
		if strings.HasSuffix(lowerPath, ".png") || strings.HasSuffix(lowerPath, ".jpg") ||
			strings.HasSuffix(lowerPath, ".jpeg") || strings.HasSuffix(lowerPath, ".gif") ||
			strings.HasSuffix(lowerPath, ".svg") || strings.HasSuffix(lowerPath, ".webp") {
			// Handle as image block
			if !r.inTable {
				r.ForceCloseTextBlock()
				r.AddImageBlock(linkPath)
			}
			return ast.WalkSkipChildren, nil
		}
	}

	if entering {
		r.SetMarkStart()
	} else {
		// Handle as regular link (same behavior as [[]] for both [[]] and ![[]])
		if !IsUrl(linkPath) {
			// Treat as a file path if no URL scheme
			linkPath = filepath.Join(r.GetBaseFilepath(), linkPath)
			ext := filepath.Ext(linkPath)
			// if empty or contains spaces
			linkPath = cleanLinkSection(linkPath)

			// todo: should be improved
			if ext == "" || strings.Contains(ext, " ") {
				linkPath += ".md" // Default to .md if no extension is provided

			}
		}

		to := int32(text.UTF16RuneCountString(r.GetText()))

		r.AddMark(model.BlockContentTextMark{
			Range: &model.Range{From: int32(r.GetMarkStart()), To: to},
			Type:  model.BlockContentTextMark_Link,
			Param: linkPath,
		})
	}
	return ast.WalkContinue, nil
}
