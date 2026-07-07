package markdown

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const directoryIcon = "📂"

// dirTree is the synthetic directory-page structure: one page per directory
// under the (possibly collapsed) root, each linking its subdirectories and
// documents.
type dirTree struct {
	root string   // "." or the single collapsed top segment
	dirs []string // every directory incl. root, sorted, hidden dirs excluded
	// children maps a dir to its immediate subdirs and document entries.
	subdirs map[string][]string
	files   map[string][]string
}

func dirSourceKey(dir string) string { return "dir:" + dir }

// buildDirTree derives the directory structure from the document listing
// (md + csv entries, already in sorted walk order).
func buildDirTree(entries []source.Entry) *dirTree {
	tree := &dirTree{
		root:    ".",
		subdirs: map[string][]string{},
		files:   map[string][]string{},
	}

	// Root at the deepest directory common to every document (v1 rule):
	// collapses the typical single-top zip shape and deeper single-branch
	// chains that would otherwise become empty intermediate pages.
	if len(entries) > 0 {
		root := path.Dir(entries[0].Name)
		for _, e := range entries[1:] {
			root = commonDir(root, path.Dir(e.Name))
		}
		tree.root = root
	}

	dirSet := map[string]bool{tree.root: true}
	for _, e := range entries {
		dir := path.Dir(e.Name)
		if !withinDir(dir, tree.root) {
			continue
		}
		for d := dir; withinDir(d, tree.root); d = path.Dir(d) {
			if hiddenDir(d) {
				break
			}
			dirSet[d] = true
			if d == tree.root {
				break
			}
		}
		if !hiddenDir(dir) {
			tree.files[dir] = append(tree.files[dir], e.Name)
		}
	}
	for dir := range dirSet {
		tree.dirs = append(tree.dirs, dir)
		if dir != tree.root {
			tree.subdirs[path.Dir(dir)] = append(tree.subdirs[path.Dir(dir)], dir)
		}
	}
	sort.Strings(tree.dirs)
	for _, subdirs := range tree.subdirs {
		sort.Strings(subdirs)
	}
	// Documents listed alphabetically within a directory (v1 order; the
	// input arrives md-then-csv, not mixed).
	for _, files := range tree.files {
		sort.Strings(files)
	}
	return tree
}

// commonDir returns the deepest directory containing both, "." when they
// share no prefix.
func commonDir(a, b string) string {
	if a == "." || b == "." {
		return "."
	}
	if a == b {
		return a
	}
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	shared := 0
	for shared < len(as) && shared < len(bs) && as[shared] == bs[shared] {
		shared++
	}
	if shared == 0 {
		return "."
	}
	return strings.Join(as[:shared], "/")
}

func withinDir(dir, root string) bool {
	if root == "." {
		return true
	}
	return dir == root || strings.HasPrefix(dir, root+"/")
}

func hiddenDir(dir string) bool {
	if dir == "." {
		return false
	}
	for segment := range strings.SplitSeq(dir, "/") {
		if strings.HasPrefix(segment, ".") {
			return true
		}
	}
	return false
}

// emitDirectoryPages streams one page per directory: subdirectory links
// first, then document links (v1 ordering).
func (c *Converter) emitDirectoryPages(ctx context.Context, sink importv2.Sink) error {
	for _, dir := range c.dirs.dirs {
		blocks := make([]*model.Block, 0, len(c.dirs.subdirs[dir])+len(c.dirs.files[dir]))
		for i, subdir := range c.dirs.subdirs[dir] {
			blocks = append(blocks, &model.Block{
				Id: fmt.Sprintf("dirlink%d", i),
				Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
					TargetBlockId: dirSourceKey(subdir),
					Style:         model.BlockContentLink_Page,
				}},
			})
		}
		for i, file := range c.dirs.files[dir] {
			blocks = append(blocks, &model.Block{
				Id: fmt.Sprintf("doclink%d", i),
				Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
					TargetBlockId: file,
					Style:         model.BlockContentLink_Page,
					CardStyle:     model.BlockContentLink_Card,
				}},
			})
		}
		title := path.Base(dir)
		if dir == "." {
			title = rootCollectionName
		}
		details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:           domain.String(title),
			bundle.RelationKeyIconEmoji:      domain.String(directoryIcon),
			bundle.RelationKeySourceFilePath: domain.String(sourcePathHash(dirSourceKey(dir))),
		})
		object := &importv2.Object{
			SourceKey: dirSourceKey(dir),
			SbType:    coresb.SmartBlockTypePage,
			Payload: &importv2.Snapshot{
				Blocks:      blocks,
				Details:     details,
				ObjectTypes: []string{bundle.TypeKeyPage.String()},
			},
			IsRootCandidate: dir == c.dirs.root,
		}
		if err := sink.Object(ctx, object); err != nil {
			return err
		}
	}
	return nil
}
