package markdown

import (
	"path"
	"slices"
	"sort"
	"strings"

	yamlv3 "gopkg.in/yaml.v3"

	"github.com/anyproto/anytype-heart/core/block/importv2/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// setCreatedInContext records the object a document was created in as its
// createdInContext pair (source-key form; the resolver maps the key to the
// final id). With directory pages the generated directory page is the
// parent; otherwise the first matching rule wins:
//
//  1. Rows: a markdown page directly inside `X/` with `X.csv` beside it is
//     a row of that collection (no ref, the shape of collection
//     membership). Notion database exports.
//  2. Front matter: a `parent` or `up` property naming a page of the import
//     (Obsidian Breadcrumbs/Dataview conventions).
//  3. The folder note of the document's directory, linked or not: `X.md`
//     beside `X/` (Notion, Outline), else `X/X.md`, `X/index.md`,
//     `X/_index.md`, `X/README.md` inside it (Obsidian Folder Notes,
//     Docusaurus, MkDocs, Hugo). A folder note itself is placed where its
//     directory is.
//  4. A page in the directory just above linking the document, unless that
//     page is a hub linking into several directories (a vault's Home or
//     index page links deep notes it does not own).
//  5. The folder note of the nearest ancestor directory.
//
// The ref is the parent's first block linking the document, when the parent
// was converted first and links it; empty otherwise.
func (c *Converter) setCreatedInContext(entryName string, frontMatterParents []string, details *domain.Details) {
	parent, ok := c.parentOf(entryName, frontMatterParents)
	if !ok {
		return
	}
	c.chosenParents[entryName] = parent.key
	details.SetString(bundle.RelationKeyCreatedInContext, parent.key)
	if parent.ref != "" {
		details.SetString(bundle.RelationKeyCreatedInContextRef, parent.ref)
	}
}

// parentOf returns the first rule's parent that does not close a cycle with
// the parents already chosen. Every edge is checked when it is added, so the
// chosen parents always form a forest (front matter can name any page, and
// a folder note's own parent comes from the folder rules).
func (c *Converter) parentOf(entryName string, frontMatterParents []string) (parentLink, bool) {
	if c.dirs != nil {
		parent, ok := c.dirs.parentOf[entryName]
		return parent, ok
	}
	for _, parent := range c.parentCandidates(entryName, frontMatterParents) {
		if parent.key != entryName && !c.reaches(parent.key, entryName) {
			return parent, true
		}
	}
	return parentLink{}, false
}

// reaches reports whether from's chain of chosen parents arrives at target.
func (c *Converter) reaches(from, target string) bool {
	for steps, at := 0, from; steps <= len(c.chosenParents); steps++ {
		if at == target {
			return true
		}
		next, ok := c.chosenParents[at]
		if !ok {
			return false
		}
		at = next
	}
	return false
}

// parentCandidates lists the rules' parents in precedence order.
func (c *Converter) parentCandidates(entryName string, frontMatterParents []string) []parentLink {
	var candidates []parentLink
	dir := path.Dir(entryName)
	if c.flavour.CSVCollections && dir != "." && isMarkdown(entryName) {
		// csvMembers' rule: the md pages directly inside `X/` are `X.csv`'s rows.
		if csvName, ok := c.csvForDir(dir); ok {
			candidates = append(candidates, parentLink{key: csvName})
		}
	}
	for _, parent := range frontMatterParents {
		candidates = append(candidates, c.linkedParent(parent, entryName))
	}
	// home is the directory the document is placed in: its own, or for a
	// folder note inside its directory, the directory above.
	home := dir
	if c.isInsideFolderNote(entryName) {
		home = path.Dir(dir)
	}
	if home != "." {
		if note, ok := c.folderNoteOf(home); ok {
			candidates = append(candidates, c.linkedParent(note, entryName))
		}
	}
	candidates = append(candidates, c.linkersAbove(entryName)...)
	if home != "." {
		for ancestor := path.Dir(home); ancestor != "."; ancestor = path.Dir(ancestor) {
			if note, ok := c.folderNoteOf(ancestor); ok {
				candidates = append(candidates, c.linkedParent(note, entryName))
				break
			}
		}
	}
	return candidates
}

// csvForDir finds the csv collection named after dir, extension in any case
// (the listing accepts `.CSV` too).
func (c *Converter) csvForDir(dir string) (string, bool) {
	if c.csvByDir == nil {
		c.csvByDir = map[string]string{}
		for _, entry := range c.csvEntries {
			key := strings.TrimSuffix(entry.Name, path.Ext(entry.Name))
			if _, taken := c.csvByDir[key]; !taken {
				c.csvByDir[key] = entry.Name
			}
		}
	}
	name, ok := c.csvByDir[dir]
	return name, ok
}

// linkedParent pairs a chosen parent with its first block linking the child.
func (c *Converter) linkedParent(parent, child string) parentLink {
	for _, link := range c.linksTo[child] {
		if link.key == parent {
			return link
		}
	}
	return parentLink{key: parent}
}

// linkersAbove is rule 4: the pages in the directory just above the
// document's that link it and are not hubs, in conversion order (the cycle
// check in parentOf may reject the first).
func (c *Converter) linkersAbove(entryName string) []parentLink {
	dir := path.Dir(entryName)
	if dir == "." {
		return nil
	}
	above := path.Dir(dir)
	var linkers []parentLink
	for _, link := range c.linksTo[entryName] {
		if path.Dir(link.key) == above && len(c.linkedSubtrees[link.key]) < 2 {
			linkers = append(linkers, link)
		}
	}
	return linkers
}

// indexLinks records every page the converted page links — link blocks and
// mention marks of its final blocks, embeds and metadata-generated mentions
// included — with the first block doing so. Links into directories below the
// page also count the page's distinct linked subtrees, which is how a hub is
// told from a parent: `Projects.md` links into `Projects/` only, a Home page
// into everything.
func (c *Converter) indexLinks(pageName string, blocks []*model.Block) {
	if !isMarkdown(pageName) {
		return
	}
	for _, b := range blocks {
		if link := b.GetLink(); link != nil {
			c.noteLink(pageName, link.TargetBlockId, b.Id)
		}
		for _, mark := range b.GetText().GetMarks().GetMarks() {
			if mark.Type == model.BlockContentTextMark_Mention {
				c.noteLink(pageName, mark.Param, b.Id)
			}
		}
	}
}

func (c *Converter) noteLink(pageName, target, blockId string) {
	if target == pageName || !c.isSourceEntry(target) || !c.isPageEntry(target) {
		return
	}
	for _, link := range c.linksTo[target] {
		if link.key == pageName {
			return
		}
	}
	c.linksTo[target] = append(c.linksTo[target], parentLink{key: pageName, ref: blockId})
	pageDir, targetDir := path.Dir(pageName), path.Dir(target)
	if !isAncestorDir(pageDir, targetDir) {
		return
	}
	rest := targetDir
	if pageDir != "." {
		rest = strings.TrimPrefix(targetDir, pageDir+"/")
	}
	subtree, _, _ := strings.Cut(rest, "/")
	if c.linkedSubtrees[pageName] == nil {
		c.linkedSubtrees[pageName] = map[string]bool{}
	}
	c.linkedSubtrees[pageName][subtree] = true
}

// folderNoteOf returns the page standing for dir, in convention priority.
func (c *Converter) folderNoteOf(dir string) (string, bool) {
	if dir == "." {
		return "", false
	}
	if note, ok := c.folderNotes[dir]; ok {
		return note, note != ""
	}
	note := ""
	for _, stem := range []string{
		dir,
		dir + "/" + path.Base(dir),
		dir + "/index",
		dir + "/_index",
		dir + "/README",
		dir + "/readme",
	} {
		if entry, ok := c.markdownByStem()[stem]; ok {
			note = entry
			break
		}
	}
	c.folderNotes[dir] = note
	return note, note != ""
}

// markdownByStem maps each markdown entry's path without its extension to
// the entry, so lookups accept `.md` in any case; the first entry in listing
// order wins a stem shared by several casings.
func (c *Converter) markdownByStem() map[string]string {
	if c.mdStems == nil {
		c.mdStems = make(map[string]string, len(c.mdEntries))
		for _, entry := range c.mdEntries {
			stem := strings.TrimSuffix(entry.Name, path.Ext(entry.Name))
			if _, taken := c.mdStems[stem]; !taken {
				c.mdStems[stem] = entry.Name
			}
		}
	}
	return c.mdStems
}

// isInsideFolderNote reports whether the entry is the folder note its own
// directory resolves to from inside (`X/X.md`, `X/index.md`, ...).
func (c *Converter) isInsideFolderNote(entryName string) bool {
	note, ok := c.folderNoteOf(path.Dir(entryName))
	return ok && note == entryName
}

// frontMatterParentsOf reads `parent` and `up` properties (any key case,
// every value of a list) and resolves them to pages of the import, in
// precedence order: a wikilink (`[[Note]]`, `[[Note|alias]]`), a relative or
// root path, with or without the .md extension. Unresolvable values are
// ignored; the cycle check in parentOf may still reject any of them, so all
// are kept for it to fall through.
func (c *Converter) frontMatterParentsOf(entryName string, frontMatter []byte) []string {
	var fields map[string]any
	if err := yamlv3.Unmarshal(frontMatter, &fields); err != nil {
		return nil
	}
	// `parent` before `up`; within one, the exact lowercase key before other
	// casings (sorted, so the choice never depends on map order).
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rank := func(key string) int {
		switch {
		case key == "parent":
			return 0
		case strings.EqualFold(key, "parent"):
			return 1
		case key == "up":
			return 2
		case strings.EqualFold(key, "up"):
			return 3
		}
		return -1
	}
	sort.SliceStable(keys, func(i, j int) bool { return rank(keys[i]) < rank(keys[j]) })
	var parents []string
	for _, key := range keys {
		if rank(key) < 0 {
			continue
		}
		values, isList := fields[key].([]any)
		if !isList {
			values = []any{fields[key]}
		}
		for _, value := range values {
			ref, ok := value.(string)
			if !ok {
				continue
			}
			if target, ok := c.resolvePageRef(entryName, ref); ok && !slices.Contains(parents, target) {
				parents = append(parents, target)
			}
		}
	}
	return parents
}

func (c *Converter) resolvePageRef(fromPage, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimSuffix(strings.TrimPrefix(ref, "[["), "]]")
	ref, _, _ = strings.Cut(ref, "|")
	ref, _, _ = strings.Cut(ref, "#")
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	// A leading slash is the source root, never the page's directory.
	bases := []string{path.Join(path.Dir(fromPage), ref), ref}
	if rooted := strings.TrimLeft(ref, "/"); rooted != ref {
		bases = []string{rooted}
	}
	extensionless := !strings.EqualFold(path.Ext(ref), ".md")
	for _, base := range bases {
		if entry, ok := c.lookupEntry(base); ok && c.isPageEntry(entry) {
			return entry, true
		}
		if !extensionless {
			continue
		}
		// The page as named, with its markdown extension in any case
		// (`[[Parent]]` → `Parent.MD`), before the lowercase guess that
		// lookupEntry may widen to a basename match elsewhere.
		if entry, ok := c.markdownByStem()[base]; ok {
			return entry, true
		}
		if entry, ok := c.lookupEntry(base + ".md"); ok && c.isPageEntry(entry) {
			return entry, true
		}
	}
	return "", false
}

// conversionOrder sorts pages shallowest directory first and, within a
// directory, its folder note first, so parents are converted before their
// children and the children's refs can name the linking block.
func (c *Converter) conversionOrder(entries []source.Entry) []source.Entry {
	ordered := append([]source.Entry{}, entries...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i].Name, ordered[j].Name
		if da, db := dirDepth(path.Dir(a)), dirDepth(path.Dir(b)); da != db {
			return da < db
		}
		if na, nb := c.isInsideFolderNote(a), c.isInsideFolderNote(b); na != nb {
			return na
		}
		return a < b
	})
	return ordered
}

// isAncestorDir reports whether dir lies strictly above descendant.
func isAncestorDir(dir, descendant string) bool {
	if dir == descendant {
		return false
	}
	return dir == "." || strings.HasPrefix(descendant, dir+"/")
}

func dirDepth(dir string) int {
	if dir == "." {
		return 0
	}
	return strings.Count(dir, "/") + 1
}

func (c *Converter) isSourceEntry(name string) bool {
	_, ok := c.source.Stat(name)
	return ok
}

func isMarkdown(name string) bool {
	return strings.EqualFold(path.Ext(name), ".md")
}
